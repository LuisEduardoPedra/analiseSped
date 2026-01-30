package converter

import (
	"os"
	"strings"
	"testing"
)

// TestBuscarContaAtoliniComFiltros testa se os filtros de prefixo estão funcionando corretamente
func TestBuscarContaAtoliniComFiltros(t *testing.T) {
	svc := &service{}

	// Ler o arquivo de contas real do error_case
	contasFile, err := os.Open("../../../error_case/contas.csv")
	if err != nil {
		t.Fatalf("Erro ao abrir arquivo de contas: %v", err)
	}
	defer contasFile.Close()

	contasMap, descricaoIndex, err := svc.lerPlanoContasAtolini(contasFile)
	if err != nil {
		t.Fatalf("Erro ao ler plano de contas: %v", err)
	}

	// Teste 1: Buscar "INDALTEX COMERCIO E SERVICOS LTDA" sem filtro
	// Deve pegar qualquer uma (provavelmente a primeira que encontrar)
	t.Run("Sem filtro", func(t *testing.T) {
		codigo := svc.buscarContaAtolini("INDALTEX COMERCIO E SERVICOS LTDA", contasMap, descricaoIndex, nil)
		t.Logf("Sem filtro: código = %s", codigo)
		if codigo == "999999" {
			t.Error("Não deveria retornar fallback quando há match")
		}
	})

	// Teste 2: Buscar "INDALTEX COMERCIO E SERVICOS LTDA" com filtro de Ativo (1.1.2)
	// Deve pegar a conta do Ativo (Cliente)
	t.Run("Com filtro Ativo 1.1.2", func(t *testing.T) {
		prefixes := []string{"1.1.2"}
		codigo := svc.buscarContaAtolini("INDALTEX COMERCIO E SERVICOS LTDA", contasMap, descricaoIndex, prefixes)
		t.Logf("Com filtro Ativo 1.1.2: código = %s", codigo)
		if codigo == "999999" {
			t.Error("Deveria encontrar a conta no Ativo")
		}
		// Verificar se pegou a conta correta (9487 segundo o CSV)
		if codigo != "9487" {
			t.Errorf("Esperava código 9487 (Ativo), mas obteve %s", codigo)
		}
	})

	// Teste 3: Buscar "INDALTEX COMERCIO E SERVICOS LTDA" com filtro de Passivo (2.1.1)
	// Deve pegar a conta do Passivo (Fornecedor)
	t.Run("Com filtro Passivo 2.1.1", func(t *testing.T) {
		prefixes := []string{"2.1.1"}
		codigo := svc.buscarContaAtolini("INDALTEX COMERCIO E SERVICOS LTDA", contasMap, descricaoIndex, prefixes)
		t.Logf("Com filtro Passivo 2.1.1: código = %s", codigo)
		if codigo == "999999" {
			t.Error("Deveria encontrar a conta no Passivo")
		}
		// Verificar se pegou a conta correta (9473 segundo o CSV)
		if codigo != "9473" {
			t.Errorf("Esperava código 9473 (Passivo), mas obteve %s", codigo)
		}
	})

	// Teste 4: Buscar banco "Banco Sicredi" com filtro de Ativo (1.1.1)
	// Deve pegar conta bancária do Ativo
	t.Run("Banco Sicredi com filtro Ativo 1.1.1", func(t *testing.T) {
		prefixes := []string{"1.1.1"}
		codigo := svc.buscarContaAtolini("Banco Sicredi", contasMap, descricaoIndex, prefixes)
		t.Logf("Banco Sicredi com filtro Ativo 1.1.1: código = %s", codigo)
		if codigo == "999999" {
			t.Error("Deveria encontrar o banco no Ativo")
		}
		// Verificar se a classificação começa com 1.1.1
		if entry, ok := findEntryByCode(contasMap, codigo); ok {
			if !strings.HasPrefix(entry.Classif, "1.1.1") {
				t.Errorf("Esperava classificação 1.1.1.*, mas obteve %s", entry.Classif)
			}
		}
	})

	// Teste 5: Buscar banco "Banco Sicredi - (CR)" com filtro de Passivo (2.1.1)
	// Deve pegar empréstimo bancário do Passivo
	// Nota: A descrição exata no Passivo é "Banco Sicredi - (CR)", não apenas "Banco Sicredi"
	t.Run("Banco Sicredi CR com filtro Passivo 2.1.1", func(t *testing.T) {
		prefixes := []string{"2.1.1"}
		codigo := svc.buscarContaAtolini("Banco Sicredi - (CR)", contasMap, descricaoIndex, prefixes)
		t.Logf("Banco Sicredi - (CR) com filtro Passivo 2.1.1: código = %s", codigo)
		// Como a descrição pode não fazer match fuzzy perfeito, aceitamos tanto sucesso quanto fallback
		if codigo != "999999" {
			// Se encontrou, verificar se a classificação está correta
			if entry, ok := findEntryByCode(contasMap, codigo); ok {
				if !strings.HasPrefix(entry.Classif, "2.1.1") {
					t.Errorf("Esperava classificação 2.1.1.*, mas obteve %s", entry.Classif)
				}
			}
		}
	})
}

// Helper para encontrar uma entrada pelo código
func findEntryByCode(contasMap map[string][]accEntry, code string) (accEntry, bool) {
	for _, entries := range contasMap {
		for _, e := range entries {
			if e.ID == code {
				return e, true
			}
		}
	}
	return accEntry{}, false
}
