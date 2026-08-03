package tencent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTencentAdapter_RequestQuoteViaHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("Request path: %s", r.URL.Path)
		w.Write([]byte(`v_sh600519="Moutai~100.00~100.00~105.00~110.00~101.00~1000000~100000~200000~100.01~100~100.02~200~100.03~300~100.04~400~100.05~500~100.06~600~100.07~700~100.08~800~100.09~900~100.10~1000~20260803140000~-0.38~-0.38~100.38~100000000~100000000~100~50";`))
	}))
	defer server.Close()

	a := NewProvider().CreateAdapter(nil).(*TencentAdapter)
	a.baseURL = server.URL

	// Direct parse test
	raw := "v_sh600519=\"Moutai~100.00~100.00~105.00~110.00~101.00~1000000~100000~200000~100.01~100~100.02~200~100.03~300~100.04~400~100.05~500~100.06~600~100.07~700~100.08~800~100.09~900~100.10~1000~20260803140000~-0.38~-0.38~100.38~100000000~100000000~100~50\";"
	directResults, _ := a.parseQuoteString(raw)
	if len(directResults) != 1 {
		t.Fatalf("Direct parse: expected 1, got %d", len(directResults))
	}

	// Full Request via HTTP
	results, err := a.Request(context.Background(), map[string]interface{}{
		"interface": "stock_quote_tencent",
		"symbols":   "600519.SH",
	})
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("HTTP Request: expected 1, got %d", len(results))
	}
	if results[0]["instrument_id"] != "600519.SH" {
		t.Errorf("instrument_id: got %s", results[0]["instrument_id"])
	}
}

func TestTencentAdapter_IndexQuote(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Index quote returns same format as stock quote
		w.Write([]byte(`v_sh000001="上证指数~3833.54~3804.69~3847.09~3822.37~100.00~597529427~27418~27709~3833.50~100~3833.60~200~3833.70~300~3833.80~400~3833.90~500~3834.00~600~3834.10~700~3834.20~800~3834.30~900~3834.40~1000~20260803140000~28.85~0.76~3804.69~0~0~0~0~0";`))
	}))
	defer server.Close()

	a := NewProvider().CreateAdapter(nil).(*TencentAdapter)
	a.baseURL = server.URL

	results, err := a.Request(context.Background(), map[string]interface{}{
		"interface": "index_quote_tencent",
		"symbols":   "000001.SH",
	})
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}
}
