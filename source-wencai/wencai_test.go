package wencai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "cookie") {
			w.Write([]byte(`{"code":"000000","data":"test-cookie-123","message":"ok"}`))
		} else {
			w.Write([]byte(`{"success":true,"message":"","data":{"result":{"title":["代码","名称","最新价","涨跌幅"],"result":[["000001","平安银行",12.34,2.5],["000002","万科A",30.00,-1.2]]}}}`))
		}
	})

	server := httptest.NewServer(handler)
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
	rows, err := a.Request(context.Background(), map[string]interface{}{
		"interface": "stock_strategy_wencai",
		"query":     "市盈率低于10",
	})
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("Expected 2 rows, got %d", len(rows))
	}

	// Verify first row has title metadata
	firstRow := rows[0]
	if _, ok := firstRow["_title"]; !ok {
		t.Fatal("First row missing _title metadata")
	}

	// Verify field names mapped from title
	if v, ok := firstRow["代码"]; !ok {
		t.Fatal("Missing '代码' field")
	} else if v != "000001" {
		t.Errorf("Expected code 000001, got %v", v)
	}

	// Verify second row
	secondRow := rows[1]
	if name, ok := secondRow["名称"]; !ok {
		t.Fatal("Missing '名称' field on second row")
	} else if name != "万科A" {
		t.Errorf("Expected name 万科A, got %v", name)
	}
}

func TestWencaiRequestEmptyResult(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "cookie") {
			w.Write([]byte(`{"code":"000000","data":"test-cookie","message":"ok"}`))
		} else {
			w.Write([]byte(`{"success":true,"message":"","data":{"result":{"title":[],"result":[]}}}`))
		}
	})

	server := httptest.NewServer(handler)
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
	rows, err := a.Request(context.Background(), map[string]interface{}{
		"interface": "stock_strategy_wencai",
		"query":     "不存在",
	})
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Empty result should return a single metadata row
	if len(rows) != 1 {
		t.Fatalf("Expected 1 metadata row, got %d", len(rows))
	}
	if _, ok := rows[0]["_raw"]; !ok {
		t.Fatal("Metadata row missing _raw")
	}
}

func TestWencaiRequestCookieFailure(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"code":"500","data":"","message":"server error"}`))
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	origCookieURL := _COOKIE_URL
	origWencaiURL := _WENCAI_URL
	_COOKIE_URL = server.URL
	_WENCAI_URL = server.URL
	defer func() {
		_COOKIE_URL = origCookieURL
		_WENCAI_URL = origWencaiURL
	}()

	a := NewWencaiAdapter()
	_, err := a.Request(context.Background(), map[string]interface{}{
		"interface": "stock_strategy_wencai",
		"query":     "测试",
	})
	if err == nil {
		t.Fatal("Expected error for cookie failure")
	}
	if !strings.Contains(err.Error(), "cookie error") {
		t.Errorf("Expected cookie error, got: %v", err)
	}
}

func TestWencaiRequestPageAndLimit(t *testing.T) {
	receivedPath := ""
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "cookie") {
			receivedPath = r.URL.RawQuery
		}
		w.Write([]byte(`{"code":"000000","data":"ok","message":"ok"}`))
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	origCookieURL := _COOKIE_URL
	origWencaiURL := _WENCAI_URL
	_COOKIE_URL = server.URL
	_WENCAI_URL = server.URL + "/wencai"
	defer func() {
		_COOKIE_URL = origCookieURL
		_WENCAI_URL = origWencaiURL
	}()

	a := NewWencaiAdapter()
	_, err := a.Request(context.Background(), map[string]interface{}{
		"interface": "stock_strategy_wencai",
		"query":     "测试",
		"page":      3,
		"limit":     100,
	})
	// Cookie will fail, but we can still check the query params were constructed
	// before the failure (by checking that the wencai URL was called with correct params)
	_ = err // cookie fails, that's expected

	// Check that the query params included page=3 and perpage=100
	if !strings.Contains(receivedPath, "page=3") {
		t.Fatalf("Expected page=3 in query, got: %s", receivedPath)
	}
	if !strings.Contains(receivedPath, "perpage=100") {
		t.Fatalf("Expected perpage=100 in query, got: %s", receivedPath)
	}
}
