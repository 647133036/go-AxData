package source

import "testing"

func TestProviderRegistryContainsAllSources(t *testing.T) {
	if len(ProviderRegistry) == 0 {
		t.Fatal("ProviderRegistry is empty")
	}

	// Every major source code must be registered at least once. Exact entry
	// counts are deliberately not asserted: they break every time a provider
	// is added.
	expectedSources := []string{
		"tdx", "cls", "cninfo", "eastmoney", "kph", "mock", "sina", "tencent", "ths", "wencai",
	}

	actual := map[string]int{}
	for _, pi := range ProviderRegistry {
		actual[pi.SourceCode]++
	}

	for _, src := range expectedSources {
		if actual[src] == 0 {
			t.Errorf("Missing source code: %s", src)
		}
	}
}

func TestAnalystSuiteProvidersRegistered(t *testing.T) {
	// Each interface the analysis modules call must resolve to a real table
	// with the write mode the modules expect.
	expected := map[string]struct {
		table     string
		writeMode string
	}{
		"eastmoney_financial_income":   {"fin_income", "snapshot"},
		"eastmoney_financial_balance":  {"fin_balance", "snapshot"},
		"eastmoney_financial_cashflow": {"fin_cashflow", "snapshot"},
		"eastmoney_business_scope":     {"business_scope", "snapshot"},
		"eastmoney_earnings_forecast":  {"earnings_forecast", "snapshot"},
		"eastmoney_valuation_snapshot": {"valuation_snapshot", "snapshot"},
	}

	for name, want := range expected {
		pi, ok := ProviderRegistry[name]
		if !ok {
			t.Errorf("Missing provider interface: %s", name)
			continue
		}
		if pi.SourceCode != "eastmoney" {
			t.Errorf("%s: source = %q, want eastmoney", name, pi.SourceCode)
		}
		if pi.Table != want.table {
			t.Errorf("%s: table = %q, want %q", name, pi.Table, want.table)
		}
		if pi.WriteMode != want.writeMode {
			t.Errorf("%s: write mode = %q, want %q", name, pi.WriteMode, want.writeMode)
		}
	}
}
