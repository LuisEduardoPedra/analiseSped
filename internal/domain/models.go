// package domain/models.go
package domain

import (
	"encoding/xml"
	"time"
)

// AnalysisType defines the type of analysis.
type AnalysisType string

// Constants for analysis types.
const (
	TypeICMS  AnalysisType = "ICMS"
	TypeIPIST AnalysisType = "IPIST"
)

// StatusCode defines a type for analysis status codes.
type StatusCode int

// Constants defining possible analysis results.
const (
	StatusOK                StatusCode = 0
	StatusDiscrepanciaICMS  StatusCode = 1
	StatusNaoEncontradaSPED StatusCode = 2
	StatusXMLInvalido       StatusCode = 3
	StatusDiscrepanciaIPIST StatusCode = 4
)

// AnalysisResult is the generic structure for analysis results.
type AnalysisResult struct {
	Type       AnalysisType `json:"type"`
	NFeKey     string       `json:"nfe_key"`
	StatusCode StatusCode   `json:"status_code"`
	Alerts     []string     `json:"alerts"`
	Data       interface{}  `json:"data"`
}

// ICMSData holds specific data for ICMS analysis.
type ICMSData struct {
	DocNumber string   `json:"doc_number"`
	IcmsXML   float64  `json:"icms_xml"`
	IcmsSPED  float64  `json:"icms_sped"`
	CfopsSPED []string `json:"cfops_sped"`
}

// IPISTData holds specific data for IPI/ST analysis.
type IPISTData struct {
	STValueXML   float64 `json:"st_value_xml"`
	IPIValueXML  float64 `json:"ipi_value_xml"`
	STValueSPED  float64 `json:"st_value_sped"`
	IPIValueSPED float64 `json:"ipi_value_sped"`
}

// SpedInfo contains information extracted from the SPED file for a specific NFe.
type SpedInfo struct {
	Icms            float64
	Cfops           []string
	TemCfopIgnorado bool
}

// SpedTaxContext stores accumulated tax values for an NFe during SPED reading.
type SpedTaxContext struct {
	C100STValue  float64
	C100IPIValue float64
	C170SumST    float64
	C170SumIPI   float64
	C190SumST    float64
}

// XMLTaxData stores tax values extracted from a single XML.
type XMLTaxData struct {
	STValue  float64
	IPIValue float64
}

// NFeProc represents the root structure of a processed NFe XML.
type NFeProc struct {
	XMLName xml.Name `xml:"nfeProc"`
	NFe     NFeXML   `xml:"NFe"`
	ProtNFe struct {
		InfProt struct {
			ChNFe string `xml:"chNFe"`
		} `xml:"infProt"`
	} `xml:"protNFe"`
}

// NFeXML represents the <NFe> node in the XML.
type NFeXML struct {
	InfNFe struct {
		ID    string   `xml:"Id,attr"`
		Ide   IdeXML   `xml:"ide"`
		Det   []DetXML `xml:"det"`
		Total TotalXML `xml:"total"`
	} `xml:"infNFe"`
}

// IdeXML represents the <ide> node (NFe identification).
type IdeXML struct {
	NNF string `xml:"nNF"`
}

// TotalXML represents the <total> node with tax totals.
type TotalXML struct {
	ICMSTot ICMSTotXML `xml:"ICMSTot"`
}

// ICMSTotXML represents the <ICMSTot> node with ICMS, ST, and IPI values.
type ICMSTotXML struct {
	VST  float64 `xml:"vST"`
	VIPI float64 `xml:"vIPI"`
}

// DetXML represents the <det> node (product/service details).
type DetXML struct {
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
		} `xml:"ICMS"`
	} `xml:"imposto"`
}

// --- Modelos de Conversor Francesinha ---

// ContaSicredi representa uma entrada do arquivo Contas.csv para o conversor Sicredi.
type ContaSicredi struct {
	Code    string
	Classif string
	Desc    string
}

// Lancamento representa uma linha de lançamento do arquivo de entrada.
type Lancamento struct {
	DataLiquidacao time.Time
	Descricao      string
	Valor          float64
	Historico      string
}

// OutputRow representa uma linha do arquivo CSV de saída.
type OutputRow struct {
	Operacao         string
	Data             string
	DescricaoCredito string
	ContaCredito     string
	Valor            string
	Historico        string
}

// --- Modelos de Conversor Receitas ACISA ---

// ContaReceitasAcisa representa uma entrada do arquivo Contas.csv para o conversor de receitas.
type ContaReceitasAcisa struct {
	Code    string
	Classif string
	Desc    string
}

// ReceitasAcisaOutputRow representa uma linha do arquivo CSV de saída do conversor de receitas.
type ReceitasAcisaOutputRow struct {
	Data        string
	Descricao   string
	Conta       string
	Mensalidade string
	Pis         string
	Historico   string
}

// --- Modelos de Conversores Atolini ---

// ContaAtolini representa uma conta genérica para os conversores Atolini.
type ContaAtolini struct {
	Code    string
	Classif string
	Desc    string
}

// AtoliniPagamentosOutputRow representa uma linha do CSV de saída para Atolini Pagamentos.
type AtoliniPagamentosOutputRow struct {
	Data              string
	Debito            string
	DescricaoConta    string
	Credito           string
	DescricaoCredito  string
	Valor             string
	Historico         string
	ValorOriginal     string
	ValorPago         string
	ValorJuros        string
	ValorMulta        string
	ValorDesconto     string
	ValorDespesas     string
	VarCam            string
	ValorLiqPagoBanco string
}

// AtoliniRecebimentosOutputRow representa uma linha do CSV de saída para Atolini Recebimentos.
type AtoliniRecebimentosOutputRow struct {
	Data             string
	DescricaoCredito string
	ContaCredito     string
	DescricaoDebito  string
	ContaDebito      string
	Historico        string
	ValorPrincipal   string
	Juros            string
	Desconto         string
	DespBanco        string
	DespCartorio     string
	VlLiqPago        string
}
