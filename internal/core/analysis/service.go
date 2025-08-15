// internal/core/analysis/service.go
package analysis

import (
	"bufio"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/LuisEduardoPedra/analiseSped/internal/domain"
	"golang.org/x/text/encoding/charmap"
)

// (A interface, a struct 'service' e a função NewService não mudam)
type Service interface {
	AnalisarArquivos(spedFile io.Reader, xmlFiles []io.Reader, cfopsIgnorados []string) ([]domain.AnaliseResult, error)
}
type service struct{}

func NewService() Service {
	return &service{}
}

func (s *service) AnalisarArquivos(spedFile io.Reader, xmlFiles []io.Reader, cfopsIgnorados []string) ([]domain.AnaliseResult, error) {
	cfopsMap := make(map[string]bool)
	for _, cfop := range cfopsIgnorados {
		cfopsMap[cfop] = true
	}

	spedData, err := s.parseSpedFile(spedFile, cfopsMap)
	if err != nil {
		return nil, fmt.Errorf("falha ao processar arquivo SPED: %w", err)
	}

	var resultadosProblematicos []domain.AnaliseResult

	for _, xmlFile := range xmlFiles {
		resultadoXML, err := s.parseXML(xmlFile)
		if err != nil {
			// --- CORREÇÃO APLICADA ---
			resultadoXML.StatusCode = domain.StatusXMLInvalido
			resultadosProblematicos = append(resultadosProblematicos, resultadoXML)
			continue
		}

		if spedInfo, ok := spedData[resultadoXML.ChaveNFe]; ok {
			resultadoXML.IcmsSPED = spedInfo.Icms
			resultadoXML.CfopsSPED = spedInfo.Cfops

			if !spedInfo.TemCfopIgnorado && resultadoXML.IcmsXML != resultadoXML.IcmsSPED {
				resultadoXML.Discrepancia = true
				// --- CORREÇÃO APLICADA ---
				resultadoXML.StatusCode = domain.StatusDiscrepanciaICMS
				resultadosProblematicos = append(resultadosProblematicos, resultadoXML)
			}
		} else {
			// --- CORREÇÃO APLICADA ---
			resultadoXML.StatusCode = domain.StatusNaoEncontradaSPED
			resultadosProblematicos = append(resultadosProblematicos, resultadoXML)
		}
	}

	return resultadosProblematicos, nil
}

// (O restante do arquivo não precisa de alterações)
func (s *service) parseSpedFile(spedFile io.Reader, cfopsSemCredito map[string]bool) (map[string]domain.SpedInfo, error) {
	spedData := make(map[string]domain.SpedInfo)
	spedDecoder := charmap.ISO8859_1.NewDecoder()
	spedReader := spedDecoder.Reader(spedFile)
	scanner := bufio.NewScanner(spedReader)
	var linhas []string
	for scanner.Scan() {
		linhas = append(linhas, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("erro ao ler SPED: %w", err)
	}
	for i, linha := range linhas {
		campos := strings.Split(strings.TrimSpace(linha), "|")
		if len(campos) > 9 && campos[1] == "C100" {
			chaveNFe := campos[9]
			info := domain.SpedInfo{Cfops: []string{}}
			for j := i + 1; j < len(linhas); j++ {
				camposC190 := strings.Split(strings.TrimSpace(linhas[j]), "|")
				if len(camposC190) > 1 && camposC190[1] == "C100" {
					break
				}
				if len(camposC190) > 7 && camposC190[1] == "C190" {
					cfop := camposC190[3]
					info.Cfops = append(info.Cfops, cfop)
					if cfopsSemCredito[cfop] {
						info.TemCfopIgnorado = true
					}
					icmsStr := strings.Replace(camposC190[7], ",", ".", 1)
					valorIcms, _ := strconv.ParseFloat(icmsStr, 64)
					info.Icms += valorIcms
				}
			}
			info.Icms = round(info.Icms, 2)
			spedData[chaveNFe] = info
		}
	}
	return spedData, nil
}
func (s *service) parseXML(xmlFile io.Reader) (domain.AnaliseResult, error) {
	resultado := domain.AnaliseResult{NumNota: "ERRO", ChaveNFe: "ERRO"}
	xmlData, err := io.ReadAll(xmlFile)
	if err != nil {
		return resultado, fmt.Errorf("erro ao ler dados do XML: %w", err)
	}
	var nfeProc domain.NFeProc
	if err := xml.Unmarshal(xmlData, &nfeProc); err != nil {
		return resultado, fmt.Errorf("falha ao fazer parse do XML: %w", err)
	}
	nfe := nfeProc.NFe
	if nfe.InfNFe.Ide.NNF == "" {
		return resultado, fmt.Errorf("XML inválido ou não é uma NF-e")
	}
	resultado.NumNota = nfe.InfNFe.Ide.NNF
	resultado.ChaveNFe = nfeProc.ProtNFe.InfProt.ChNFe
	totalICMS := 0.0
	for _, det := range nfe.InfNFe.Det {
		icms := det.Imposto.ICMS
		var vICMSStr string
		switch {
		case icms.ICMS00.VICMS != "":
			vICMSStr = icms.ICMS00.VICMS
		case icms.ICMS10.VICMS != "":
			vICMSStr = icms.ICMS10.VICMS
		case icms.ICMS20.VICMS != "":
			vICMSStr = icms.ICMS20.VICMS
		case icms.ICMS70.VICMS != "":
			vICMSStr = icms.ICMS70.VICMS
		case icms.ICMS90.VICMS != "":
			vICMSStr = icms.ICMS90.VICMS
		case icms.ICMSSN101.VCreditICMSSN != "":
			vICMSStr = icms.ICMSSN101.VCreditICMSSN
		}
		if vICMS, err := strconv.ParseFloat(vICMSStr, 64); err == nil {
			totalICMS += vICMS
		}
	}
	resultado.IcmsXML = round(totalICMS, 2)
	return resultado, nil
}
func round(val float64, places int) float64 {
	pow := math.Pow(10, float64(places))
	return math.Round(val*pow) / pow
}
