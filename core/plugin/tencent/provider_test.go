package tencent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewProvider(t *testing.T) {
	p := NewProvider()
	if p.ProviderID() != "axdata.source.tencent" {
		t.Errorf("ProviderID: got %s, want axdata.source.tencent", p.ProviderID())
	}
	if p.SourceCode() != "tencent" {
		t.Errorf("SourceCode: got %s, want tencent", p.SourceCode())
	}
	if p.Version() != "0.1.0" {
		t.Errorf("Version: got %s, want 0.1.0", p.Version())
	}
	ifaces := p.Interfaces()
	if len(ifaces) != 3 {
		t.Fatalf("Expected 3 interfaces, got %d", len(ifaces))
	}
}

func TestProvider_Interfaces(t *testing.T) {
	ifaces := NewProvider().Interfaces()
	names := make(map[string]bool)
	for _, iface := range ifaces {
		names[iface.Name] = true
		if iface.SourceCode != "tencent" {
			t.Errorf("SourceCode for %s: want tencent, got %s", iface.Name, iface.SourceCode)
		}
	}
	if !names["stock_quote_tencent"] {
		t.Error("Missing stock_quote_tencent")
	}
	if !names["stock_kline_tencent"] {
		t.Error("Missing stock_kline_tencent")
	}
	if !names["index_quote_tencent"] {
		t.Error("Missing index_quote_tencent")
	}
}

func TestProvider_CreateAdapter(t *testing.T) {
	adapter := NewProvider().CreateAdapter(nil)
	if adapter.Name() != "tencent" {
		t.Errorf("Name: got %s, want tencent", adapter.Name())
	}
	if adapter.Description() == "" {
		t.Error("Description should not be empty")
	}
}

func TestTencentAdapter_RequestMissingInterface(t *testing.T) {
	adapter := NewProvider().CreateAdapter(nil)
	_, err := adapter.Request(context.Background(), map[string]interface{}{
		"symbols": "600519.SH",
	})
	if err == nil {
		t.Fatal("Expected error for missing interface")
	}
}

func TestTencentAdapter_RequestMissingSymbols(t *testing.T) {
	adapter := NewProvider().CreateAdapter(nil)
	_, err := adapter.Request(context.Background(), map[string]interface{}{
		"interface": "stock_quote_tencent",
	})
	if err == nil {
		t.Fatal("Expected error for missing symbols")
	}
}

func TestTencentAdapter_RequestUnknownInterface(t *testing.T) {
	adapter := NewProvider().CreateAdapter(nil)
	_, err := adapter.Request(context.Background(), map[string]interface{}{
		"interface": "unknown_iface",
		"symbols":   "600519.SH",
	})
	if err == nil {
		t.Fatal("Expected error for unknown interface")
	}
}

func TestTencentAdapter_RequestQuote_429(t *testing.T) {
	// The tencent adapter does not check HTTP status codes; it reads the body regardless.
	// A 429 returns empty body → parseQuoteString returns 0 results (not an error).
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
	}))
	defer server.Close()

	a := &TencentAdapter{
		baseURL:     server.URL,
		client:      http.DefaultClient,
		description: "test",
	}

	results, err := a.Request(context.Background(), map[string]interface{}{
		"interface": "stock_quote_tencent",
		"symbols":   "600519.SH",
	})
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected 0 results from 429, got %d", len(results))
	}
}

func TestTencentAdapter_RequestQuoteSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Tencent quote uses ~ as delimiter
		w.Write([]byte(`v_sh600519="Moutai~100.00~100.00~105.00~110.00~101.00~1000000~100000~200000~100.01~100~100.02~200~100.03~300~100.04~400~100.05~500~100.06~600~100.07~700~100.08~800~100.09~900~100.10~1000~20260803140000~-0.38~-0.38~100.38~100000000~100000000~100~50";`))
	}))
	defer server.Close()

	a := &TencentAdapter{
		baseURL:     server.URL,
		client:      http.DefaultClient,
		description: "test",
	}

	results, err := a.Request(context.Background(), map[string]interface{}{
		"interface": "stock_quote_tencent",
		"symbols":   "600519.SH",
	})
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	t.Logf("Results: %+v", results)
	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}
	if results[0]["instrument_id"] != "600519.SH" {
		t.Errorf("instrument_id: got %s, want 600519.SH", results[0]["instrument_id"])
	}
}

func TestTencentAdapter_RequestKlineInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	}))
	defer server.Close()

	// The adapter hardcodes the kline URL; mock the http client via Do override
	// Instead, test the parseKlineJSON method directly.
	adapter := NewProvider().CreateAdapter(nil)
	a := adapter.(*TencentAdapter)
	_, err := a.parseKlineJSON([]byte(`not json`))
	if err == nil {
		t.Fatal("Expected error for invalid JSON")
	}
	_ = server
}

func TestTencentAdapter_RequestKlineNoData(t *testing.T) {
	adapter := NewProvider().CreateAdapter(nil)
	a := adapter.(*TencentAdapter)
	_, err := a.parseKlineJSON([]byte(`{"data": {"sh600519": {"other": []}}}`))
	if err == nil {
		t.Fatal("Expected error when kline data not found")
	}
}

func TestParseKlineJSON_Empty(t *testing.T) {
	adapter := NewProvider().CreateAdapter(nil)
	a := adapter.(*TencentAdapter)

	// parseKlineJSON returns an error when no kline data is found, not empty results
	_, err := a.parseKlineJSON([]byte(`{"data":{}}`))
	if err == nil {
		t.Fatal("Expected error for empty data")
	}
}

func TestTencentCleanSymbol(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"600519.SH", "sh600519"},
		{"000001.SZ", "sz000001"},
		{"sh600519", "sh600519"},
		{"000001", "sz000001"},
		{"600000", "sh600000"},
		{"  000001.SZ  ", "sz000001"},
	}
	for _, tt := range tests {
		got := tencentCleanSymbol(tt.input)
		if got != tt.want {
			t.Errorf("tencentCleanSymbol(%q) = %s, want %s", tt.input, got, tt.want)
		}
	}
}

func TestTencentFormatSymbols(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"600519.SH", "sh600519"},
		{"600519.SH,000001.SZ", "sh600519,sz000001"},
		{"000001,600000", "sz000001,sh600000"},
	}
	for _, tt := range tests {
		got := tencentFormatSymbols(tt.input)
		if got != tt.want {
			t.Errorf("tencentFormatSymbols(%q) = %s, want %s", tt.input, got, tt.want)
		}
	}
}

func TestTencentCodeToAxCode(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"sh600519", "600519.SH"},
		{"sz000001", "000001.SZ"},
		{"SH600519", "600519.SH"},
		{"sz000001", "000001.SZ"},
		{"800000", "800000"},
	}
	for _, tt := range tests {
		got := tencentCodeToAxCode(tt.input)
		if got != tt.want {
			t.Errorf("tencentCodeToAxCode(%q) = %s, want %s", tt.input, got, tt.want)
		}
	}
}

func TestTencentExtractCode(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"v_sh600519", "sh600519"},
		{"v_sz000001", "sz000001"},
	}
	for _, tt := range tests {
		got := tencentExtractCode(tt.input)
		if got != tt.want {
			t.Errorf("tencentExtractCode(%q) = %s, want %s", tt.input, got, tt.want)
		}
	}
}

func TestParseKlineJSON(t *testing.T) {
	adapter := NewProvider().CreateAdapter(nil)
	a := adapter.(*TencentAdapter)

	jsonData := `{
		"data": {
			"sh600519": {
				"qfqdaily": [
					"20260803,1330.03,1350.60,1361.76,1325.77,5512752,7373462605.00"
				]
			}
		}
	}`

	var raw map[string]interface{}
	json.Unmarshal([]byte(jsonData), &raw)

	results := a.parseKlineArray(raw["data"].(map[string]interface{})["sh600519"].(map[string]interface{})["qfqdaily"].([]interface{}))
	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}
	if results[0]["trade_date"] != "20260803" {
		t.Errorf("trade_date: got %v, want 20260803", results[0]["trade_date"])
	}
	if results[0]["close"] != "1350.60" {
		t.Errorf("close: got %v, want 1350.60", results[0]["close"])
	}
}
