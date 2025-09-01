// internal/core/converter/service.go
package converter

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/LuisEduardoPedra/analiseSped/internal/domain"
	"github.com/schollz/closestmatch"
	"github.com/shakinm/xlsReader/xls" // <-- BIBLIOTECA SUBSTITUÍDA
	"github.com/xuri/excelize/v2"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Service define a interface para o serviço de conversão de arquivos.
type Service interface {
	ProcessSicrediFiles(lancamentosFile io.Reader, contasFile io.Reader, lancamentosFilename string) ([]byte, error)
}

type service struct{}

// NewService cria uma nova instância do serviço de conversão.
func NewService() Service {
	return &service{}
}

// convertXLSXtoCSV converte um arquivo .xlsx para um CSV em memória.
func (svc *service) convertXLSXtoCSV(file io.Reader) (io.Reader, error) {
	f, err := excelize.OpenReader(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	writer.Comma = ';'

	for _, name := range f.GetSheetList() {
		rows, err := f.GetRows(name)
		if err != nil {
			continue
		}
		for _, row := range rows {
			if err := writer.Write(row); err != nil {
				return nil, err
			}
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}

	return &buffer, nil
}

// convertXLStoCSV foi reescrita para usar a biblioteca 'shakinm/xlsReader'.
func (svc *service) convertXLStoCSV(file io.Reader) (io.Reader, error) {
	// A biblioteca 'xlsReader' precisa de um caminho de arquivo ou de um objeto *os.File.
	// A maneira mais fácil de lidar com um io.Reader é salvá-lo em um arquivo temporário.
	tempFile, err := os.CreateTemp("", "temp-*.xls")
	if err != nil {
		return nil, fmt.Errorf("falha ao criar arquivo temporário: %w", err)
	}
	defer os.Remove(tempFile.Name()) // Garante que o arquivo seja deletado no final
	defer tempFile.Close()

	// Copia o conteúdo do io.Reader para o arquivo temporário
	if _, err := io.Copy(tempFile, file); err != nil {
		return nil, fmt.Errorf("falha ao escrever no arquivo temporário: %w", err)
	}
	// Fecha o arquivo para que xls.OpenFile possa lê-lo
	tempFile.Close()

	// Abre o arquivo .xls a partir do caminho temporário
	workbook, err := xls.OpenFile(tempFile.Name())
	if err != nil {
		return nil, fmt.Errorf("falha ao abrir o arquivo .xls: %w", err)
	}

	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	writer.Comma = ';'

	// Itera sobre todas as abas do workbook
	for sheetIndex := 0; sheetIndex < workbook.GetNumberSheets(); sheetIndex++ {
		sheet, err := workbook.GetSheet(sheetIndex)
		if err != nil || sheet == nil {
			continue
		}

		// Itera pelas linhas da aba
		for i := 0; i <= int(sheet.GetNumberRows()); i++ {
			row, err := sheet.GetRow(i)
			if err != nil || row == nil {
				continue // Pula linhas vazias ou com erro
			}

			var csvRow []string
			// Itera sobre as colunas da linha
			for _, col := range row.GetCols() {
				if col != nil {
					// GetString() é o método para obter o valor da célula como texto
					csvRow = append(csvRow, col.GetString())
				} else {
					csvRow = append(csvRow, "") // Adiciona uma string vazia para células nulas
				}
			}

			if err := writer.Write(csvRow); err != nil {
				return nil, err
			}
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}

	return &buffer, nil
}

// ProcessSicrediFiles executa a lógica principal de conversão.
func (svc *service) ProcessSicrediFiles(lancamentosFile io.Reader, contasFile io.Reader, lancamentosFilename string) ([]byte, error) {

	var lancamentosCSVReader io.Reader
	ext := strings.ToLower(filepath.Ext(lancamentosFilename))

	switch ext {
	case ".xlsx":
		csvData, err := svc.convertXLSXtoCSV(lancamentosFile)
		if err != nil {
			return nil, fmt.Errorf("erro ao converter .xlsx para .csv: %w", err)
		}
		lancamentosCSVReader = csvData
	case ".xls":
		csvData, err := svc.convertXLStoCSV(lancamentosFile)
		if err != nil {
			return nil, fmt.Errorf("erro ao converter .xls para .csv: %w", err)
		}
		lancamentosCSVReader = csvData
	case ".csv":
		lancamentosCSVReader = lancamentosFile
	default:
		return nil, fmt.Errorf("formato de arquivo de lançamentos não suportado: %s", ext)
	}

	contasMap, err := svc.carregarContas(contasFile)
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar arquivo de contas: %w", err)
	}

	descricoesContas := make([]string, 0, len(contasMap))
	for descNorm := range contasMap {
		descricoesContas = append(descricoesContas, descNorm)
	}
	cm := closestmatch.New(descricoesContas, []int{3, 4})

	lancamentos, err := svc.carregarLancamentos(lancamentosCSVReader)
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar arquivo de lançamentos: %w", err)
	}

	sort.Slice(lancamentos, func(i, j int) bool {
		return lancamentos[i].DataLiquidacao.Before(lancamentos[j].DataLiquidacao)
	})

	finalRows := svc.montarOutput(lancamentos, contasMap, cm)

	outputCSV, err := svc.gerarCSV(finalRows)
	if err != nil {
		return nil, fmt.Errorf("erro ao gerar CSV final: %w", err)
	}

	return outputCSV, nil
}

func (svc *service) carregarContas(contasFile io.Reader) (map[string]string, error) {
	decoder := charmap.ISO8859_1.NewDecoder()
	reader := csv.NewReader(transform.NewReader(contasFile, decoder))
	reader.Comma = ';'
	reader.LazyQuotes = true

	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	contasMap := make(map[string]string)
	for _, record := range records {
		if len(record) < 3 {
			continue
		}
		codigo := strings.TrimSpace(record[0])
		descricao := strings.TrimSpace(record[2])

		if _, err := strconv.Atoi(codigo); err != nil || descricao == "" {
			continue
		}

		descNorm := svc.normalizeText(descricao)
		if _, exists := contasMap[descNorm]; !exists {
			contasMap[descNorm] = codigo
		}
	}
	return contasMap, nil
}

func (svc *service) carregarLancamentos(lancamentosFile io.Reader) ([]domain.Lancamento, error) {
	decoder := charmap.ISO8859_1.NewDecoder()
	reader := csv.NewReader(transform.NewReader(lancamentosFile, decoder))
	reader.Comma = ';'
	reader.LazyQuotes = true

	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	var lancamentos []domain.Lancamento
	for _, record := range records {
		if len(record) < 9 || !strings.HasPrefix(strings.ToUpper(record[0]), "SIMPLES") {
			continue
		}

		dataLiq, err := time.Parse("02/01/2006", record[6])
		if err != nil {
			continue
		}

		valor, err := svc.parseBRLNumber(record[8])
		if err != nil {
			valor = 0.0
		}

		descricaoCredito := strings.TrimSpace(record[4])

		historico := fmt.Sprintf("RECEBIMENTO DE %s CONFORME BOLETO %s COM VENCIMENTO EM %s REFERENTE DOCUMENTO %s",
			descricaoCredito, record[2], record[5], record[1])

		lancamentos = append(lancamentos, domain.Lancamento{
			DataLiquidacao: dataLiq,
			Descricao:      descricaoCredito,
			Valor:          valor,
			Historico:      historico,
		})
	}
	return lancamentos, nil
}

func (svc *service) montarOutput(lancamentos []domain.Lancamento, contasMap map[string]string, cm *closestmatch.ClosestMatch) []domain.OutputRow {
	if len(lancamentos) == 0 {
		return nil
	}

	var finalRows []domain.OutputRow
	var group []domain.Lancamento
	currentDate := lancamentos[0].DataLiquidacao

	for _, l := range lancamentos {
		if l.DataLiquidacao.Equal(currentDate) {
			group = append(group, l)
		} else {
			svc.processarGrupo(group, &finalRows, contasMap, cm)
			group = []domain.Lancamento{l}
			currentDate = l.DataLiquidacao
		}
	}
	svc.processarGrupo(group, &finalRows, contasMap, cm)

	return finalRows
}

func (svc *service) processarGrupo(grupo []domain.Lancamento, finalRows *[]domain.OutputRow, contasMap map[string]string, cm *closestmatch.ClosestMatch) {
	if len(grupo) == 0 {
		return
	}

	var totalDiario float64
	for _, l := range grupo {
		totalDiario += l.Valor
	}

	dataLancamento := grupo[0].DataLiquidacao.AddDate(0, 0, 1).Format("02/01/2006")

	*finalRows = append(*finalRows, domain.OutputRow{
		Operacao:     "D",
		Data:         dataLancamento,
		ContaCredito: "99999999",
		Valor:        strings.Replace(fmt.Sprintf("%.2f", totalDiario), ".", ",", 1),
		Historico:    "Títulos recebidos na data",
	})

	for _, l := range grupo {
		descNorm := svc.normalizeText(l.Descricao)
		codigoConta := "99999999"
		if code, ok := contasMap[descNorm]; ok {
			codigoConta = code
		} else {
			match := cm.Closest(descNorm)
			if match != "" {
				if code, ok := contasMap[match]; ok {
					codigoConta = code
				}
			}
		}

		*finalRows = append(*finalRows, domain.OutputRow{
			Operacao:         "C",
			Data:             dataLancamento,
			DescricaoCredito: l.Descricao,
			ContaCredito:     codigoConta,
			Valor:            strings.Replace(fmt.Sprintf("%.2f", l.Valor), ".", ",", 1),
			Historico:        l.Historico,
		})
	}
}

func (svc *service) gerarCSV(rows []domain.OutputRow) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := charmap.Windows1252.NewEncoder()
	writer := csv.NewWriter(transform.NewWriter(&buffer, encoder))
	writer.Comma = ';'

	header := []string{"Operação", "Data", "Descrição Credito", "Conta Credito", "Valor", "Historico"}
	if err := writer.Write(header); err != nil {
		return nil, err
	}

	for _, row := range rows {
		record := []string{row.Operacao, row.Data, row.DescricaoCredito, row.ContaCredito, row.Valor, row.Historico}
		if err := writer.Write(record); err != nil {
			return nil, err
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

var nonAlphanumericRegex = regexp.MustCompile(`[^A-Z0-9 ]+`)
var whitespaceRegex = regexp.MustCompile(`\s+`)

func (svc *service) normalizeText(str string) string {
	t := transform.Chain(norm.NFD, transform.RemoveFunc(func(r rune) bool {
		return unicode.Is(unicode.Mn, r)
	}))
	result, _, _ := transform.String(t, str)
	result = strings.ToUpper(result)
	result = nonAlphanumericRegex.ReplaceAllString(result, " ")
	result = whitespaceRegex.ReplaceAllString(result, " ")
	return strings.TrimSpace(result)
}

func (svc *service) parseBRLNumber(val string) (float64, error) {
	s := strings.TrimSpace(val)
	if s == "" {
		return 0, nil
	}
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, ",", ".")
	return strconv.ParseFloat(s, 64)
}
