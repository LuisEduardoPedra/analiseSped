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

// HandleSicrediConversion lida com a conversão de arquivos do Sicredi.
func (h *ConverterHandler) HandleSicrediConversion(c *gin.Context) {
	// 1. Obter os arquivos da requisição
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

	// 2. Validar extensão do arquivo de lançamentos
	ext := strings.ToLower(filepath.Ext(lancamentosFileHeader.Filename))
	if ext != ".csv" && ext != ".xls" && ext != ".xlsx" {
		responses.Error(c, http.StatusBadRequest, fmt.Sprintf("Extensão de arquivo de lançamentos não suportada: %s", ext))
		return
	}

	// 3. Abrir os arquivos
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

	// 4. Chamar o serviço de conversão
	outputCSV, err := h.service.ProcessSicrediFiles(lancamentosFile, contasFile, lancamentosFileHeader.Filename)
	if err != nil {
		fmt.Printf("Erro ao processar arquivos Sicredi: %v\n", err)
		responses.Error(c, http.StatusInternalServerError, "Erro ao processar os arquivos", err.Error())
		return
	}

	// 5. Retornar o arquivo CSV gerado
	fileName := fmt.Sprintf("LancamentosFinal_%s.csv", time.Now().Format("20060102_150405"))
	c.Header("Content-Disposition", "attachment; filename="+fileName)
	c.Data(http.StatusOK, "text/csv; charset=utf-8", outputCSV)
}

// HandleReceitasAcisaConversion lida com a conversão de receitas ACISA.
func (h *ConverterHandler) HandleReceitasAcisaConversion(c *gin.Context) {
	// 1. Obter os arquivos da requisição
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

	// 2. Validar extensão do arquivo excel
	ext := strings.ToLower(filepath.Ext(excelFileHeader.Filename))
	if ext != ".xls" && ext != ".xlsx" {
		responses.Error(c, http.StatusBadRequest, fmt.Sprintf("Extensão de arquivo excel não suportada: %s", ext))
		return
	}

	// Obter prefixos de classe (parâmetro opcional)
	classPrefixesStr := c.PostForm("classPrefixes")
	var classPrefixes []string
	if classPrefixesStr != "" {
		parts := strings.Split(classPrefixesStr, ",")
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				classPrefixes = append(classPrefixes, trimmed)
			}
		}
	}

	// 3. Abrir os arquivos
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

	// 4. Chamar o serviço de conversão
	outputCSV, err := h.service.ProcessReceitasAcisaFiles(excelFile, contasFile, excelFileHeader.Filename, classPrefixes)
	if err != nil {
		fmt.Printf("Erro ao processar arquivos para receitas ACISA: %v\n", err)
		responses.Error(c, http.StatusInternalServerError, "Erro ao processar os arquivos", err.Error())
		return
	}

	// 5. Retornar o arquivo CSV gerado
	fileName := fmt.Sprintf("ReceitasAcisa_%s.csv", time.Now().Format("20060102_150405"))
	c.Header("Content-Disposition", "attachment; filename="+fileName)
	c.Data(http.StatusOK, "text/csv; charset=utf-8", outputCSV)
}
