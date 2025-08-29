// internal/api/handlers/analysis_handler.go
package handlers

import (
	"io"
	"net/http"
	"strings"

	"github.com/LuisEduardoPedra/analiseSped/internal/api/responses"
	"github.com/LuisEduardoPedra/analiseSped/internal/core/analysis"
	"github.com/gin-gonic/gin"
)

// AnalysisHandler handles analysis-related API requests.
type AnalysisHandler struct {
	service analysis.Service
}

// NewAnalysisHandler creates a new analysis handler.
func NewAnalysisHandler(service analysis.Service) *AnalysisHandler {
	return &AnalysisHandler{
		service: service,
	}
}

// HandleAnalysisIcms handles ICMS analysis requests.
func (h *AnalysisHandler) HandleAnalysisIcms(c *gin.Context) {
	spedFileHeader, err := c.FormFile("spedFile")
	if err != nil {
		responses.Error(c, http.StatusBadRequest, "Arquivo SPED não encontrado ou inválido")
		return
	}
	spedFile, err := spedFileHeader.Open()
	if err != nil {
		responses.Error(c, http.StatusInternalServerError, "Não foi possível abrir o arquivo SPED")
		return
	}
	defer spedFile.Close()

	form, _ := c.MultipartForm()
	xmlFileHeaders := form.File["xmlFiles"]
	if len(xmlFileHeaders) == 0 {
		responses.Error(c, http.StatusBadRequest, "Nenhum arquivo XML foi enviado")
		return
	}

	var xmlReaders []io.Reader
	for _, header := range xmlFileHeaders {
		file, err := header.Open()
		if err != nil {
			responses.Error(c, http.StatusInternalServerError, "Não foi possível abrir um dos arquivos XML")
			return
		}
		defer file.Close()
		xmlReaders = append(xmlReaders, file)
	}

	cfopsStr := c.PostForm("cfopsIgnorados")
	var cfopsIgnorados []string
	if cfopsStr != "" {
		parts := strings.Split(cfopsStr, ",")
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				cfopsIgnorados = append(cfopsIgnorados, trimmed)
			}
		}
	}

	resultados, err := h.service.AnalyzeICMSFiles(spedFile, xmlReaders, cfopsIgnorados)
	if err != nil {
		responses.Error(c, http.StatusInternalServerError, "Erro na análise de ICMS", err.Error())
		return
	}

	responses.Success(c, resultados, "Análise de ICMS concluída com sucesso")
}

// HandleAnalysisIpiSt handles IPI and ST analysis requests.
func (h *AnalysisHandler) HandleAnalysisIpiSt(c *gin.Context) {
	spedFileHeader, err := c.FormFile("spedFile")
	if err != nil {
		responses.Error(c, http.StatusBadRequest, "Arquivo SPED não encontrado ou inválido")
		return
	}
	spedFile, err := spedFileHeader.Open()
	if err != nil {
		responses.Error(c, http.StatusInternalServerError, "Não foi possível abrir o arquivo SPED")
		return
	}
	defer spedFile.Close()

	form, _ := c.MultipartForm()
	xmlFileHeaders := form.File["xmlFiles"]
	if len(xmlFileHeaders) == 0 {
		responses.Error(c, http.StatusBadRequest, "Nenhum arquivo XML foi enviado")
		return
	}

	var xmlReaders []io.Reader
	var closers []io.Closer
	defer func() {
		for _, closer := range closers {
			closer.Close()
		}
	}()

	for _, header := range xmlFileHeaders {
		file, err := header.Open()
		if err != nil {
			responses.Error(c, http.StatusInternalServerError, "Não foi possível abrir um dos arquivos XML")
			return
		}
		xmlReaders = append(xmlReaders, file)
		closers = append(closers, file)
	}

	resultados, err := h.service.AnalyzeIPISTFiles(spedFile, xmlReaders)
	if err != nil {
		responses.Error(c, http.StatusInternalServerError, "Erro na análise de IPI e ST", err.Error())
		return
	}

	responses.Success(c, resultados, "Análise de IPI e ST concluída com sucesso")
}
