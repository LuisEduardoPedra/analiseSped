// internal/domain/models.go
package domain

import "encoding/xml"

// (As structs NFeProc, NFeXML e SpedInfo permanecem as mesmas da versão anterior)
type NFeProc struct {
	XMLName xml.Name `xml:"nfeProc"`
	NFe     NFeXML   `xml:"NFe"`
	ProtNFe struct {
		InfProt struct {
			ChNFe string `xml:"chNFe"`
		} `xml:"infProt"`
	} `xml:"protNFe"`
}

type NFeXML struct {
	InfNFe struct {
		ID  string `xml:"Id,attr"`
		Ide struct {
			NNF string `xml:"nNF"`
		} `xml:"ide"`
		Det []struct {
			Imposto struct {
				ICMS struct {
					ICMS00 struct {
						VICMS string `xml:"vICMS"`
					} `xml:"ICMS00"`
					ICMS10 struct {
						VICMS string `xml:"vICMS"`
					} `xml:"ICMS10"`
					ICMS20 struct {
						VICMS string `xml:"vICMS"`
					} `xml:"ICMS20"`
					ICMS70 struct {
						VICMS string `xml:"vICMS"`
					} `xml:"ICMS70"`
					ICMS90 struct {
						VICMS string `xml:"vICMS"`
					} `xml:"ICMS90"`
					ICMSSN101 struct {
						VCreditICMSSN string `xml:"vCredICMSSN"`
					} `xml:"ICMSSN101"`
					ICMSSN102 struct {
						VICMS string `xml:"vICMS"`
					} `xml:"ICMSSN102"`
				} `xml:"ICMS"`
			} `xml:"imposto"`
		} `xml:"det"`
	} `xml:"infNFe"`
}

type SpedInfo struct {
	Icms            float64
	Cfops           []string
	TemCfopIgnorado bool
}

// --- ALTERAÇÕES AQUI ---

// StatusCode define um tipo para nossos códigos de status numéricos.
type StatusCode int

// Definimos os códigos de status que a API pode retornar.
// O frontend usará esses números para determinar como exibir o resultado.
const (
	StatusDiscrepanciaICMS  StatusCode = 1 // Problema: Diferença de valores de ICMS.
	StatusNaoEncontradaSPED StatusCode = 2 // Problema: Nota não localizada no SPED.
	StatusXMLInvalido       StatusCode = 3 // Problema: O arquivo XML não pôde ser lido.
)

// AnaliseResult foi atualizado para usar o StatusCode numérico.
type AnaliseResult struct {
	NumNota      string     `json:"num_nota"`
	ChaveNFe     string     `json:"chave_nfe"`
	IcmsXML      float64    `json:"icms_xml"`
	IcmsSPED     float64    `json:"icms_sped"`
	CfopsSPED    []string   `json:"cfops_sped"`
	Discrepancia bool       `json:"discrepancia"`
	StatusCode   StatusCode `json:"status_code"` // Campo 'status' agora é 'status_code' e numérico.
}
