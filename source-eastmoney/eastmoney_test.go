package eastmoney

import (
	"context"
	"testing"
)

func TestNewEastMoneyAdapter(t *testing.T) {
	a := NewEastMoneyAdapter()
	if a == nil {
		t.Fatal("Adapter is nil")
	}
	if a.Name() != "eastmoney" {
		t.Errorf("Name: got %s, want eastmoney", a.Name())
	}
}

func TestEastMoneyRequestUnknownInterface(t *testing.T) {
	a := NewEastMoneyAdapter()
	_, err := a.Request(context.Background(), map[string]interface{}{
		"interface": "unknown_interface",
	})
	if err == nil {
		t.Fatal("Expected error for unknown interface")
	}
}

func TestEastMoneyRequestNoInterface(t *testing.T) {
	a := NewEastMoneyAdapter()
	_, err := a.Request(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Fatal("Expected error for missing interface")
	}
}

func TestParseCodeList(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"000001,000002", []string{"000001", "000002"}},
		{"000001，000002", []string{"000001", "000002"}},
		{"000001 000002", []string{"000001", "000002"}},
		{"000001.SH,000002.SZ,000003.BJ", []string{"000001", "000002", "000003"}},
		{"SH000001", []string{"000001"}},
		{"", []string{}},
		{"000001,invalid,000002", []string{"000001", "000002"}},
	}

	for _, tc := range tests {
		result := parseCodeList(tc.input)
		if len(result) != len(tc.want) {
			t.Errorf("parseCodeList(%q): got %v, want %v", tc.input, result, tc.want)
			continue
		}
		for i, code := range result {
			if code != tc.want[i] {
				t.Errorf("parseCodeList(%q)[%d]: got %s, want %s", tc.input, i, code, tc.want[i])
			}
		}
	}
}

func TestGetParamString(t *testing.T) {
	params := map[string]interface{}{
		"key1": "value1",
	}
	if getParamString(params, "key1", "default") != "value1" {
		t.Errorf("getParamString: got unexpected value")
	}
	if getParamString(params, "missing", "default") != "default" {
		t.Errorf("getParamString: got unexpected default")
	}
	if getParamString(params, "empty", "default") != "default" {
		params["empty"] = ""
		t.Errorf("getParamString: got unexpected default")
	}
}

func TestGetParamInt(t *testing.T) {
	params := map[string]interface{}{
		"intVal": 42,
		"strVal": "7",
	}
	if getParamInt(params, "intVal", 0) != 42 {
		t.Errorf("getParamInt(int): got unexpected value")
	}
	if getParamInt(params, "strVal", 0) != 7 {
		t.Errorf("getParamInt(string): got unexpected value")
	}
	if getParamInt(params, "missing", 99) != 99 {
		t.Errorf("getParamInt(missing): got unexpected value")
	}
}

func TestEastMoneyRequestInterfaceNames(t *testing.T) {
	a := NewEastMoneyAdapter()
	supported := []string{
		"eastmoney_market_index_realtime",
		"eastmoney_market_index_all_em",
		"eastmoney_stocks_all_em",
		"eastmoney_stock_realtime_snapshot",
		"eastmoney_sector_realtime",
		"eastmoney_limit_up_pool",
		"eastmoney_is_trade_day",
	}
	for _, name := range supported {
		if !supportedInterfaces[name] {
			t.Errorf("Expected %s to be supported", name)
		}
		// Just verify dispatch doesn't panic
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel to fail fast without network
		_, err := a.Request(ctx, map[string]interface{}{"interface": name})
		if err == nil {
			// It's OK if error is nil (might have been mocked)
		}
	}
}
