// internal/api/handlers/converter_handler.go
package handlers

import (
	"fmt"
	"net/http"
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

	// 2. Abrir os arquivos
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

	// 3. Chamar o serviço de conversão, passando os streams e o nome do arquivo
	outputCSV, err := h.service.ProcessSicrediFiles(lancamentosFile, contasFile, lancamentosFileHeader.Filename)
	if err != nil {
		responses.Error(c, http.StatusInternalServerError, "Erro ao processar os arquivos", err.Error())
		return
	}

	// 4. Retornar o arquivo CSV gerado
	fileName := fmt.Sprintf("LancamentosFinal_%s.csv", time.Now().Format("20060102_150405"))
	c.Header("Content-Disposition", "attachment; filename="+fileName)
	c.Data(http.StatusOK, "text/csv; charset=utf-8", outputCSV)
}
