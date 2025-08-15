// internal/api/handlers/analysis_handler.go
package handlers

import (
	"io"
	"net/http"
	"strings"

	"github.com/LuisEduardoPedra/analiseSped/internal/core/analysis"
	"github.com/gin-gonic/gin"
)

// (A struct AnalysisHandler e a função NewAnalysisHandler não mudam)
type AnalysisHandler struct {
	service analysis.Service
}

func NewAnalysisHandler(service analysis.Service) *AnalysisHandler {
	return &AnalysisHandler{
		service: service,
	}
}

func (h *AnalysisHandler) HandleAnalysis(c *gin.Context) {
	// 1. Receber os arquivos (lógica inalterada)
	spedFileHeader, err := c.FormFile("spedFile")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Arquivo SPED não encontrado ou inválido"})
		return
	}
	spedFile, err := spedFileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Não foi possível abrir o arquivo SPED"})
		return
	}
	defer spedFile.Close()

	form, _ := c.MultipartForm()
	xmlFileHeaders := form.File["xmlFiles"]
	if len(xmlFileHeaders) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Nenhum arquivo XML foi enviado"})
		return
	}

	var xmlReaders []io.Reader
	// Usamos um loop anônimo com `defer` para garantir que todos os arquivos sejam fechados.
	func() {
		for _, header := range xmlFileHeaders {
			file, err := header.Open()
			if err != nil {
				// Este defer não será executado se err != nil, então está seguro.
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Não foi possível abrir um dos arquivos XML"})
				return
			}
			defer file.Close()
			xmlReaders = append(xmlReaders, file)
		}
	}()

	// --- ALTERAÇÕES AQUI ---
	// 2. Receber a lista de CFOPs do formulário.
	// O frontend deve enviar um campo de texto chamado 'cfopsIgnorados' com os valores separados por vírgula.
	cfopsStr := c.PostForm("cfopsIgnorados")
	var cfopsIgnorados []string
	if cfopsStr != "" {
		// Divide a string pela vírgula e remove espaços em branco de cada CFOP.
		parts := strings.Split(cfopsStr, ",")
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				cfopsIgnorados = append(cfopsIgnorados, trimmed)
			}
		}
	}
	// Se 'cfopsIgnorados' estiver vazio, uma lista vazia será passada, o que está correto.

	// 3. Chamar o serviço com a lista de CFOPs.
	resultados, err := h.service.AnalisarArquivos(spedFile, xmlReaders, cfopsIgnorados)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resultados)
}
