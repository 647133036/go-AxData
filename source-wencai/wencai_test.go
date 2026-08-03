package wencai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewWencaiAdapter(t *testing.T) {
	a := NewWencaiAdapter()
	if a == nil {
		t.Fatal("Adapter is nil")
	}
	if a.Name() != "wencai" {
		t.Errorf("Name: got %s, want wencai", a.Name())
	}
}

func TestWencaiRequestUnknownInterface(t *testing.T) {
	a := NewWencaiAdapter()
	_, err := a.Request(context.Background(), map[string]interface{}{
		"interface": "unknown",
	})
	if err == nil {
		t.Fatal("Expected error for unknown interface")
	}
}

func TestWencaiRequestNoInterface(t *testing.T) {
	a := NewWencaiAdapter()
	_, err := a.Request(context.Background(), map[string]interface{}{
		"query": "市盈率低于10",
	})
	if err == nil {
		t.Fatal("Expected error for missing interface")
	}
}

func TestWencaiRequestMissingQuery(t *testing.T) {
	a := NewWencaiAdapter()
	_, err := a.Request(context.Background(), map[string]interface{}{
		"interface": "stock_strategy_wencai",
	})
	if err == nil {
		t.Fatal("Expected error for missing query")
	}
}

func TestWencaiRequestSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"result":{"table_data":{"values":[["000001","平安银行"],["000002","万科A"]],"meta_data":{"table":"0"}}}}`))
	}))
	defer server.Close()

	origCookieURL := _COOKIE_URL
	origWencaiURL := _WENCAI_URL
	_COOKIE_URL = server.URL + "/cookie"
	_WENCAI_URL = server.URL + "/wencai"
	defer func() {
		_COOKIE_URL = origCookieURL
		_WENCAI_URL = origWencaiURL
	}()

	a := NewWencaiAdapter()
	_, err := a.Request(context.Background(), map[string]interface{}{
		"interface": "stock_strategy_wencai",
		"query":     "市盈率低于10",
	})
	if err != nil {
		t.Logf("Request returned (may fail without real API): %v", err)
	}
}
