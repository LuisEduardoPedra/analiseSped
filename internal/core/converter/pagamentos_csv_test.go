package converter

import (
	"bytes"
	"encoding/csv"
	"testing"

	"github.com/LuisEduardoPedra/analiseSped/internal/domain"
)

func TestGerarCSVAtoliniPagamentos_ColunasAlinhadas(t *testing.T) {
	svc := &service{}

	rows := []domain.AtoliniPagamentosOutputRow{
		{
			Data:              "13/01/2026",
			Debito:            "8188",
			DescricaoConta:    "ANAY FITAS COMERCIAL E DISTRIBUIDORA LTDA",
			Credito:           "9",
			DescricaoCredito:  "SICREDI 77361-1",
			Valor:             "2645,39",
			Historico:         "ANAY FITAS COMERCIAL E DISTRIBUIDORA LTDA NF 380056 - 1E",
			ValorOriginal:     "2645,39",
			ValorPago:         "2645,39",
			ValorJuros:        "0,00",
			ValorMulta:        "4,76",
			ValorDesconto:     "0,00",
			ValorDespesas:     "0,00",
			VarCam:            "0,00",
			ValorLiqPagoBanco: "2650,15",
		},
	}

	out, err := svc.gerarCSVAtoliniPagamentos(rows)
	if err != nil {
		t.Fatalf("erro ao gerar CSV: %v", err)
	}

	reader := csv.NewReader(bytes.NewReader(out))
	reader.Comma = ';'

	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("erro ao ler CSV gerado: %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("esperado 2 linhas (header + dado), obtido %d", len(records))
	}

	header := records[0]
	row := records[1]

	if len(header) != 15 {
		t.Fatalf("header deveria ter 15 colunas, obteve %d", len(header))
	}
	if len(row) != 15 {
		t.Fatalf("linha de dados deveria ter 15 colunas, obteve %d", len(row))
	}

	if header[8] != "Valor Pago" {
		t.Fatalf("coluna 9 do header deveria ser 'Valor Pago', obteve %q", header[8])
	}
	if header[9] != "Valor Juros" || header[10] != "Valor Multa" {
		t.Fatalf("header desalinhado para juros/multa: %q / %q", header[9], header[10])
	}

	if row[8] != "2645,39" {
		t.Fatalf("Valor Pago desalinhado, obteve %q", row[8])
	}
	if row[9] != "0,00" {
		t.Fatalf("Valor Juros desalinhado, obteve %q", row[9])
	}
	if row[10] != "4,76" {
		t.Fatalf("Valor Multa desalinhado, obteve %q", row[10])
	}
}
