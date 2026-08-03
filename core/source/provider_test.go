package source

import "testing"

func TestProviderRegistryContainsAllSources(t *testing.T) {
	// Verify ProviderRegistry is not empty
	if len(ProviderRegistry) == 0 {
		t.Fatal("ProviderRegistry is empty")
	}

	// Check that we have entries for all major source codes
	expectedSources := map[string]int{
		"tdx": 8,
		"cls": 17,
		"cninfo": 32,
		"eastmoney": 19,
		"kph": 11,
		"mock": 1,
		"sina": 4,
		"tencent": 5,
		"ths": 1,
		"wencai": 1,
	}

	actual := map[string]int{}
	for _, pi := range ProviderRegistry {
		actual[pi.SourceCode]++
	}

	for src, count := range expectedSources {
		got, ok := actual[src]
		if !ok {
			t.Errorf("Missing source code: %s", src)
			continue
		}
		if got != count {
			t.Errorf("Source %s: got %d entries, want %d", src, got, count)
		}
	}
}
