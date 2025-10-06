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
	"unicode/utf8"

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
	ProcessAtoliniPagamentos(excelFile io.Reader, contasFile io.Reader, debitPrefixes []string, creditPrefixes []string) ([]byte, error)
	ProcessAtoliniRecebimentos(excelFile io.Reader, contasFile io.Reader, debitPrefixes []string, creditPrefixes []string) ([]byte, error)
}

type service struct{}

// NewService cria uma nova instância do serviço de conversão.
func NewService() Service {
	return &service{}
}

// ---------------------- utilitários comuns ----------------------

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

// sanitizeForCSV remove/controla caracteres de controle e retorna string "limpa"
// - remove tabs, newlines embutidos, converte controles para espaço e trim
func sanitizeForCSV(s string) string {
	if s == "" {
		return ""
	}

	start := 0
	end := len(s)

	for start < end {
		r, size := utf8.DecodeRuneInString(s[start:end])
		if !unicode.IsSpace(r) {
			break
		}
		start += size
	}
	for end > start {
		r, size := utf8.DecodeLastRuneInString(s[start:end])
		if !unicode.IsSpace(r) {
			break
		}
		end -= size
	}
	if start >= end {
		return ""
	}

	var b strings.Builder
	b.Grow(end - start)

	for i := start; i < end; {
		r, size := utf8.DecodeRuneInString(s[i:end])
		i += size

		if r == '\r' || r == '\n' || r == '\t' {
			continue
		}
		if r < 32 {
			b.WriteByte(' ')
			continue
		}
		b.WriteRune(r)
	}

	return b.String()
}

// parseBRLNumber: heurística robusta para entradas brasileiras/anglo
func (svc *service) parseBRLNumber(val string) (float64, error) {
	s := strings.TrimSpace(val)
	if s == "" {
		return 0.0, nil
	}
	s = strings.ReplaceAll(s, "R$", "")
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "\u00a0", "")
	s = strings.TrimSpace(s)
	if s == "" {
		return 0.0, nil
	}

	// tratar sinais/parenteses
	neg := false
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		neg = true
		s = strings.TrimPrefix(strings.TrimSuffix(s, ")"), "(")
	}
	if strings.HasPrefix(s, "-") {
		neg = true
		s = strings.TrimPrefix(s, "-")
	}

	// localizar última ocorrência de . e , para decidir formato
	lastDot := strings.LastIndex(s, ".")
	lastComma := strings.LastIndex(s, ",")

	if lastComma > lastDot {
		s = strings.ReplaceAll(s, ".", "")
		s = strings.ReplaceAll(s, ",", ".")
	} else if lastDot > lastComma {
		if strings.Count(s, ".") > 1 {
			parts := strings.Split(s, ".")
			decimalPart := parts[len(parts)-1]
			intPart := strings.Join(parts[:len(parts)-1], "")
			s = intPart + "." + decimalPart
		}
	} else {
		s = strings.ReplaceAll(s, ".", "")
		s = strings.ReplaceAll(s, ",", ".")
	}

	// remover quaisquer caracteres que não sejam dígitos ou ponto residual
	filtered := []rune{}
	for _, r := range s {
		if (r >= '0' && r <= '9') || r == '.' {
			filtered = append(filtered, r)
		}
	}
	s = string(filtered)
	if s == "" {
		return 0.0, nil
	}

	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0.0, err
	}
	if neg {
		f = -f
	}
	// arredondamento simples (2 decimais)
	return mathRound(f, 2), nil
}

func mathRound(val float64, precision int) float64 {
	pow := 1.0
	for i := 0; i < precision; i++ {
		pow *= 10
	}
	if val >= 0 {
		return float64(int64(val*pow+0.5)) / pow
	}
	return float64(int64(val*pow-0.5)) / pow
}

func (svc *service) formatTwoDecimalsComma(val float64) string {
	return strings.Replace(fmt.Sprintf("%.2f", val), ".", ",", 1)
}

// ---------------------- conversores Excel/CSV ----------------------

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
		// talvez seja xlsx lido como bytes; tentar excelize
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

func (svc *service) loadGenericExcel(file io.Reader) ([][]string, error) {
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	reader := bytes.NewReader(data)

	// tenta xlsx
	f, err := excelize.OpenReader(reader)
	if err == nil {
		defer f.Close()
		sheetName := f.GetSheetList()[0]
		return f.GetRows(sheetName)
	}

	// tenta xls
	reader.Seek(0, io.SeekStart)
	workbook, err := xls.OpenReader(reader)
	if err == nil {
		if len(workbook.GetSheets()) > 0 {
			sheet, err := workbook.GetSheet(0)
			if err != nil {
				return nil, fmt.Errorf("erro ao obter planilha do arquivo .xls: %w", err)
			}
			var allRows [][]string
			for _, row := range sheet.GetRows() {
				var csvRow []string
				for _, cell := range row.GetCols() {
					csvRow = append(csvRow, cell.GetString())
				}
				allRows = append(allRows, csvRow)
			}
			return allRows, nil
		}
		return nil, fmt.Errorf("o arquivo .xls não contém planilhas")
	}

	return nil, fmt.Errorf("unsupported workbook file format")
}

// ---------------------- SICREDI (mantido) ----------------------

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
		ContaCredito: "999999",
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

func (svc *service) matchContaSicredi(descricao string, contasEntries map[string][]domain.ContaSicredi, allKeys []string, classPrefixes []string) (code, matchedKey, matchedClass, mtype string) {
	key := svc.normalizeText(descricao)
	if key == "" {
		return "999999", "", "", "nao_aplicavel"
	}

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

	if entries, ok := searchEntries[key]; ok && len(entries) > 0 {
		sort.Slice(entries, func(i, j int) bool { return len(entries[i].Classif) > len(entries[j].Classif) })
		chosen := entries[0]
		return chosen.Code, key, chosen.Classif, "exata" + mtypeSuffix
	}

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

	return "999999", "", "", "nao_encontrada"
}

func (svc *service) gerarCSVSicredi(rows []domain.OutputRow) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := charmap.Windows1252.NewEncoder() // manter cp1252 para compatibilidade com LançamentosFinal.csv
	writer := csv.NewWriter(transform.NewWriter(&buffer, encoder))
	writer.Comma = ';'

	header := []string{"Operação", "Data", "Descrição Credito", "Conta Credito", "Valor", "Historico"}
	for i := range header {
		header[i] = sanitizeForCSV(header[i])
	}
	if err := writer.Write(header); err != nil {
		return nil, err
	}

	for _, row := range rows {
		record := []string{
			sanitizeForCSV(row.Operacao),
			sanitizeForCSV(row.Data),
			sanitizeForCSV(row.DescricaoCredito),
			sanitizeForCSV(row.ContaCredito),
			sanitizeForCSV(row.Valor),
			sanitizeForCSV(row.Historico),
		}
		if err := writer.Write(record); err != nil {
			return nil, err
		}
	}

	writer.Flush()
	return buffer.Bytes(), writer.Error()
}

// ---------------------- RECEITAS ACISA (mantido) ----------------------

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
			} else if len(entries) > 0 {
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

func (svc *service) matchContaReceitas(descricao string, contasEntries map[string][]domain.ContaReceitasAcisa, allKeys []string, classPrefixes []string) (code, matchedKey, matchedClass, mtype string) {
	key := svc.normalizeText(descricao)
	if key == "" {
		return "999999", "", "", "nao_aplicavel"
	}

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

	if entries, ok := searchEntries[key]; ok && len(entries) > 0 {
		sort.Slice(entries, func(i, j int) bool { return len(entries[i].Classif) > len(entries[j].Classif) })
		chosen := entries[0]
		return chosen.Code, key, chosen.Classif, "exata" + mtypeSuffix
	}

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

	return "999999", "", "", "nao_encontrada"
}

func (svc *service) gerarCSVReceitasAcisa(rows []domain.ReceitasAcisaOutputRow) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := charmap.Windows1252.NewEncoder()
	writer := csv.NewWriter(transform.NewWriter(&buffer, encoder))
	writer.Comma = ';'

	header := []string{"Data", "Descrição", "Conta", "Mensalidade", "Pis", "Histórico"}
	for i := range header {
		header[i] = sanitizeForCSV(header[i])
	}
	if err := writer.Write(header); err != nil {
		return nil, err
	}

	for _, row := range rows {
		record := []string{
			sanitizeForCSV(row.Data),
			sanitizeForCSV(row.Descricao),
			sanitizeForCSV(row.Conta),
			sanitizeForCSV(row.Mensalidade),
			sanitizeForCSV(row.Pis),
			sanitizeForCSV(row.Historico),
		}
		if err := writer.Write(record); err != nil {
			return nil, err
		}
	}

	writer.Flush()
	return buffer.Bytes(), writer.Error()
}

// ---------------------- ATOLINI - PAGAMENTOS (corrigido) ----------------------

// accEntry para plano de contas Atolini
type accEntry struct {
	ID      string
	Classif string
	Desc    string
}

// lerPlanoContasAtolini agora mantém todas as entradas por descrição (descNorm -> []accEntry)
// e retorna a ordem das chaves (descricaoIndex) para fuzzy.
func (svc *service) lerPlanoContasAtolini(contasFile io.Reader) (map[string][]accEntry, []string, error) {
	decoder := charmap.ISO8859_1.NewDecoder()
	reader := csv.NewReader(transform.NewReader(contasFile, decoder))
	reader.Comma = ';'
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1

	records, err := reader.ReadAll()
	if err != nil {
		return nil, nil, err
	}

	byDesc := make(map[string][]accEntry)
	order := []string{}
	seen := map[string]bool{}

	for _, rec := range records {
		if len(rec) < 3 {
			continue
		}
		rawID := strings.TrimSpace(rec[0])
		classif := strings.TrimSpace(rec[1])
		desc := strings.TrimSpace(rec[2])

		if desc == "" || rawID == "" {
			continue
		}

		// garantir que rawID representa algo numérico/código válido
		idForParse := strings.ReplaceAll(rawID, ".", "")
		idForParse = strings.ReplaceAll(idForParse, ",", ".")
		if idForParse == "" {
			continue
		}
		if _, perr := strconv.ParseFloat(idForParse, 64); perr != nil {
			// se o ID não for numérico, ainda podemos aceitar, mas normalmente pulamos
			// mantemos o continue para evitar lixo
			continue
		}

		id := strings.TrimSuffix(rawID, ".0")
		key := svc.normalizeText(desc)
		if key == "" {
			continue
		}
		byDesc[key] = append(byDesc[key], accEntry{
			ID:      id,
			Classif: classif,
			Desc:    desc,
		})
		if !seen[key] {
			seen[key] = true
			order = append(order, key)
		}
	}

	// opcional: ordenar listas por especificidade da classif (maior comprimento primeiro)
	for k := range byDesc {
		list := byDesc[k]
		sort.SliceStable(list, func(i, j int) bool {
			return len(list[i].Classif) > len(list[j].Classif)
		})
		byDesc[k] = list
	}

	return byDesc, order, nil
}

// buscarContaAtolini agora aceita filtros de classPrefixes.
// retorna o código da conta ou "999999".
func (svc *service) buscarContaAtolini(texto string, contasMap map[string][]accEntry, descricaoIndex []string, classPrefixes []string) string {
	t := strings.TrimSpace(texto)
	if t == "" {
		return "999999"
	}
	descNorm := svc.normalizeText(t)
	if descNorm == "" {
		return "999999"
	}
	altNorm := stripLeadingNumberPrefix(descNorm)

	// helper: pick best entry from slice applying classPrefixes filter (prefers longest classif)
	pickBest := func(entries []accEntry, prefixes []string) (accEntry, bool) {
		candidates := entries
		if len(prefixes) > 0 {
			var filtered []accEntry
			for _, e := range entries {
				for _, p := range prefixes {
					if strings.HasPrefix(e.Classif, p) {
						filtered = append(filtered, e)
						break
					}
				}
			}
			if len(filtered) > 0 {
				candidates = filtered
			} else {
				// if filtering left none, return false to indicate no suitable candidate
				return accEntry{}, false
			}
		}
		if len(candidates) == 0 {
			return accEntry{}, false
		}
		sort.SliceStable(candidates, func(i, j int) bool {
			return len(candidates[i].Classif) > len(candidates[j].Classif)
		})
		return candidates[0], true
	}

	tryKey := func(key string) (string, bool) {
		if key == "" {
			return "", false
		}
		if entries, ok := contasMap[key]; ok && len(entries) > 0 {
			if be, ok2 := pickBest(entries, classPrefixes); ok2 {
				return strings.TrimSpace(be.ID), true
			}
		}
		return "", false
	}

	// 1) exato
	if code, ok := tryKey(descNorm); ok {
		return code
	}
	if altNorm != descNorm {
		if code, ok := tryKey(altNorm); ok {
			return code
		}
	}

	// 2) fuzzy: construir candidateKeys aplicando filtro por classPrefixes (se houver)
	candidateKeys := descricaoIndex
	if len(classPrefixes) > 0 {
		var filteredKeys []string
		for _, k := range descricaoIndex {
			entries := contasMap[k]
			for _, e := range entries {
				for _, p := range classPrefixes {
					if strings.HasPrefix(e.Classif, p) {
						filteredKeys = append(filteredKeys, k)
						goto nextK
					}
				}
			}
		nextK:
		}
		if len(filteredKeys) > 0 {
			candidateKeys = filteredKeys
		} else {
			// se nenhum chave passou pelo filtro, não fazemos fuzzy entre todos para evitar escolhas fora do filtro
			// portanto retornamos fallback
			return "999999"
		}
	}

	if len(candidateKeys) > 0 {
		cm := closestmatch.New(candidateKeys, []int{3, 4, 5})
		if match := cm.Closest(descNorm); match != "" {
			if code, ok := tryKey(match); ok {
				return code
			}
		}
		if altNorm != descNorm {
			if matchAlt := cm.Closest(altNorm); matchAlt != "" {
				if code, ok := tryKey(matchAlt); ok {
					return code
				}
			}
		}
	}

	return "999999"
}

// ---------------------- ATOLINI - UTILITÁRIOS DE DATA E NF ----------------------

// findDateInRow: tenta reconhecer datas na linha. Para evitar interpretações erradas de números
// como datas do Excel, restringimos o intervalo de serial aceito.
// Aceitamos serial Excel entre 35000 (≈1995) e 47000 (≈2028) — evita anos estranhos.
func (svc *service) findDateInRow(row []string) (string, bool) {
	dateRegex1 := regexp.MustCompile(`\b\d{2}/\d{2}/\d{4}\b`)
	dateRegex2 := regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}\b`)
	for _, c := range row {
		clean := strings.TrimSpace(c)
		if clean == "" {
			continue
		}
		if m := dateRegex1.FindString(clean); m != "" {
			return m, true
		}
		if m := dateRegex2.FindString(clean); m != "" {
			if t, err := time.Parse("2006-01-02", m); err == nil {
				return t.Format("02/01/2006"), true
			}
		}
		if f, err := strconv.ParseFloat(clean, 64); err == nil {
			// aplicar intervalo plausível para serial Excel
			if f > 35000 && f < 47000 {
				t := excelSerialToDate(f)
				return t.Format("02/01/2006"), true
			}
		}
	}
	return "", false
}

func excelSerialToDate(serial float64) time.Time {
	// base Excel serial -> 1899-12-30
	base := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
	frac := serial - float64(int64(serial))
	duration := time.Duration(int64(serial)*24) * time.Hour
	duration += time.Duration(frac * 24 * float64(time.Hour))
	return base.Add(duration)
}

func (svc *service) findDateInPreviousRows(sheet [][]string, idx int, lookback int) (string, bool) {
	start := idx - 1
	end := idx - lookback
	if end < 0 {
		end = 0
	}
	for i := start; i >= end; i-- {
		row := sheet[i]
		if len(row) > 0 {
			maxScan := len(row)
			if maxScan > 6 {
				maxScan = 6
			}
			for col := 0; col < maxScan; col++ {
				cell := strings.TrimSpace(row[col])
				if cell == "" {
					continue
				}
				lower := strings.ToLower(cell)
				if strings.Contains(lower, "data de pagamento") || strings.Contains(lower, "data do pagamento") || strings.Contains(lower, "data pagamento") || strings.Contains(lower, "data de receb") || strings.Contains(lower, "data do receb") || strings.Contains(lower, "data receb") || strings.Contains(lower, "dt receb") || strings.Contains(lower, "data baixa") {
					if idx := strings.Index(cell, ":"); idx != -1 && idx+1 < len(cell) {
						if d, ok := svc.parseDateDayFirst(strings.TrimSpace(cell[idx+1:])); ok {
							return d, true
						}
					}
					for _, offset := range []int{col + 1, col + 2, col + 3} {
						if offset >= len(row) {
							continue
						}
						if d, ok := svc.parseDateDayFirst(strings.TrimSpace(row[offset])); ok {
							return d, true
						}
					}
					if d, ok := svc.findDateInRow(row); ok {
						return d, true
					}
				}
			}
		}
		if d, ok := svc.findDateInRow(row); ok {
			return d, true
		}
	}
	return "", false
}

// loadAtoliniData encapsula a lógica comum de leitura do plano de contas e do
// arquivo Excel de lançamentos. O carregador de contas é passado como função
// para permitir reutilização tanto em pagamentos quanto em recebimentos.
func loadAtoliniData[T1 any, T2 any](
	svc *service,
	excelFile io.Reader,
	contasFile io.Reader,
	contasLoader func(io.Reader) (T1, T2, error),
) (T1, T2, [][]string, error) {
	var zeroT1 T1
	var zeroT2 T2

	contasMap, descricaoIndex, err := contasLoader(contasFile)
	if err != nil {
		return zeroT1, zeroT2, nil, fmt.Errorf("erro ao carregar arquivo de contas: %w", err)
	}

	rows, err := svc.loadGenericExcel(excelFile)
	if err != nil {
		return zeroT1, zeroT2, nil, fmt.Errorf("erro ao carregar arquivo de lançamentos: %w", err)
	}

	return contasMap, descricaoIndex, rows, nil
}

// ---------------------- ATOLINI - PAGAMENTOS (processamento) ----------------------

// Ajustado para usar lerPlanoContasAtolini (mapa detalhado) e aplicar filtros: debitPrefixes / creditPrefixes.
func (svc *service) ProcessAtoliniPagamentos(
	excelFile io.Reader,
	contasFile io.Reader,
	debitPrefixes []string,
	creditPrefixes []string,
) ([]byte, error) {
	contasMap, descricaoIndex, rows, err := loadAtoliniData(svc, excelFile, contasFile, svc.lerPlanoContasAtolini)
	if err != nil {
		return nil, err
	}

	out := make([]domain.AtoliniPagamentosOutputRow, 0, len(rows))
	var blockDate string
	var blockDateSanitized string
	inHistorico := false

	// ---------- caches p/ evitar fuzzy match repetido ----------
	// chave = UPPER(desc) + "|" + strings.Join(prefixos, ",")
	debCache := make(map[string]string, 256)
	credCache := make(map[string]string, 64)

	debitKeySuffix := strings.Join(debitPrefixes, ",")
	creditKeySuffix := strings.Join(creditPrefixes, ",")

	buildCacheKey := func(upperDesc string, suffix string) string {
		if suffix == "" {
			return upperDesc
		}
		var b strings.Builder
		b.Grow(len(upperDesc) + 1 + len(suffix))
		b.WriteString(upperDesc)
		b.WriteByte('|')
		b.WriteString(suffix)
		return b.String()
	}

	// ---------- helpers leves ----------
	var (
		trimCache  []string
		lowerCache []string
		upperCache []string
		trimGen    []uint64
		lowerGen   []uint64
		upperGen   []uint64
		rowGen     uint64
	)

	ensureRowCapacity := func(size int) {
		if len(trimCache) >= size {
			return
		}
		trimCache = make([]string, size)
		lowerCache = make([]string, size)
		upperCache = make([]string, size)
		trimGen = make([]uint64, size)
		lowerGen = make([]uint64, size)
		upperGen = make([]uint64, size)
	}

	advanceRow := func(size int) {
		ensureRowCapacity(size)
		rowGen++
	}

	trimmedCell := func(row []string, idx int) string {
		if idx < 0 || idx >= len(row) {
			return ""
		}
		if trimGen[idx] != rowGen {
			trimCache[idx] = strings.TrimSpace(row[idx])
			trimGen[idx] = rowGen
		}
		return trimCache[idx]
	}

	lowerCell := func(row []string, idx int) string {
		if idx < 0 || idx >= len(row) {
			return ""
		}
		if lowerGen[idx] != rowGen {
			lowerCache[idx] = strings.ToLower(trimmedCell(row, idx))
			lowerGen[idx] = rowGen
		}
		return lowerCache[idx]
	}

	upperCell := func(row []string, idx int) string {
		if idx < 0 || idx >= len(row) {
			return ""
		}
		if upperGen[idx] != rowGen {
			upperCache[idx] = strings.ToUpper(trimmedCell(row, idx))
			upperGen[idx] = rowGen
		}
		return upperCache[idx]
	}

	rowHasPrefixN := func(row []string, n int, prefixes ...string) bool {
		if n > len(row) {
			n = len(row)
		}
		for i := 0; i < n; i++ {
			cell := lowerCell(row, i)
			for _, p := range prefixes {
				if strings.HasPrefix(cell, p) {
					return true
				}
			}
		}
		return false
	}

	// detecta a linha "Data de pag..." e fixa blockDate (preferindo J; senão à direita do rótulo)
	updateBlockDateIfHeader := func(row []string) bool {
		labelCol := -1
		maxScan := 6
		if maxScan > len(row) {
			maxScan = len(row)
		}
		for i := 0; i < maxScan; i++ {
			if strings.Contains(lowerCell(row, i), "data de pag") {
				labelCol = i
				break
			}
		}
		if labelCol == -1 {
			return false
		}
		for _, ci := range [...]int{9, labelCol + 1, labelCol + 2} {
			if d, ok := svc.parseDateDayFirst(trimmedCell(row, ci)); ok {
				blockDate = d
				blockDateSanitized = sanitizeForCSV(d)
				return true
			}
		}
		if d2, ok2 := svc.findDateInRow(row); ok2 {
			blockDate = d2
			blockDateSanitized = sanitizeForCSV(d2)
			return true
		}
		return false
	}

	bankHints := [...]string{"SICRED", "BANCO", "BRADESCO", "ITAU", "SANTAND", "CAIXA", "BB", "CAIXA GERAL", "REBATE", "BAIXA DEVOL"}

	isBankishUpper := func(upper string) bool {
		if upper == "" {
			return false
		}
		for _, h := range bankHints {
			if strings.Contains(upper, h) {
				return true
			}
		}
		return false
	}

	valueColumns := [...]int{8, 10, 11, 12, 9}

	// Valor: prioridade coluna I(8); depois vizinhas e J(9) como último recurso.
	pickValor := func(row []string) (float64, bool) {
		for _, ci := range valueColumns {
			v := trimmedCell(row, ci)
			if v == "" || v == "0,00" {
				continue
			}
			if parsed, err := svc.parseBRLNumber(v); err == nil {
				return parsed, true
			}
		}
		return 0, false
	}

	formatMoney := func(row []string, idx int) string {
		raw := trimmedCell(row, idx)
		if raw == "" {
			return ""
		}
		if parsed, err := svc.parseBRLNumber(raw); err == nil {
			return svc.formatTwoDecimalsComma(parsed)
		}
		return raw
	}

	pickBanco := func(row []string) (string, string) {
		if upper := upperCell(row, 19); isBankishUpper(upper) {
			return trimmedCell(row, 19), upper
		} // T
		for _, c := range [...]int{18, 20, 21} { // S, U, V
			if upper := upperCell(row, c); isBankishUpper(upper) {
				return trimmedCell(row, c), upper
			}
		}
		return "", ""
	}

	extractDoc := func(row []string) string {
		// célula com 3+ dígitos (barato e suficiente p/ fallback)
		for i := range row {
			c := trimmedCell(row, i)
			if len(c) < 3 {
				continue
			}
			digitCount := 0
			for _, r := range c {
				if r >= '0' && r <= '9' {
					digitCount++
					if digitCount >= 3 {
						return c
					}
				}
			}
		}
		return ""
	}

	// ----------------------- único loop O(n) -----------------------
	for _, row := range rows {
		advanceRow(len(row))
		// 1) data do bloco (cabeçalho)
		if updateBlockDateIfHeader(row) {
			continue
		}

		// 2) começo/fim do bloco "Histórico:"
		if rowHasPrefixN(row, 3, "histórico", "historico") {
			inHistorico = true
			continue
		}
		if rowHasPrefixN(row, 3, "total do histórico", "total do historico", "total da data") {
			inHistorico = false
			continue
		}
		if !inHistorico || blockDate == "" {
			continue
		}

		// 3) valor rápido (I -> vizinhas -> J)
		val, ok := pickValor(row)
		if !ok {
			continue
		}

		// 4) descrição (B) + histórico (B + " NF " + D / doc)
		descDeb := trimmedCell(row, 1) // B
		hist := descDeb
		if dcol := trimmedCell(row, 3); dcol != "" { // D
			if descDeb != "" {
				hist = descDeb + " NF " + dcol
			} else {
				hist = " NF " + dcol
			}
		} else if doc := extractDoc(row); doc != "" && descDeb != "" {
			hist = descDeb + " NF " + doc
		}

		// 5) banco (crédito)
		descCred, descCredUpper := pickBanco(row)

		// 6) matching com CACHE
		var debID, credID string

		if descDeb != "" {
			debKey := buildCacheKey(upperCell(row, 1), debitKeySuffix)
			if id, ok := debCache[debKey]; ok {
				debID = id
			} else {
				debID = svc.buscarContaAtolini(descDeb, contasMap, descricaoIndex, debitPrefixes)
				debCache[debKey] = debID
			}
		}

		if descCred != "" {
			credKey := buildCacheKey(descCredUpper, creditKeySuffix)
			if id, ok := credCache[credKey]; ok {
				credID = id
			} else {
				credID = svc.buscarContaAtolini(descCred, contasMap, descricaoIndex, creditPrefixes)
				credCache[credKey] = credID
			}
		}

		out = append(out, domain.AtoliniPagamentosOutputRow{
			Data:              blockDateSanitized,
			Debito:            sanitizeForCSV(debID),
			DescricaoConta:    sanitizeForCSV(descDeb),
			Credito:           sanitizeForCSV(credID),
			DescricaoCredito:  sanitizeForCSV(descCred),
			Valor:             sanitizeForCSV(svc.formatTwoDecimalsComma(val)),
			Historico:         sanitizeForCSV(hist),
			ValorOriginal:     sanitizeForCSV(formatMoney(row, 7)),
			ValorPago:         sanitizeForCSV(formatMoney(row, 8)),
			ValorJuros:        sanitizeForCSV(formatMoney(row, 9)),
			ValorMulta:        sanitizeForCSV(formatMoney(row, 11)),
			ValorDesconto:     sanitizeForCSV(formatMoney(row, 12)),
			ValorDespesas:     sanitizeForCSV(formatMoney(row, 13)),
			VarCam:            sanitizeForCSV(formatMoney(row, 15)),
			ValorLiqPagoBanco: sanitizeForCSV(formatMoney(row, 17)),
		})
	}

	return svc.gerarCSVAtoliniPagamentos(out)
}

func (svc *service) gerarCSVAtoliniPagamentos(rows []domain.AtoliniPagamentosOutputRow) ([]byte, error) {
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	writer.Comma = ';'

	header := []string{"Data", "Débito", "Descrição Débito", "Crédito", "Descrição Crédito", "Valor", "histórico", "Valor Original", "Valor Pago",
		"Valor Juros", "Valor Multa", "Valor Desconto", "Valor Despesas", "Var Cam", "Valor Liq Pago Banco"}
	for i := range header {
		header[i] = sanitizeForCSV(header[i])
	}
	if err := writer.Write(header); err != nil {
		return nil, err
	}

	for _, row := range rows {
		record := []string{
			row.Data,
			row.Debito,
			row.DescricaoConta,
			row.Credito,
			row.DescricaoCredito,
			row.Valor,
			row.Historico,
			row.ValorOriginal,
			row.ValorPago,
			row.ValorJuros,
			row.ValorMulta,
			row.ValorDesconto,
			row.ValorDespesas,
			row.VarCam,
			row.ValorLiqPagoBanco,
		}
		if err := writer.Write(record); err != nil {
			return nil, err
		}
	}

	writer.Flush()
	return buffer.Bytes(), writer.Error()
}

// ---------------------- ATOLINI - RECEBIMENTOS (mantido/refinado) ----------------------

// ContaEntry usado internamente para mapear descrição -> várias entradas (código + classif + desc)
type ContaEntry struct {
	Code   string
	Classf string
	Desc   string
}

// lerContasRecebimentos: lê o CSV de contas (ISO-8859-1) e retorna:
// - uma lista ordenada de descrições normalizadas (descricaoIndex),
// - um mapa de descrição normalizada -> lista de entradas (contasMap)
func (svc *service) lerContasRecebimentos(contasFile io.Reader) ([]string, map[string][]ContaEntry, error) {
	decoder := charmap.ISO8859_1.NewDecoder()
	reader := csv.NewReader(transform.NewReader(contasFile, decoder))
	reader.Comma = ';'
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1

	records, err := reader.ReadAll()
	if err != nil {
		return nil, nil, err
	}

	contasMap := make(map[string][]ContaEntry)
	order := make([]string, 0, len(records))
	seen := make(map[string]bool)

	for _, rec := range records {
		if len(rec) < 3 {
			continue
		}
		code := strings.TrimSpace(rec[0])
		classif := strings.TrimSpace(rec[1])
		desc := strings.TrimSpace(rec[2])

		if code == "" || desc == "" {
			continue
		}

		descNorm := svc.normalizeText(desc)
		if descNorm == "" {
			continue
		}

		contasMap[descNorm] = append(contasMap[descNorm], ContaEntry{
			Code:   code,
			Classf: classif,
			Desc:   desc,
		})
		if !seen[descNorm] {
			seen[descNorm] = true
			order = append(order, descNorm)
		}
	}

	return order, contasMap, nil
}

// findContaCodigoByDescricao: encontra o código da conta dado uma descrição (texto),
// usando correspondência exata ou fuzzy, aplicando filtro por classif (prefixos) quando fornecido.
//
// Retorna o código encontrado ou "999999" como fallback.
func (svc *service) findContaCodigoByDescricao(descricao string, descricaoIndex []string, contasMap map[string][]ContaEntry, classPrefixes []string) string {
	if strings.TrimSpace(descricao) == "" {
		return "999999"
	}
	descNorm := svc.normalizeText(descricao)

	// Helper: seleciona melhor entry da lista, preferindo classif mais longa (mais específica)
	pickBestEntry := func(entries []ContaEntry, prefixes []string) (ContaEntry, bool) {
		// se houver prefixes, filtrar pelas entradas que começam com algum prefixo
		candidates := entries
		if len(prefixes) > 0 {
			var filtered []ContaEntry
			for _, e := range entries {
				for _, p := range prefixes {
					if strings.HasPrefix(e.Classf, p) {
						filtered = append(filtered, e)
						break
					}
				}
			}
			if len(filtered) > 0 {
				candidates = filtered
			} else {
				return ContaEntry{}, false
			}
		}
		if len(candidates) == 0 {
			return ContaEntry{}, false
		}
		// escolher o com classif mais longa (mais específica)
		sort.Slice(candidates, func(i, j int) bool {
			return len(candidates[i].Classf) > len(candidates[j].Classf)
		})
		return candidates[0], true
	}

	// 1) tentar match exato
	if entries, ok := contasMap[descNorm]; ok && len(entries) > 0 {
		if be, ok2 := pickBestEntry(entries, classPrefixes); ok2 {
			return strings.TrimSpace(be.Code)
		}
	}

	// 1.b) tentar sem prefixo numérico (ex: "748 - SICREDI ..." -> "SICREDI ...")
	alt := stripLeadingNumberPrefix(descNorm)
	if alt != descNorm {
		if entries, ok := contasMap[alt]; ok && len(entries) > 0 {
			if be, ok2 := pickBestEntry(entries, classPrefixes); ok2 {
				return strings.TrimSpace(be.Code)
			}
		}
	}

	// 2) se não encontrou exato, fazer fuzzy entre as chaves candidatas
	// construir lista de chaves candidato: se houver classPrefixes, filtrar chaves que têm pelo menos uma entry com classif correspondente
	candidateKeys := descricaoIndex
	if len(classPrefixes) > 0 {
		var filteredKeys []string
		for _, k := range descricaoIndex {
			entries := contasMap[k]
			for _, e := range entries {
				for _, p := range classPrefixes {
					if strings.HasPrefix(e.Classf, p) {
						filteredKeys = append(filteredKeys, k)
						goto nextKey2
					}
				}
			}
		nextKey2:
		}
		if len(filteredKeys) > 0 {
			candidateKeys = filteredKeys
		} else {
			return "999999"
		}
	}

	if len(candidateKeys) > 0 {
		cm := closestmatch.New(candidateKeys, []int{3, 4, 5})
		match := cm.Closest(descNorm)
		if match != "" {
			if entries, ok := contasMap[match]; ok && len(entries) > 0 {
				if be, ok2 := pickBestEntry(entries, classPrefixes); ok2 {
					return strings.TrimSpace(be.Code)
				}
			}
		}
		// tentativa fuzzy no alt (sem prefixo numérico)
		if alt != descNorm {
			match2 := cm.Closest(alt)
			if match2 != "" {
				if entries, ok := contasMap[match2]; ok && len(entries) > 0 {
					if be, ok2 := pickBestEntry(entries, classPrefixes); ok2 {
						return strings.TrimSpace(be.Code)
					}
				}
			}
		}
	}

	// fallback
	return "999999"
}

func (svc *service) parseDateDayFirst(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	if t, err := time.Parse("02/01/2006", s); err == nil {
		return t.Format("02/01/2006"), true
	}
	if len(s) >= 10 {
		if t, err := time.Parse("2006-01-02", s[:10]); err == nil {
			return t.Format("02/01/2006"), true
		}
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		if f > 35000 && f < 47000 {
			return excelSerialToDate(f).Format("02/01/2006"), true
		}
	}
	return "", false
}

var recebimentoLancamentoRegex = regexp.MustCompile(`^\s*\d+\s*-\s+.+$`)
var extractAfterHyphenRegex = regexp.MustCompile(`^\s*\d+\s*-\s*(.*)$`)

func extractAfterHyphen(cell string) string {
	if cell == "" {
		return ""
	}
	if m := extractAfterHyphenRegex.FindStringSubmatch(cell); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func isDataMarker(cell string) bool {
	if cell == "" {
		return false
	}
	return strings.HasPrefix(strings.ToUpper(strings.TrimSpace(cell)), "DATA:")
}

func isPortadorMarker(cell string) bool {
	if cell == "" {
		return false
	}
	u := strings.ToUpper(strings.TrimSpace(cell))
	if strings.Contains(u, "PORTADOR") || strings.Contains(u, "PORTADOR DO PAGAMENTO") || strings.Contains(u, "PORTADOR:") {
		return true
	}
	return false
}

// ** FUNÇÕES AUXILIARES **
func findDataMarkerIndex(row []string) int {
	for i, c := range row {
		if isDataMarker(c) {
			return i
		}
	}
	return -1
}

func findPortadorIndex(row []string) int {
	for i, c := range row {
		if isPortadorMarker(c) {
			return i
		}
	}
	return -1
}

func isLancamento(cell string) bool {
	if cell == "" {
		return false
	}
	return recebimentoLancamentoRegex.MatchString(cell)
}

// cleanPortadorText normaliza o texto de portador removendo partes fixas como
// "Portador do Pagamento" ou ":" finais, preservando porém eventuais
// prefixos numéricos (ex: "7 - BANCO...") para que apareçam no CSV.
func cleanPortadorText(s string) string {
	if s == "" {
		return ""
	}
	s = strings.TrimSpace(s)
	// remover ":" final
	s = strings.TrimRight(s, ":")
	// se começa com PORTADOR, remover essa parte
	u := strings.ToUpper(s)
	if strings.HasPrefix(u, "PORTADOR DO PAGAMENTO") {
		idx := strings.Index(u, "PORTADOR DO PAGAMENTO")
		rest := strings.TrimSpace(s[idx+len("PORTADOR DO PAGAMENTO"):])
		s = rest
		u = strings.ToUpper(s)
	}
	if strings.HasPrefix(u, "PORTADOR") {
		idx := strings.Index(u, "PORTADOR")
		rest := strings.TrimSpace(s[idx+len("PORTADOR"):])
		s = rest
	}
	return strings.TrimSpace(s)
}

// stripLeadingNumberPrefix remove prefixos como "123 - " ou "123- " no início da string normalizada
// Recebe tanto string normal quanto a versao normalizada (por precaucao), e retorna string normalizada se possível.
func stripLeadingNumberPrefix(s string) string {
	if s == "" {
		return s
	}
	// regex para prefixo numérico seguido de - ou :
	prefixRegex := regexp.MustCompile(`^\s*\d+\s*[-:]\s*(.*)$`)
	if m := prefixRegex.FindStringSubmatch(s); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	// caso somente número e espaço: "123 RESTANTE..."
	prefixOnlyNum := regexp.MustCompile(`^\s*\d+\s+(.*)$`)
	if m := prefixOnlyNum.FindStringSubmatch(s); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return s
}

// pickPortadorFromRow tenta extrair o nome do portador a partir de colunas próximas ao marcador.
// Portadores podem iniciar com código numérico ("7 - BANCO ..."), portanto não
// descartamos valores que combinem esse padrão.
func pickPortadorFromRow(row []string, markerIdx int) string {
	// primeiro: se o próprio marcador tem conteúdo após ":" (ex: "Portador do Pagamento: 7 - BANCO ...")
	if markerIdx >= 0 && markerIdx < len(row) {
		valMarker := strings.TrimSpace(row[markerIdx])
		if valMarker != "" {
			// tentar extrair parte após ":" se houver
			if idx := strings.Index(valMarker, ":"); idx != -1 && idx+1 < len(valMarker) {
				part := strings.TrimSpace(valMarker[idx+1:])
				if part != "" && !isPortadorMarker(part) {
					cleaned := cleanPortadorText(part)
					if cleaned != "" {
						return cleaned
					}
				}
			}
		}
	}

	// procurar em ordem: col à direita imediata, mais direita, esquerda imediata, mais esquerda, então colunas 0..12
	searchOrder := []int{}
	for _, delta := range []int{1, 2, -1, -2, 3, -3, 4, -4} {
		searchOrder = append(searchOrder, markerIdx+delta)
	}
	// fallback scanning more columns (0..12)
	for i := 0; i <= 12; i++ {
		searchOrder = append(searchOrder, i)
	}
	seen := map[int]bool{}
	for _, idx := range searchOrder {
		if idx < 0 || idx >= len(row) {
			continue
		}
		if seen[idx] {
			continue
		}
		seen[idx] = true
		val := strings.TrimSpace(row[idx])
		if val == "" {
			continue
		}
		// ignorar se a célula contém o marcador "PORTADOR" (não queremos isso)
		if isPortadorMarker(val) {
			continue
		}
		// Se chegou aqui, é um candidato. limpar e retornar.
		cleaned := cleanPortadorText(val)
		if cleaned == "" {
			continue
		}
		return cleaned
	}
	return ""
}

// findLastPortadorBefore procura, retroativamente a partir de idx, o último portador válido em até lookback linhas.
// Retorna o texto limpo do portador e true se encontrado.
func findLastPortadorBefore(sheet [][]string, idx int, lookback int) (string, bool) {
	start := idx - 1
	end := idx - lookback
	if end < 0 {
		end = 0
	}
	for i := start; i >= end; i-- {
		row := sheet[i]
		if pIdx := findPortadorIndex(row); pIdx != -1 {
			// tentar extrair com pickPortadorFromRow
			if val := pickPortadorFromRow(row, pIdx); val != "" {
				return val, true
			}
			// fallback: procurar célula à direita
			for j := pIdx + 1; j <= pIdx+6 && j < len(row); j++ {
				if row[j] != "" {
					return cleanPortadorText(row[j]), true
				}
			}
		}
		// caso a própria linha contenha um "NNN - NOME DO PORTADOR" sem marcador
		for _, c := range row {
			if m := regexp.MustCompile(`^\s*\d+\s*-\s*.+`).FindString(c); m != "" {
				// interpretar como possível portador (cuidado: pode ser um lançamento)
				// heurística: se a linha tem poucas colunas não vazias (provável header de portador), aceitar
				nonEmpty := 0
				for _, cc := range row {
					if strings.TrimSpace(cc) != "" {
						nonEmpty++
					}
				}
				if nonEmpty <= 6 {
					return cleanPortadorText(m), true
				}
			}
		}
	}
	return "", false
}

func (svc *service) ProcessAtoliniRecebimentos(excelFile io.Reader, contasFile io.Reader, debitPrefixes []string, creditPrefixes []string) ([]byte, error) {
	descricaoIndex, contasMap, rows, err := loadAtoliniData(svc, excelFile, contasFile, svc.lerContasRecebimentos)
	if err != nil {
		return nil, err
	}

	finalRows := make([]domain.AtoliniRecebimentosOutputRow, 0, len(rows))

	var (
		blockDate          string
		blockDateSanitized string
		currentDescDebito  string
		currentCodDebito   = "999999"
	)

	debCache := make(map[string]string, 256)
	credCache := make(map[string]string, 256)
	debitKeySuffix := strings.Join(debitPrefixes, ",")
	creditKeySuffix := strings.Join(creditPrefixes, ",")

	buildCacheKey := func(upperDesc string, suffix string) string {
		if suffix == "" {
			return upperDesc
		}
		var b strings.Builder
		b.Grow(len(upperDesc) + 1 + len(suffix))
		b.WriteString(upperDesc)
		b.WriteByte('|')
		b.WriteString(suffix)
		return b.String()
	}

	setCurrentDebit := func(desc string) {
		desc = strings.TrimSpace(desc)
		if desc == "" {
			currentDescDebito = ""
			currentCodDebito = "999999"
			return
		}
		currentDescDebito = desc
		upper := strings.ToUpper(desc)
		key := buildCacheKey(upper, debitKeySuffix)
		if code, ok := debCache[key]; ok {
			currentCodDebito = code
			return
		}
		code := svc.findContaCodigoByDescricao(desc, descricaoIndex, contasMap, debitPrefixes)
		if code == "" {
			code = "999999"
		}
		debCache[key] = code
		currentCodDebito = code
	}

	var (
		trimCache  []string
		lowerCache []string
		upperCache []string
		trimGen    []uint64
		lowerGen   []uint64
		upperGen   []uint64
		rowGen     uint64
	)

	ensureRowCapacity := func(size int) {
		if len(trimCache) >= size {
			return
		}
		trimCache = make([]string, size)
		lowerCache = make([]string, size)
		upperCache = make([]string, size)
		trimGen = make([]uint64, size)
		lowerGen = make([]uint64, size)
		upperGen = make([]uint64, size)
	}

	advanceRow := func(size int) {
		ensureRowCapacity(size)
		rowGen++
	}

	trimmedCell := func(row []string, idx int) string {
		if idx < 0 || idx >= len(row) {
			return ""
		}
		if trimGen[idx] != rowGen {
			trimCache[idx] = strings.TrimSpace(row[idx])
			trimGen[idx] = rowGen
		}
		return trimCache[idx]
	}

	lowerCell := func(row []string, idx int) string {
		if idx < 0 || idx >= len(row) {
			return ""
		}
		if lowerGen[idx] != rowGen {
			lowerCache[idx] = strings.ToLower(trimmedCell(row, idx))
			lowerGen[idx] = rowGen
		}
		return lowerCache[idx]
	}

	upperCell := func(row []string, idx int) string {
		if idx < 0 || idx >= len(row) {
			return ""
		}
		if upperGen[idx] != rowGen {
			upperCache[idx] = strings.ToUpper(trimmedCell(row, idx))
			upperGen[idx] = rowGen
		}
		return upperCache[idx]
	}

	type columnHints struct {
		doc            int
		historico      int
		valorPrincipal int
		juros          int
		desconto       int
		despBanco      int
		despCartorio   int
		vlLiqPago      int
		banco          int
	}

	hints := columnHints{
		doc:            -1,
		historico:      -1,
		valorPrincipal: -1,
		juros:          -1,
		desconto:       -1,
		despBanco:      -1,
		despCartorio:   -1,
		vlLiqPago:      -1,
		banco:          -1,
	}

	updateColumnHints := func(row []string) bool {
		var (
			docHit       bool
			histHit      bool
			principalHit bool
			jurosHit     bool
			descHit      bool
			despBcoHit   bool
			despCartHit  bool
			vlLiqHit     bool
			bancoHit     bool
		)

		for idx := range row {
			upper := upperCell(row, idx)
			if upper == "" {
				continue
			}

			if !docHit && (strings.Contains(upper, "DOCUMENT") || strings.HasPrefix(upper, "DOC")) {
				docHit = true
				if hints.doc == -1 {
					hints.doc = idx
				}
			}
			if !histHit && strings.Contains(upper, "HIST") {
				histHit = true
				if hints.historico == -1 {
					hints.historico = idx
				}
			}
			if strings.Contains(upper, "VALOR PRINC") || strings.Contains(upper, "VL PRINC") || strings.Contains(upper, "VLR PRINC") {
				principalHit = true
				if hints.valorPrincipal == -1 {
					hints.valorPrincipal = idx
				}
			}
			if strings.Contains(upper, "JUROS") {
				jurosHit = true
				if hints.juros == -1 {
					hints.juros = idx
				}
			}
			if strings.Contains(upper, "DESCONTO") || strings.Contains(upper, "DESC.") {
				descHit = true
				if hints.desconto == -1 {
					hints.desconto = idx
				}
			}
			if strings.Contains(upper, "DESP") && strings.Contains(upper, "BANC") {
				despBcoHit = true
				if hints.despBanco == -1 {
					hints.despBanco = idx
				}
			}
			if strings.Contains(upper, "DESP") && strings.Contains(upper, "CART") {
				despCartHit = true
				if hints.despCartorio == -1 {
					hints.despCartorio = idx
				}
			}
			if strings.Contains(upper, "VL") && strings.Contains(upper, "LIQ") {
				vlLiqHit = true
				if hints.vlLiqPago == -1 {
					hints.vlLiqPago = idx
				}
			}
			if strings.Contains(upper, "BANCO") || strings.Contains(upper, "INSTITU") {
				bancoHit = true
				if hints.banco == -1 {
					hints.banco = idx
				}
			}
		}

		hits := 0
		if docHit {
			hits++
		}
		if histHit {
			hits++
		}
		if principalHit {
			hits++
		}
		if jurosHit {
			hits++
		}
		if descHit {
			hits++
		}
		if despBcoHit {
			hits++
		}
		if despCartHit {
			hits++
		}
		if vlLiqHit {
			hits++
		}
		if bancoHit {
			hits++
		}

		return hits >= 3
	}

	bankHints := []string{"SICRED", "SICOOB", "BANCO", "BRADESCO", "ITAU", "ITAÚ", "SANTAND", "CAIXA", "CEF", "BB", "COOP", "COOPER", "FINANC", "REBATE", "BAIXA DEVOL"}

	isBankish := func(upper string) bool {
		if upper == "" {
			return false
		}
		for _, hint := range bankHints {
			if strings.Contains(upper, hint) {
				return true
			}
		}
		return false
	}

	firstNonEmpty := func(row []string, indices []int) string {
		seen := make(map[int]struct{}, len(indices))
		for _, idx := range indices {
			if idx < 0 || idx >= len(row) {
				continue
			}
			if _, ok := seen[idx]; ok {
				continue
			}
			seen[idx] = struct{}{}
			if val := trimmedCell(row, idx); val != "" {
				return val
			}
		}
		return ""
	}

	parseValueFrom := func(row []string, indices []int) (float64, bool) {
		seen := make(map[int]struct{}, len(indices))
		for _, idx := range indices {
			if idx < 0 || idx >= len(row) {
				continue
			}
			if _, ok := seen[idx]; ok {
				continue
			}
			seen[idx] = struct{}{}
			val := trimmedCell(row, idx)
			if val == "" {
				continue
			}
			if parsed, err := svc.parseBRLNumber(val); err == nil {
				return parsed, true
			}
		}
		return 0, false
	}

	buildCandidates := func(lancIdx int, abs []int, rel []int) []int {
		total := len(abs) + len(rel)
		if total == 0 {
			return nil
		}
		indices := make([]int, 0, total)
		seen := make(map[int]struct{}, total)
		for _, idx := range abs {
			if idx < 0 {
				continue
			}
			if _, ok := seen[idx]; ok {
				continue
			}
			seen[idx] = struct{}{}
			indices = append(indices, idx)
		}
		for _, off := range rel {
			idx := lancIdx + off
			if idx < 0 {
				continue
			}
			if _, ok := seen[idx]; ok {
				continue
			}
			seen[idx] = struct{}{}
			indices = append(indices, idx)
		}
		return indices
	}

	pickDescricaoCredito := func(row []string, lancIdx int) (string, string) {
		base := stripLeadingNumberPrefix(extractAfterHyphen(trimmedCell(row, lancIdx)))
		if base == "" {
			base = stripLeadingNumberPrefix(trimmedCell(row, lancIdx))
		}
		base = strings.TrimSpace(base)
		baseUpper := strings.ToUpper(base)

		if base != "" && isBankish(baseUpper) {
			return base, baseUpper
		}

		fallbackDesc := base
		fallbackUpper := baseUpper

		seen := make(map[int]struct{})

		tryIndex := func(idx int) (string, string, bool) {
			if idx < 0 || idx >= len(row) {
				return "", "", false
			}
			if _, ok := seen[idx]; ok {
				return "", "", false
			}
			seen[idx] = struct{}{}
			raw := trimmedCell(row, idx)
			if raw == "" {
				return "", "", false
			}
			cleaned := strings.TrimSpace(stripLeadingNumberPrefix(raw))
			if cleaned == "" {
				cleaned = raw
			}
			upper := strings.ToUpper(cleaned)
			if isBankish(upper) {
				return cleaned, upper, true
			}
			if fallbackDesc == "" {
				fallbackDesc = cleaned
				fallbackUpper = upper
			}
			return "", "", false
		}

		candidates := []int{hints.banco, lancIdx + 6, lancIdx + 5, lancIdx + 7}
		candidates = append(candidates, []int{18, 19, 20, 21, 22, 17, 16, 15}...)
		for i := len(row) - 1; i >= 0; i-- {
			candidates = append(candidates, i)
		}

		for _, idx := range candidates {
			if desc, upper, ok := tryIndex(idx); ok {
				return desc, upper
			}
		}

		return fallbackDesc, fallbackUpper
	}

	updateBlockDate := func(row []string, lancIdx int) bool {
		nonEmpty := 0
		for i := range row {
			if trimmedCell(row, i) != "" {
				nonEmpty++
			}
		}

		var candidate string

		if dIdx := findDataMarkerIndex(row); dIdx != -1 {
			marker := trimmedCell(row, dIdx)
			if idx := strings.Index(marker, ":"); idx != -1 && idx+1 < len(marker) {
				if d, ok := svc.parseDateDayFirst(strings.TrimSpace(marker[idx+1:])); ok {
					candidate = d
				}
			}
			if candidate == "" {
				if d, ok := svc.parseDateDayFirst(trimmedCell(row, dIdx+1)); ok {
					candidate = d
				}
			}
			if candidate == "" {
				if d, ok := svc.parseDateDayFirst(trimmedCell(row, dIdx+2)); ok {
					candidate = d
				}
			}
			if candidate == "" {
				if d, ok := svc.parseDateDayFirst(trimmedCell(row, 2)); ok {
					candidate = d
				}
			}
			if candidate == "" {
				if d, ok := svc.findDateInRow(row); ok {
					candidate = d
				}
			}
		}

		if candidate == "" {
			for i := 0; i < len(row) && i < 6; i++ {
				lower := lowerCell(row, i)
				if lower == "" {
					continue
				}
				if strings.Contains(lower, "data do receb") || strings.Contains(lower, "data de receb") || strings.Contains(lower, "data receb") || strings.Contains(lower, "dt receb") || (strings.Contains(lower, "data") && strings.Contains(lower, "baix")) {
					if d, ok := svc.findDateInRow([]string{row[i]}); ok {
						candidate = d
						break
					}
					for _, idx := range []int{i + 1, i + 2, i + 3} {
						if candidate != "" {
							break
						}
						if d, ok := svc.parseDateDayFirst(trimmedCell(row, idx)); ok {
							candidate = d
							break
						}
					}
					if candidate == "" {
						if d, ok := svc.findDateInRow(row); ok {
							candidate = d
						}
					}
					if candidate != "" {
						break
					}
				}
			}
		}

		if candidate == "" && lancIdx == -1 && nonEmpty == 1 {
			if d, ok := svc.findDateInRow(row); ok {
				candidate = d
			}
		}

		if candidate != "" {
			blockDate = candidate
			blockDateSanitized = sanitizeForCSV(candidate)
			return lancIdx == -1 && nonEmpty <= 6
		}
		return false
	}

	for rIdx, row := range rows {
		advanceRow(len(row))

		lancIdx := -1
		for i := range row {
			if isLancamento(row[i]) {
				lancIdx = i
				break
			}
		}

		if updateBlockDate(row, lancIdx) {
			continue
		}

		if updateColumnHints(row) {
			continue
		}

		if pIdx := findPortadorIndex(row); pIdx != -1 {
			descDeb := pickPortadorFromRow(row, pIdx)
			if descDeb == "" {
				for j := pIdx + 1; j <= pIdx+6 && j < len(row); j++ {
					candidate := strings.TrimSpace(row[j])
					if candidate == "" {
						continue
					}
					descDeb = cleanPortadorText(candidate)
					if descDeb != "" {
						break
					}
				}
			}
			if strings.TrimSpace(descDeb) == "" {
				currentDescDebito = ""
				currentCodDebito = "999999"
			} else {
				setCurrentDebit(descDeb)
			}
			continue
		}

		if lancIdx == -1 {
			continue
		}

		effectiveDate := blockDate
		effectiveDateSanitized := blockDateSanitized
		if effectiveDate == "" {
			if d, ok := svc.findDateInPreviousRows(rows, rIdx, 60); ok {
				effectiveDate = d
				effectiveDateSanitized = sanitizeForCSV(d)
			}
		}
		if effectiveDate == "" {
			if d, ok := svc.findDateInRow(row); ok {
				effectiveDate = d
				effectiveDateSanitized = sanitizeForCSV(d)
			}
		}

		if currentDescDebito == "" {
			if p, ok := findLastPortadorBefore(rows, rIdx, 60); ok {
				setCurrentDebit(p)
			}
		}

		descCredito, descCreditoUpper := pickDescricaoCredito(row, lancIdx)
		codCredito := "999999"
		if descCredito != "" {
			key := buildCacheKey(descCreditoUpper, creditKeySuffix)
			if cached, ok := credCache[key]; ok {
				codCredito = cached
			} else {
				code := svc.findContaCodigoByDescricao(descCredito, descricaoIndex, contasMap, creditPrefixes)
				if code == "" {
					code = "999999"
				}
				credCache[key] = code
				codCredito = code
			}
		}

		docCandidates := buildCandidates(lancIdx, []int{hints.doc, 4, 5}, []int{4, 5, 3})
		doc := firstNonEmpty(row, docCandidates)

		histCandidates := buildCandidates(lancIdx, []int{hints.historico, 9, 8, 10}, []int{9, 8, 10, 7})
		histBase := firstNonEmpty(row, histCandidates)

		historico := strings.TrimSpace(histBase)
		if historico != "" {
			historico += " "
		}
		historico += "CONFORME DOCUMENTO " + strings.TrimSpace(doc)
		if descCredito != "" {
			historico += " DE " + descCredito
		}
		historico = sanitizeForCSV(strings.TrimSpace(historico))

		principalCandidates := buildCandidates(lancIdx, []int{hints.valorPrincipal, 12, 11, 13}, []int{12, 11, 13, 10})
		jurosCandidates := buildCandidates(lancIdx, []int{hints.juros, 13, 12, 14}, []int{13, 12, 14})
		descontoCandidates := buildCandidates(lancIdx, []int{hints.desconto, 14, 13, 15}, []int{14, 13, 15})
		despBancoCandidates := buildCandidates(lancIdx, []int{hints.despBanco, 15, 14, 16}, []int{15, 14, 16})
		despCartCandidates := buildCandidates(lancIdx, []int{hints.despCartorio, 16, 15, 17}, []int{16, 15, 17})
		liquidoCandidates := buildCandidates(lancIdx, []int{hints.vlLiqPago, 17, 16, 18, 19}, []int{17, 16, 18})

		vPrincipal, _ := parseValueFrom(row, principalCandidates)
		vJuros, _ := parseValueFrom(row, jurosCandidates)
		vDesc, _ := parseValueFrom(row, descontoCandidates)
		vDespBco, _ := parseValueFrom(row, despBancoCandidates)
		vDespCart, _ := parseValueFrom(row, despCartCandidates)
		vVlliq, _ := parseValueFrom(row, liquidoCandidates)

		finalRows = append(finalRows, domain.AtoliniRecebimentosOutputRow{
			Data:             sanitizeForCSV(effectiveDateSanitized),
			DescricaoCredito: sanitizeForCSV(descCredito),
			ContaCredito:     sanitizeForCSV(codCredito),
			DescricaoDebito:  sanitizeForCSV(currentDescDebito),
			ContaDebito:      sanitizeForCSV(currentCodDebito),
			Historico:        historico,
			ValorPrincipal:   sanitizeForCSV(svc.formatTwoDecimalsComma(vPrincipal)),
			Juros:            sanitizeForCSV(svc.formatTwoDecimalsComma(vJuros)),
			Desconto:         sanitizeForCSV(svc.formatTwoDecimalsComma(vDesc)),
			DespBanco:        sanitizeForCSV(svc.formatTwoDecimalsComma(vDespBco)),
			DespCartorio:     sanitizeForCSV(svc.formatTwoDecimalsComma(vDespCart)),
			VlLiqPago:        sanitizeForCSV(svc.formatTwoDecimalsComma(vVlliq)),
		})
	}

	return svc.gerarCSVAtoliniRecebimentos(finalRows)
}
func (svc *service) gerarCSVAtoliniRecebimentos(rows []domain.AtoliniRecebimentosOutputRow) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := charmap.Windows1252.NewEncoder()
	writer := csv.NewWriter(transform.NewWriter(&buffer, encoder))
	writer.Comma = ';'

	header := []string{"Data", "Descrição Credito", "conta crédito", "Descrição Débito", "conta Debito", "Histórico", "valor Principal", "Juros", "Desconto", "Desp Banco", "Desp Cartório", "VlLiq Pago"}
	for i := range header {
		header[i] = sanitizeForCSV(header[i])
	}
	if err := writer.Write(header); err != nil {
		return nil, err
	}

	for _, row := range rows {
		record := []string{
			sanitizeForCSV(row.Data),
			sanitizeForCSV(row.DescricaoCredito),
			sanitizeForCSV(row.ContaCredito),
			sanitizeForCSV(row.DescricaoDebito),
			sanitizeForCSV(row.ContaDebito),
			sanitizeForCSV(row.Historico),
			sanitizeForCSV(row.ValorPrincipal),
			sanitizeForCSV(row.Juros),
			sanitizeForCSV(row.Desconto),
			sanitizeForCSV(row.DespBanco),
			sanitizeForCSV(row.DespCartorio),
			sanitizeForCSV(row.VlLiqPago),
		}
		if err := writer.Write(record); err != nil {
			return nil, err
		}
	}

	writer.Flush()
	return buffer.Bytes(), writer.Error()
}
