package handlers

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/LuisEduardoPedra/analiseSped/internal/api/responses"
	"github.com/LuisEduardoPedra/analiseSped/internal/core/converter"
	"github.com/gin-gonic/gin"
)

// ConverterHandler lida com as requisições da API relacionadas à conversão de arquivos.
type ConverterHandler struct {
	service converter.Service
}

// NewConverterHandler cria um novo handler de conversão.
func NewConverterHandler(service converter.Service) *ConverterHandler {
	return &ConverterHandler{
		service: service,
	}
}

// getPrefixesFromForm extrai e limpa os prefixos de um campo de formulário.
func getPrefixesFromForm(c *gin.Context, formKey string) []string {
	prefixesStr := c.PostForm(formKey)
	if prefixesStr == "" {
		return nil
	}
	parts := strings.Split(prefixesStr, ",")
	var prefixes []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			prefixes = append(prefixes, trimmed)
		}
	}
	return prefixes
}

// HandleSicrediConversion lida com a conversão de arquivos do Sicredi (francesinha).
func (h *ConverterHandler) HandleSicrediConversion(c *gin.Context) {
	lancamentosFileHeader, err := c.FormFile("lancamentosFile")
	if err != nil {
		responses.Error(c, http.StatusBadRequest, "Arquivo de Lançamentos (.csv, .xls, .xlsx) não encontrado ou inválido")
		return
	}

	contasFileHeader, err := c.FormFile("contasFile")
	if err != nil {
		responses.Error(c, http.StatusBadRequest, "Arquivo de Contas (.csv) não encontrado ou inválido")
		return
	}

	ext := strings.ToLower(filepath.Ext(lancamentosFileHeader.Filename))
	if ext != ".csv" && ext != ".xls" && ext != ".xlsx" {
		responses.Error(c, http.StatusBadRequest, fmt.Sprintf("Extensão de arquivo de lançamentos não suportada: %s", ext))
		return
	}

	classPrefixes := getPrefixesFromForm(c, "classPrefixes")

	lancamentosFile, err := lancamentosFileHeader.Open()
	if err != nil {
		responses.Error(c, http.StatusInternalServerError, "Não foi possível abrir o arquivo de Lançamentos")
		return
	}
	defer lancamentosFile.Close()

	contasFile, err := contasFileHeader.Open()
	if err != nil {
		responses.Error(c, http.StatusInternalServerError, "Não foi possível abrir o arquivo de Contas")
		return
	}
	defer contasFile.Close()

	outputCSV, err := h.service.ProcessSicrediFiles(lancamentosFile, contasFile, lancamentosFileHeader.Filename, classPrefixes)
	if err != nil {
		fmt.Printf("Erro ao processar arquivos Sicredi: %v\n", err)
		responses.Error(c, http.StatusInternalServerError, "Erro ao processar os arquivos", err.Error())
		return
	}

	fileName := fmt.Sprintf("LancamentosFinal_%s.csv", time.Now().Format("20060102_150405"))
	c.Header("Content-Disposition", "attachment; filename="+fileName)
	c.Data(http.StatusOK, "text/csv; charset=utf-8", outputCSV)
}

// HandleReceitasAcisaConversion lida com a conversão de receitas ACISA.
func (h *ConverterHandler) HandleReceitasAcisaConversion(c *gin.Context) {
	excelFileHeader, err := c.FormFile("excelFile")
	if err != nil {
		responses.Error(c, http.StatusBadRequest, "Arquivo Excel (.xls, .xlsx) não encontrado ou inválido")
		return
	}

	contasFileHeader, err := c.FormFile("contasFile")
	if err != nil {
		responses.Error(c, http.StatusBadRequest, "Arquivo de Contas (.csv) não encontrado ou inválido")
		return
	}

	ext := strings.ToLower(filepath.Ext(excelFileHeader.Filename))
	if ext != ".xls" && ext != ".xlsx" {
		responses.Error(c, http.StatusBadRequest, fmt.Sprintf("Extensão de arquivo excel não suportada: %s", ext))
		return
	}

	classPrefixes := getPrefixesFromForm(c, "classPrefixes")

	excelFile, err := excelFileHeader.Open()
	if err != nil {
		responses.Error(c, http.StatusInternalServerError, "Não foi possível abrir o arquivo Excel")
		return
	}
	defer excelFile.Close()

	contasFile, err := contasFileHeader.Open()
	if err != nil {
		responses.Error(c, http.StatusInternalServerError, "Não foi possível abrir o arquivo de Contas")
		return
	}
	defer contasFile.Close()

	outputCSV, err := h.service.ProcessReceitasAcisaFiles(excelFile, contasFile, excelFileHeader.Filename, classPrefixes)
	if err != nil {
		fmt.Printf("Erro ao processar arquivos para receitas ACISA: %v\n", err)
		responses.Error(c, http.StatusInternalServerError, "Erro ao processar os arquivos", err.Error())
		return
	}

	fileName := fmt.Sprintf("ReceitasAcisa_%s.csv", time.Now().Format("20060102_150405"))
	c.Header("Content-Disposition", "attachment; filename="+fileName)
	c.Data(http.StatusOK, "text/csv; charset=utf-8", outputCSV)
}

// HandleAtoliniPagamentosConversion lida com a conversão de pagamentos Atolini.
func (h *ConverterHandler) HandleAtoliniPagamentosConversion(c *gin.Context) {
	excelFileHeader, err := c.FormFile("lancamentosFile")
	if err != nil {
		responses.Error(c, http.StatusBadRequest, "Arquivo de Lançamentos (.xls, .xlsx) não encontrado ou inválido")
		return
	}

	contasFileHeader, err := c.FormFile("contasFile")
	if err != nil {
		responses.Error(c, http.StatusBadRequest, "Arquivo de Contas (.csv) não encontrado ou inválido")
		return
	}

	// CORREÇÃO: Lê os dois novos parâmetros de filtro
	debitPrefixes := getPrefixesFromForm(c, "debitClassPrefixes")
	creditPrefixes := getPrefixesFromForm(c, "creditClassPrefixes")

	excelFile, err := excelFileHeader.Open()
	if err != nil {
		responses.Error(c, http.StatusInternalServerError, "Não foi possível abrir o arquivo de Lançamentos")
		return
	}
	defer excelFile.Close()

	contasFile, err := contasFileHeader.Open()
	if err != nil {
		responses.Error(c, http.StatusInternalServerError, "Não foi possível abrir o arquivo de Contas")
		return
	}
	defer contasFile.Close()

	// CORREÇÃO: Passa os dois filtros para o serviço
	outputCSV, err := h.service.ProcessAtoliniPagamentos(excelFile, contasFile, debitPrefixes, creditPrefixes)
	if err != nil {
		fmt.Printf("Erro ao processar arquivos para Atolini Pagamentos: %v\n", err)
		responses.Error(c, http.StatusInternalServerError, "Erro ao processar os arquivos", err.Error())
		return
	}

	fileName := fmt.Sprintf("AtoliniPagamentos_%s.csv", time.Now().Format("20060102_150405"))
	c.Header("Content-Disposition", "attachment; filename="+fileName)
	c.Data(http.StatusOK, "text/csv; charset=utf-8", outputCSV)
}

// HandleAtoliniRecebimentosConversion lida com a conversão de recebimentos Atolini.
func (h *ConverterHandler) HandleAtoliniRecebimentosConversion(c *gin.Context) {
	excelFileHeader, err := c.FormFile("lancamentosFile")
	if err != nil {
		responses.Error(c, http.StatusBadRequest, "Arquivo de Lançamentos (.xls, .xlsx) não encontrado ou inválido")
		return
	}

	contasFileHeader, err := c.FormFile("contasFile")
	if err != nil {
		responses.Error(c, http.StatusBadRequest, "Arquivo de Contas (.csv) não encontrado ou inválido")
		return
	}

	debitPrefixes := getPrefixesFromForm(c, "debitPrefixes")
	creditPrefixes := getPrefixesFromForm(c, "creditPrefixes")

	excelFile, err := excelFileHeader.Open()
	if err != nil {
		responses.Error(c, http.StatusInternalServerError, "Não foi possível abrir o arquivo de Lançamentos")
		return
	}
	defer excelFile.Close()

	contasFile, err := contasFileHeader.Open()
	if err != nil {
		responses.Error(c, http.StatusInternalServerError, "Não foi possível abrir o arquivo de Contas")
		return
	}
	defer contasFile.Close()

	outputCSV, err := h.service.ProcessAtoliniRecebimentos(excelFile, contasFile, debitPrefixes, creditPrefixes)
	if err != nil {
		fmt.Printf("Erro ao processar arquivos para Atolini Recebimentos: %v\n", err)
		responses.Error(c, http.StatusInternalServerError, "Erro ao processar os arquivos", err.Error())
		return
	}

	fileName := fmt.Sprintf("AtoliniRecebimentos_%s.csv", time.Now().Format("20060102_150405"))
	c.Header("Content-Disposition", "attachment; filename="+fileName)
	c.Data(http.StatusOK, "text/csv; charset=utf-8", outputCSV)
}
