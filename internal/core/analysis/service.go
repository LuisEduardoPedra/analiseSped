// package analysis/service.go
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

const EPSILON = 0.01

// Service defines the interface for SPED file analysis services.
type Service interface {
	AnalyzeICMSFiles(spedFile io.Reader, xmlFiles []io.Reader, cfopsToIgnore []string) ([]domain.AnalysisResult, error)
	AnalyzeIPISTFiles(spedFile io.Reader, xmlFiles []io.Reader) ([]domain.AnalysisResult, error)
}

type service struct{}

// NewService creates a new analysis service.
func NewService() Service {
	return &service{}
}

// AnalyzeIPISTFiles analyzes IPI and ST from SPED and XML files.
func (s *service) AnalyzeIPISTFiles(spedFile io.Reader, xmlFiles []io.Reader) ([]domain.AnalysisResult, error) {
	xmlDataMap, err := s.parseXMLsForIPIST(xmlFiles)
	if err != nil {
		return nil, fmt.Errorf("falha ao processar arquivos XML: %w", err)
	}

	spedDataMap, err := s.parseSpedForIPIST(spedFile)
	if err != nil {
		return nil, fmt.Errorf("falha ao processar arquivo SPED: %w", err)
	}

	var finalResults []domain.AnalysisResult
	for nfeKey, xmlData := range xmlDataMap {
		spedData, foundInSped := spedDataMap[nfeKey]
		if !foundInSped {
			continue
		}

		stDifference := xmlData.STValue - spedData.STValueSPED
		ipiDifference := xmlData.IPIValue - spedData.IPIValueSPED

		var statusCode domain.StatusCode = domain.StatusOK
		var alerts []string = spedData.Alerts

		if math.Abs(stDifference) > EPSILON || math.Abs(ipiDifference) > EPSILON {
			statusCode = domain.StatusDiscrepanciaIPIST
			alerts = append(alerts, "Discrepância detectada nos valores de IPI/ST")
		}

		if statusCode != domain.StatusOK {
			data := domain.IPISTData{
				STValueXML:   xmlData.STValue,
				IPIValueXML:  xmlData.IPIValue,
				STValueSPED:  spedData.STValueSPED,
				IPIValueSPED: spedData.IPIValueSPED,
			}
			result := domain.AnalysisResult{
				Type:       domain.TypeIPIST,
				NFeKey:     nfeKey,
				StatusCode: statusCode,
				Alerts:     alerts,
				Data:       data,
			}
			finalResults = append(finalResults, result)
		}
	}

	return finalResults, nil
}

// parseXMLsForIPIST parses XML files for IPI and ST data.
func (s *service) parseXMLsForIPIST(xmlFiles []io.Reader) (map[string]domain.XMLTaxData, error) {
	xmlDataMap := make(map[string]domain.XMLTaxData)

	for _, xmlFile := range xmlFiles {
		bytes, err := io.ReadAll(xmlFile)
		if err != nil {
			continue
		}

		var nfeProc domain.NFeProc
		if err := xml.Unmarshal(bytes, &nfeProc); err != nil {
			continue
		}

		infNFe := nfeProc.NFe.InfNFe
		nfeKey := strings.TrimPrefix(infNFe.ID, "NFe")
		if nfeKey != "" {
			xmlDataMap[nfeKey] = domain.XMLTaxData{
				STValue:  infNFe.Total.ICMSTot.VST,
				IPIValue: infNFe.Total.ICMSTot.VIPI,
			}
		}
	}
	return xmlDataMap, nil
}

// SpedIPISTResult holds SPED data for IPI/ST.
type SpedIPISTResult struct {
	STValueSPED  float64
	IPIValueSPED float64
	Alerts       []string
}

// parseSpedForIPIST parses SPED file for IPI and ST data.
func (s *service) parseSpedForIPIST(spedFile io.Reader) (map[string]SpedIPISTResult, error) {
	contexts := make(map[string]*domain.SpedTaxContext)
	var currentC100Key string

	decoder := charmap.ISO8859_1.NewDecoder()
	scanner := bufio.NewScanner(decoder.Reader(spedFile))

	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, "|")
		if len(parts) < 2 {
			continue
		}
		recordType := parts[1]
		switch recordType {
		case "C100":
			if len(parts) > 25 {
				nfeKey := parts[9]
				currentC100Key = nfeKey
				if _, ok := contexts[nfeKey]; !ok {
					contexts[nfeKey] = &domain.SpedTaxContext{}
				}
				contexts[nfeKey].C100IPIValue = parseNumberSped(parts[25])
				contexts[nfeKey].C100STValue = parseNumberSped(parts[24])
			}
		case "C170":
			if ctx, ok := contexts[currentC100Key]; ok && len(parts) > 24 {
				ctx.C170SumST += parseNumberSped(parts[18])
				ctx.C170SumIPI += parseNumberSped(parts[24])
			}
		case "C190":
			if ctx, ok := contexts[currentC100Key]; ok && len(parts) > 9 {
				ctx.C190SumST += parseNumberSped(parts[9])
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("erro ao ler arquivo SPED: %w", err)
	}

	finalizedResults := make(map[string]SpedIPISTResult)
	for key, ctx := range contexts {
		var finalST, finalIPI float64
		var alerts []string

		if ctx.C100STValue > EPSILON {
			finalST = ctx.C100STValue
		} else if ctx.C170SumST > EPSILON {
			finalST = ctx.C170SumST
		} else if ctx.C190SumST > EPSILON {
			finalST = ctx.C190SumST
		}

		if ctx.C100IPIValue > EPSILON {
			finalIPI = ctx.C100IPIValue
		} else if ctx.C170SumIPI > EPSILON {
			finalIPI = ctx.C170SumIPI
		}

		if ctx.C100STValue > 0 && math.Abs(ctx.C100STValue-ctx.C170SumST) > 0.5 {
			alerts = append(alerts, "Divergência entre ST do C100 e a soma dos itens C170")
		}
		if ctx.C100IPIValue > 0 && math.Abs(ctx.C100IPIValue-ctx.C170SumIPI) > 0.5 {
			alerts = append(alerts, "Divergência entre IPI do C100 e a soma dos itens C170")
		}

		finalizedResults[key] = SpedIPISTResult{
			STValueSPED:  finalST,
			IPIValueSPED: finalIPI,
			Alerts:       alerts,
		}
	}
	return finalizedResults, nil
}

// AnalyzeICMSFiles analyzes ICMS from SPED and XML files.
func (s *service) AnalyzeICMSFiles(spedFile io.Reader, xmlFiles []io.Reader, cfopsToIgnore []string) ([]domain.AnalysisResult, error) {
	cfopsMap := make(map[string]bool)
	for _, cfop := range cfopsToIgnore {
		cfopsMap[cfop] = true
	}

	spedData, err := s.parseSpedFileForICMS(spedFile, cfopsMap)
	if err != nil {
		return nil, fmt.Errorf("falha ao processar arquivo SPED: %w", err)
	}

	var problematicResults []domain.AnalysisResult

	for _, xmlFile := range xmlFiles {
		xmlResult, err := s.parseXMLForICMS(xmlFile)
		if err != nil {
			data := domain.ICMSData{
				DocNumber: xmlResult.DocNumber,
				IcmsXML:   xmlResult.IcmsXML,
			}
			result := domain.AnalysisResult{
				Type:       domain.TypeICMS,
				NFeKey:     xmlResult.NFeKey,
				StatusCode: domain.StatusXMLInvalido,
				Alerts:     []string{err.Error()},
				Data:       data,
			}
			problematicResults = append(problematicResults, result)
			continue
		}

		var statusCode domain.StatusCode = domain.StatusOK
		var alerts []string

		if spedInfo, ok := spedData[xmlResult.NFeKey]; ok {
			data := domain.ICMSData{
				DocNumber: xmlResult.DocNumber,
				IcmsXML:   xmlResult.IcmsXML,
				IcmsSPED:  spedInfo.Icms,
				CfopsSPED: spedInfo.Cfops,
			}

			if !spedInfo.TemCfopIgnorado && xmlResult.IcmsXML != spedInfo.Icms {
				statusCode = domain.StatusDiscrepanciaICMS
				alerts = append(alerts, fmt.Sprintf("Discrepância detectada: ICMS XML=%.2f, SPED=%.2f", xmlResult.IcmsXML, spedInfo.Icms))
			}

			if statusCode != domain.StatusOK {
				result := domain.AnalysisResult{
					Type:       domain.TypeICMS,
					NFeKey:     xmlResult.NFeKey,
					StatusCode: statusCode,
					Alerts:     alerts,
					Data:       data,
				}
				problematicResults = append(problematicResults, result)
			}
		} else {
			data := domain.ICMSData{
				DocNumber: xmlResult.DocNumber,
				IcmsXML:   xmlResult.IcmsXML,
			}
			result := domain.AnalysisResult{
				Type:       domain.TypeICMS,
				NFeKey:     xmlResult.NFeKey,
				StatusCode: domain.StatusNaoEncontradaSPED,
				Alerts:     []string{"NFe não encontrada no SPED"},
				Data:       data,
			}
			problematicResults = append(problematicResults, result)
		}
	}
	return problematicResults, nil
}

// parseXMLForICMS parses an XML file for ICMS data.
func (s *service) parseXMLForICMS(xmlFile io.Reader) (struct {
	DocNumber string
	NFeKey    string
	IcmsXML   float64
}, error) {
	result := struct {
		DocNumber string
		NFeKey    string
		IcmsXML   float64
	}{DocNumber: "ERRO", NFeKey: "ERRO"}
	xmlData, err := io.ReadAll(xmlFile)
	if err != nil {
		return result, fmt.Errorf("erro ao ler dados do XML: %w", err)
	}

	var nfeProc domain.NFeProc
	if err := xml.Unmarshal(xmlData, &nfeProc); err != nil {
		return result, fmt.Errorf("falha ao fazer parse do XML: %w", err)
	}

	infNFe := nfeProc.NFe.InfNFe
	if infNFe.Ide.NNF == "" {
		return result, fmt.Errorf("XML inválido ou não é uma NF-e")
	}

	result.DocNumber = infNFe.Ide.NNF
	result.NFeKey = nfeProc.ProtNFe.InfProt.ChNFe

	var totalICMS float64
	for _, det := range infNFe.Det {
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
	result.IcmsXML = round(totalICMS, 2)
	return result, nil
}

// parseSpedFileForICMS parses SPED file for ICMS data.
func (s *service) parseSpedFileForICMS(spedFile io.Reader, cfopsSemCredito map[string]bool) (map[string]domain.SpedInfo, error) {
	spedData := make(map[string]domain.SpedInfo)
	decoder := charmap.ISO8859_1.NewDecoder()
	scanner := bufio.NewScanner(decoder.Reader(spedFile))

	var currentC100Key string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		parts := strings.Split(line, "|")
		if len(parts) < 2 {
			continue
		}

		recordType := parts[1]
		switch recordType {
		case "C100":
			if len(parts) > 9 {
				currentC100Key = parts[9]
				if _, ok := spedData[currentC100Key]; !ok {
					spedData[currentC100Key] = domain.SpedInfo{Cfops: []string{}}
				}
			}
		case "C190":
			if info, ok := spedData[currentC100Key]; ok && len(parts) > 7 {
				cfop := parts[3]
				found := false
				for _, existingCfop := range info.Cfops {
					if existingCfop == cfop {
						found = true
						break
					}
				}
				if !found {
					info.Cfops = append(info.Cfops, cfop)
				}

				if cfopsSemCredito[cfop] {
					info.TemCfopIgnorado = true
				}
				icmsVal := parseNumberSped(parts[7])
				info.Icms += icmsVal
				spedData[currentC100Key] = info
			}
		}
	}

	for key, info := range spedData {
		info.Icms = round(info.Icms, 2)
		spedData[key] = info
	}

	return spedData, scanner.Err()
}

// parseNumberSped parses a number from SPED format.
func parseNumberSped(val string) float64 {
	if val == "" {
		return 0.0
	}
	s := strings.Replace(val, ",", ".", 1)
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0.0
	}
	return f
}

// round rounds a float to the specified places.
func round(val float64, places int) float64 {
	pow := math.Pow(10, float64(places))
	return math.Round(val*pow) / pow
}
