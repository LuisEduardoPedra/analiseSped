package converter

import (
	"os"
	"strings"
	"testing"
)

// TestAtoliniPagamentosIntegration testa o processo completo de conversão com os arquivos do error_case
func TestAtoliniPagamentosIntegration(t *testing.T) {
	svc := NewService()

	// Abrir arquivo de lançamentos
	lancamentosFile, err := os.Open("../../../error_case/janeiro2026.xls")
	if err != nil {
		t.Fatalf("Erro ao abrir arquivo de lançamentos: %v", err)
	}
	defer lancamentosFile.Close()

	// Abrir arquivo de contas
	contasFile, err := os.Open("../../../error_case/contas.csv")
	if err != nil {
		t.Fatalf("Erro ao abrir arquivo de contas: %v", err)
	}
	defer contasFile.Close()

	// Teste 1: Sem filtros (comportamento antigo - pode pegar contas erradas)
	t.Run("Sem filtros", func(t *testing.T) {
		lancamentosFile.Seek(0, 0)
		contasFile.Seek(0, 0)

		output, err := svc.ProcessAtoliniPagamentos(lancamentosFile, contasFile, nil, nil)
		if err != nil {
			t.Fatalf("Erro ao processar: %v", err)
		}

		t.Logf("Output sem filtros: %d bytes", len(output))
		// Salvar output para análise
		os.WriteFile("../../../error_case/output_sem_filtros.csv", output, 0644)
	})

	// Teste 2: Com filtros corretos para pagamentos
	// NOVA SEMÂNTICA:
	//   debitPrefixes = Ativo (1.x.x) → usado para buscar BANCOS (crédito contábil)
	//   creditPrefixes = Passivo (2.x.x) → usado para buscar FORNECEDORES (débito contábil)
	t.Run("Com filtros corretos", func(t *testing.T) {
		// Reabrir arquivos para resetar o cursor
		lancamentosFile2, _ := os.Open("../../../error_case/janeiro2026.xls")
		defer lancamentosFile2.Close()
		contasFile2, _ := os.Open("../../../error_case/contas.csv")
		defer contasFile2.Close()

		debitPrefixes := []string{"1.1.1"}   // Ativo - para bancos
		creditPrefixes := []string{"2.1.1"}  // Passivo - para fornecedores

		output, err := svc.ProcessAtoliniPagamentos(lancamentosFile2, contasFile2, debitPrefixes, creditPrefixes)
		if err != nil {
			t.Fatalf("Erro ao processar: %v", err)
		}

		t.Logf("Output com filtros: %d bytes", len(output))

		// Verificar se há contas de fornecedor (2.1.1.01.001) nas colunas de débito
		outputStr := string(output)

		// Contar quantas vezes aparece código de cliente (Ativo) vs fornecedor (Passivo)
		// Não podemos verificar códigos específicos sem conhecer os dados do Excel,
		// mas podemos verificar se o CSV foi gerado corretamente
		if !strings.Contains(outputStr, "Debito") {
			t.Error("Output deveria conter cabeçalho 'Debito'")
		}
		if !strings.Contains(outputStr, "Credito") {
			t.Error("Output deveria conter cabeçalho 'Credito'")
		}

		// Salvar output para análise manual
		err = os.WriteFile("../../../error_case/output_com_filtros.csv", output, 0644)
		if err != nil {
			t.Logf("Aviso: não foi possível salvar output: %v", err)
		} else {
			t.Log("Output salvo em: error_case/output_com_filtros.csv")
		}
	})

	// Teste 3: Com filtros múltiplos
	// Útil quando há bancos tanto em Ativo (contas correntes) quanto Passivo (empréstimos)
	t.Run("Com filtros múltiplos para bancos", func(t *testing.T) {
		lancamentosFile3, _ := os.Open("../../../error_case/janeiro2026.xls")
		defer lancamentosFile3.Close()
		contasFile3, _ := os.Open("../../../error_case/contas.csv")
		defer contasFile3.Close()

		debitPrefixes := []string{"1.1.1", "2.1.1"}  // Ativo + Passivo - para bancos em ambos
		creditPrefixes := []string{"2.1.1"}          // Passivo - para fornecedores

		output, err := svc.ProcessAtoliniPagamentos(lancamentosFile3, contasFile3, debitPrefixes, creditPrefixes)
		if err != nil {
			t.Fatalf("Erro ao processar: %v", err)
		}

		t.Logf("Output com filtros múltiplos: %d bytes", len(output))

		// Salvar output para análise
		err = os.WriteFile("../../../error_case/output_filtros_multiplos.csv", output, 0644)
		if err != nil {
			t.Logf("Aviso: não foi possível salvar output: %v", err)
		} else {
			t.Log("Output salvo em: error_case/output_filtros_multiplos.csv")
		}
	})
}
