package converter

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/LuisEduardoPedra/analiseSped/internal/domain"
	"github.com/schollz/closestmatch"
	"github.com/shakinm/xlsReader/xls"
	"github.com/xuri/excelize/v2"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Service define a interface para os serviços de conversão de arquivos.
type Service interface {
	ProcessSicrediFiles(lancamentosFile io.Reader, contasFile io.Reader, lancamentosFilename string, classPrefixes []string) ([]byte, error)
	ProcessReceitasAcisaFiles(excelFile io.Reader, contasFile io.Reader, excelFilename string, classPrefixes []string) ([]byte, error)
}

type service struct{}

// NewService cria uma nova instância do serviço de conversão.
func NewService() Service {
	return &service{}
}

// Funções utilitárias compartilhadas
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
		return 0.0, nil
	}
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, ",", ".")
	return strconv.ParseFloat(s, 64)
}

func (svc *service) formatTwoDecimalsComma(val float64) string {
	return strings.Replace(fmt.Sprintf("%.2f", val), ".", ",", 1)
}

// #############################################################################
// #                         CONVERSOR FRANCESINHA (SICREDI)                     #
// #############################################################################

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
	return &buffer, writer.Error()
}

func (svc *service) convertXLStoCSV(file io.Reader) (io.Reader, error) {
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	reader := bytes.NewReader(data)

	workbook, err := xls.OpenReader(reader)
	if err != nil {
		if _, errX := excelize.OpenReader(bytes.NewReader(data)); errX == nil {
			return svc.convertXLSXtoCSV(bytes.NewReader(data))
		}
		return nil, err
	}

	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	writer.Comma = ';'

	for _, sheet := range workbook.GetSheets() {
		for _, row := range sheet.GetRows() {
			var csvRow []string
			for _, cell := range row.GetCols() {
				csvRow = append(csvRow, cell.GetString())
			}
			if err := writer.Write(csvRow); err != nil {
				return nil, err
			}
		}
	}

	writer.Flush()
	return &buffer, writer.Error()
}

func (svc *service) ProcessSicrediFiles(lancamentosFile io.Reader, contasFile io.Reader, lancamentosFilename string, classPrefixes []string) ([]byte, error) {
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

	contasEntries, allKeys, err := svc.loadContasSicredi(contasFile)
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar arquivo de contas: %w", err)
	}

	lancamentos, err := svc.carregarLancamentos(lancamentosCSVReader)
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar arquivo de lançamentos: %w", err)
	}

	sort.Slice(lancamentos, func(i, j int) bool {
		return lancamentos[i].DataLiquidacao.Before(lancamentos[j].DataLiquidacao)
	})

	finalRows := svc.montarOutputSicredi(lancamentos, contasEntries, allKeys, classPrefixes)

	outputCSV, err := svc.gerarCSVSicredi(finalRows)
	if err != nil {
		return nil, fmt.Errorf("erro ao gerar CSV final: %w", err)
	}

	return outputCSV, nil
}

func (svc *service) loadContasSicredi(contasFile io.Reader) (map[string][]domain.ContaSicredi, []string, error) {
	decoder := charmap.ISO8859_1.NewDecoder()
	reader := csv.NewReader(transform.NewReader(contasFile, decoder))
	reader.Comma = ';'
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1

	records, err := reader.ReadAll()
	if err != nil {
		return nil, nil, err
	}

	contasEntries := make(map[string][]domain.ContaSicredi)
	var allKeys []string
	keysMap := make(map[string]bool)

	for _, record := range records {
		if len(record) < 3 {
			continue
		}
		code := strings.TrimSpace(record[0])
		classif := strings.TrimSpace(record[1])
		desc := strings.TrimSpace(record[2])
		key := svc.normalizeText(desc)

		if key == "" {
			continue
		}

		entry := domain.ContaSicredi{Code: code, Classif: classif, Desc: desc}
		contasEntries[key] = append(contasEntries[key], entry)

		if !keysMap[key] {
			keysMap[key] = true
			allKeys = append(allKeys, key)
		}
	}
	return contasEntries, allKeys, nil
}

func (svc *service) carregarLancamentos(lancamentosFile io.Reader) ([]domain.Lancamento, error) {
	decoder := charmap.ISO8859_1.NewDecoder()
	reader := csv.NewReader(transform.NewReader(lancamentosFile, decoder))
	reader.Comma = ';'
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1

	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	var lancamentos []domain.Lancamento
	for i, record := range records {
		if len(record) < 9 || !strings.HasPrefix(strings.ToUpper(record[0]), "SIMPLES") {
			fmt.Printf("Linha %d de lançamentos ignorada: %+v\n", i+1, record)
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

func (svc *service) montarOutputSicredi(lancamentos []domain.Lancamento, contasEntries map[string][]domain.ContaSicredi, allKeys []string, classPrefixes []string) []domain.OutputRow {
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
			svc.processarGrupoSicredi(group, &finalRows, contasEntries, allKeys, classPrefixes)
			group = []domain.Lancamento{l}
			currentDate = l.DataLiquidacao
		}
	}
	svc.processarGrupoSicredi(group, &finalRows, contasEntries, allKeys, classPrefixes)

	return finalRows
}

func (svc *service) processarGrupoSicredi(grupo []domain.Lancamento, finalRows *[]domain.OutputRow, contasEntries map[string][]domain.ContaSicredi, allKeys []string, classPrefixes []string) {
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
		Historico:    "TÍTULOS RECEBIDOS NA DATA",
	})

	for _, l := range grupo {
		codigoConta, _, _, _ := svc.matchContaSicredi(l.Descricao, contasEntries, allKeys, classPrefixes)

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

// **NOVA LÓGICA DE MATCH PARA SICREDI**
func (svc *service) matchContaSicredi(descricao string, contasEntries map[string][]domain.ContaSicredi, allKeys []string, classPrefixes []string) (code, matchedKey, matchedClass, mtype string) {
	key := svc.normalizeText(descricao)
	if key == "" {
		return "99999999", "", "", "nao_aplicavel"
	}

	// Define o escopo da busca: filtrado ou global
	searchEntries := contasEntries
	searchKeys := allKeys
	mtypeSuffix := "_all"

	if len(classPrefixes) > 0 {
		searchEntries = make(map[string][]domain.ContaSicredi)
		var filteredKeys []string
		keysMap := make(map[string]bool)

		for k, entries := range contasEntries {
			var matchingEntries []domain.ContaSicredi
			for _, entry := range entries {
				for _, p := range classPrefixes {
					if strings.HasPrefix(entry.Classif, p) {
						matchingEntries = append(matchingEntries, entry)
						break // Evita adicionar a mesma entrada múltiplas vezes
					}
				}
			}
			if len(matchingEntries) > 0 {
				searchEntries[k] = matchingEntries
				if !keysMap[k] {
					keysMap[k] = true
					filteredKeys = append(filteredKeys, k)
				}
			}
		}
		searchKeys = filteredKeys
		mtypeSuffix = "_filtered"
	}

	// 1. Busca por correspondência exata no escopo definido
	if entries, ok := searchEntries[key]; ok && len(entries) > 0 {
		sort.Slice(entries, func(i, j int) bool { return len(entries[i].Classif) > len(entries[j].Classif) })
		chosen := entries[0]
		return chosen.Code, key, chosen.Classif, "exata" + mtypeSuffix
	}

	// 2. Busca por proximidade no escopo definido
	if len(searchKeys) > 0 {
		cm := closestmatch.New(searchKeys, []int{3, 4})
		match := cm.Closest(key)
		if match != "" {
			entries := searchEntries[match]
			if len(entries) > 0 {
				sort.Slice(entries, func(i, j int) bool { return len(entries[i].Classif) > len(entries[j].Classif) })
				chosen := entries[0]
				return chosen.Code, match, chosen.Classif, "fuzzy" + mtypeSuffix
			}
		}
	}

	return "99999999", "", "", "nao_encontrada"
}

func (svc *service) gerarCSVSicredi(rows []domain.OutputRow) ([]byte, error) {
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
	return buffer.Bytes(), writer.Error()
}

// #############################################################################
// #                            CONVERSOR RECEITAS ACISA                         #
// #############################################################################

func (svc *service) ProcessReceitasAcisaFiles(excelFile io.Reader, contasFile io.Reader, excelFilename string, classPrefixes []string) ([]byte, error) {
	contasEntries, allKeys, err := svc.loadContasReceitasAcisa(contasFile)
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar arquivo de contas: %w", err)
	}

	excelData, err := svc.loadAndPrepareExcelReceitas(excelFile)
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar e preparar arquivo excel: %w", err)
	}

	var finalRows []domain.ReceitasAcisaOutputRow
	for _, row := range excelData {
		empresa := row["Empresa"]
		refMes := row["RefMes"]
		mensalidadeRaw := row["Mensalidade"]
		pisRaw := row["Pis"]

		code, matchedKey, _, _ := svc.matchContaReceitas(empresa, contasEntries, allKeys, classPrefixes)

		var descricao string
		if entries, ok := contasEntries[matchedKey]; ok {
			var chosenDesc string
			// A escolha da descrição também deve respeitar o filtro
			filteredEntries := entries
			if len(classPrefixes) > 0 {
				var tempFiltered []domain.ContaReceitasAcisa
				for _, e := range entries {
					for _, p := range classPrefixes {
						if strings.HasPrefix(e.Classif, p) {
							tempFiltered = append(tempFiltered, e)
							break
						}
					}
				}
				filteredEntries = tempFiltered
			}

			if len(filteredEntries) > 0 {
				for _, e := range filteredEntries {
					if e.Code == code {
						chosenDesc = e.Desc
						break
					}
				}
				if chosenDesc == "" {
					chosenDesc = filteredEntries[0].Desc
				}
			} else if len(entries) > 0 { // Fallback (não deveria ser necessário)
				chosenDesc = entries[0].Desc
			}
			descricao = chosenDesc
		} else {
			descricao = empresa
		}

		mensalVal, _ := svc.parseBRLNumber(mensalidadeRaw)
		pisVal, _ := svc.parseBRLNumber(pisRaw)

		finalRows = append(finalRows, domain.ReceitasAcisaOutputRow{
			Data:        refMes,
			Descricao:   descricao,
			Conta:       code,
			Mensalidade: svc.formatTwoDecimalsComma(mensalVal),
			Pis:         svc.formatTwoDecimalsComma(pisVal),
			Historico:   fmt.Sprintf("%s da competencia %s", descricao, refMes),
		})
	}

	return svc.gerarCSVReceitasAcisa(finalRows)
}

func (svc *service) loadContasReceitasAcisa(contasFile io.Reader) (map[string][]domain.ContaReceitasAcisa, []string, error) {
	decoder := charmap.ISO8859_1.NewDecoder()
	reader := csv.NewReader(transform.NewReader(contasFile, decoder))
	reader.Comma = ';'
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1

	records, err := reader.ReadAll()
	if err != nil {
		return nil, nil, err
	}

	contasEntries := make(map[string][]domain.ContaReceitasAcisa)
	var allKeys []string
	keysMap := make(map[string]bool)

	for _, record := range records {
		if len(record) < 3 {
			continue
		}
		code := strings.TrimSpace(record[0])
		classif := strings.TrimSpace(record[1])
		desc := strings.TrimSpace(record[2])
		key := svc.normalizeText(desc)

		if key == "" {
			continue
		}

		entry := domain.ContaReceitasAcisa{Code: code, Classif: classif, Desc: desc}
		contasEntries[key] = append(contasEntries[key], entry)

		if !keysMap[key] {
			keysMap[key] = true
			allKeys = append(allKeys, key)
		}
	}
	return contasEntries, allKeys, nil
}

func (svc *service) findHeaderRowReceitas(rows [][]string) int {
	maxRowsSearch := 40
	if len(rows) < maxRowsSearch {
		maxRowsSearch = len(rows)
	}
	for i := 0; i < maxRowsSearch; i++ {
		for _, cell := range rows[i] {
			if strings.Contains(svc.normalizeText(cell), "EMPRESA") {
				return i
			}
		}
	}
	return 0
}

func (svc *service) pickBestColumnReceitas(header []string, keywords []string) int {
	normCols := make([]string, len(header))
	for i, h := range header {
		normCols[i] = svc.normalizeText(h)
	}
	for _, kw := range keywords {
		nkw := svc.normalizeText(kw)
		for idx, nc := range normCols {
			if strings.Contains(nc, nkw) {
				return idx
			}
		}
	}
	return -1
}

func (svc *service) loadAndPrepareExcelReceitas(excelFile io.Reader) ([]map[string]string, error) {
	f, err := excelize.OpenReader(excelFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sheetName := f.GetSheetList()[0]
	rows, err := f.GetRows(sheetName)
	if err != nil {
		return nil, err
	}

	headerRowIndex := svc.findHeaderRowReceitas(rows)
	header := rows[headerRowIndex]

	empresaKw := []string{"EMPRESA", "NOME", "RAZAO", "FAVORECIDO"}
	refmesKw := []string{"REF", "REF. MÊS", "REF MES", "REFERENCIA"}
	mensalKw := []string{"MENSAL", "MENSALID", "MENSALIDADE", "VALOR"}
	pisKw := []string{"PIS", "P.IS"}

	idxEmpresa := svc.pickBestColumnReceitas(header, empresaKw)
	idxRefmes := svc.pickBestColumnReceitas(header, refmesKw)
	idxMensal := svc.pickBestColumnReceitas(header, mensalKw)
	idxPis := svc.pickBestColumnReceitas(header, pisKw)

	if idxEmpresa == -1 {
		return nil, fmt.Errorf("coluna 'Empresa' não encontrada no Excel")
	}

	var data []map[string]string
	for i := headerRowIndex + 1; i < len(rows); i++ {
		row := rows[i]

		getValue := func(idx int) string {
			if idx != -1 && idx < len(row) {
				return row[idx]
			}
			return ""
		}

		empresa := getValue(idxEmpresa)
		empresaUpper := strings.ToUpper(empresa)
		if strings.TrimSpace(empresa) == "" || strings.Contains(empresaUpper, "TOTAL") || strings.Contains(empresaUpper, "TOTAIS") {
			continue
		}

		dataRow := map[string]string{
			"Empresa":     empresa,
			"RefMes":      getValue(idxRefmes),
			"Mensalidade": getValue(idxMensal),
			"Pis":         getValue(idxPis),
		}
		data = append(data, dataRow)
	}

	return data, nil
}

// **NOVA LÓGICA DE MATCH PARA RECEITAS ACISA**
func (svc *service) matchContaReceitas(descricao string, contasEntries map[string][]domain.ContaReceitasAcisa, allKeys []string, classPrefixes []string) (code, matchedKey, matchedClass, mtype string) {
	key := svc.normalizeText(descricao)
	if key == "" {
		return "99999999", "", "", "nao_aplicavel"
	}

	// Define o escopo da busca: filtrado ou global
	searchEntries := contasEntries
	searchKeys := allKeys
	mtypeSuffix := "_all"

	if len(classPrefixes) > 0 {
		searchEntries = make(map[string][]domain.ContaReceitasAcisa)
		var filteredKeys []string
		keysMap := make(map[string]bool)

		for k, entries := range contasEntries {
			var matchingEntries []domain.ContaReceitasAcisa
			for _, entry := range entries {
				for _, p := range classPrefixes {
					if strings.HasPrefix(entry.Classif, p) {
						matchingEntries = append(matchingEntries, entry)
						break
					}
				}
			}
			if len(matchingEntries) > 0 {
				searchEntries[k] = matchingEntries
				if !keysMap[k] {
					keysMap[k] = true
					filteredKeys = append(filteredKeys, k)
				}
			}
		}
		searchKeys = filteredKeys
		mtypeSuffix = "_filtered"
	}

	// 1. Busca por correspondência exata no escopo definido
	if entries, ok := searchEntries[key]; ok && len(entries) > 0 {
		sort.Slice(entries, func(i, j int) bool { return len(entries[i].Classif) > len(entries[j].Classif) })
		chosen := entries[0]
		return chosen.Code, key, chosen.Classif, "exata" + mtypeSuffix
	}

	// 2. Busca por proximidade no escopo definido
	if len(searchKeys) > 0 {
		cm := closestmatch.New(searchKeys, []int{4, 5, 6})
		match := cm.Closest(key)
		if match != "" {
			entries := searchEntries[match]
			if len(entries) > 0 {
				sort.Slice(entries, func(i, j int) bool { return len(entries[i].Classif) > len(entries[j].Classif) })
				chosen := entries[0]
				return chosen.Code, match, chosen.Classif, "fuzzy" + mtypeSuffix
			}
		}
	}

	return "99999999", "", "", "nao_encontrada"
}

func (svc *service) gerarCSVReceitasAcisa(rows []domain.ReceitasAcisaOutputRow) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := charmap.Windows1252.NewEncoder()
	writer := csv.NewWriter(transform.NewWriter(&buffer, encoder))
	writer.Comma = ';'

	header := []string{"Data", "Descrição", "Conta", "Mensalidade", "Pis", "Histórico"}
	if err := writer.Write(header); err != nil {
		return nil, err
	}

	for _, row := range rows {
		record := []string{row.Data, row.Descricao, row.Conta, row.Mensalidade, row.Pis, row.Historico}
		if err := writer.Write(record); err != nil {
			return nil, err
		}
	}

	writer.Flush()
	return buffer.Bytes(), writer.Error()
}
