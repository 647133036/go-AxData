package sina

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewSinaAdapter(t *testing.T) {
	a := NewSinaAdapter()
	if a == nil {
		t.Fatal("Adapter is nil")
	}
	if a.Name() != "sina" {
		t.Errorf("Name: got %s, want sina", a.Name())
	}
}

func TestSinaRequestMissingSymbols(t *testing.T) {
	a := NewSinaAdapter()
	_, err := a.Request(context.Background(), map[string]interface{}{
		"interface": "real_time",
	})
	if err == nil {
		t.Fatal("Expected error for missing symbols")
	}
}

func TestSinaRequestUnknownInterface(t *testing.T) {
	a := NewSinaAdapter()
	_, err := a.Request(context.Background(), map[string]interface{}{
		"interface": "unknown",
		"symbols":   "sz000001",
	})
	if err == nil {
		t.Fatal("Expected error for unknown interface")
	}
}

func TestSinaRequestNoInterface(t *testing.T) {
	a := NewSinaAdapter()
	_, err := a.Request(context.Background(), map[string]interface{}{
		"symbols": "sz000001",
	})
	if err == nil {
		t.Fatal("Expected error for missing interface")
	}
}

func TestSina_requestRealTime_429(t *testing.T) {
	// Sina httpGet does not check HTTP status codes - it reads the body regardless.
	// A 429 returns empty body, parseRealTime returns 0 results.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
	}))
	defer server.Close()

	a := NewSinaAdapter()
	a.baseURL = server.URL

	results, err := a.requestRealTime(context.Background(), "sz000001")
	if err != nil {
		t.Fatalf("requestRealTime failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected 0 results from 429, got %d", len(results))
	}
}

func TestSina_requestRealTimeSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`var hq_str_sz000001="PingAnBank,14.50,14.00,14.60,140000,1050000,0.25,14.60,1000000,14.55,200000,14.50,300000,14.45,100000,14.40,50000,14.35,20000,13620400,200400000,2026-08-03,15:00:00,0.00,14.00,0.00,14.00,14.60,14.00,14.50,0.17,0.00,0.00,0.00,0.00,0.00";`))
	}))
	defer server.Close()

	a := NewSinaAdapter()
	a.baseURL = server.URL

	results, err := a.requestRealTime(context.Background(), "sz000001")
	if err != nil {
		t.Fatalf("requestRealTime failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}
	if results[0]["instrument_id"] != "000001.SZ" {
		t.Errorf("instrument_id: got %s, want 000001.SZ", results[0]["instrument_id"])
	}
}

func TestCleanSinaSymbol(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"000001", "sz000001"},
		{"600000", "sh600000"},
		{"430090", "bj430090"},
		{"sz000001", "sz000001"},
		{"SH600000", "sh600000"},
		{"000001.SZ", "sz000001"},
		{"600000.SH", "sh600000"},
		{"", ""},
	}

	for _, tc := range tests {
		result := cleanSinaSymbol(tc.input)
		if result != tc.want {
			t.Errorf("cleanSinaSymbol(%q): got %s, want %s", tc.input, result, tc.want)
		}
	}
}

func TestSinaToAxCode(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"sz000001", "000001.SZ"},
		{"sh600000", "600000.SH"},
		{"bj430090", "430090.BJ"},
		{"000001", "000001"},
	}

	for _, tc := range tests {
		result := sinaToAxCode(tc.input)
		if result != tc.want {
			t.Errorf("sinaToAxCode(%q): got %s, want %s", tc.input, result, tc.want)
		}
	}
}

func TestSinaFormatSymbols(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"000001", "sz000001"},
		{"000001,600000", "sz000001,sh600000"},
		{"000001.SZ,600000.SH", "sz000001,sh600000"},
	}

	for _, tc := range tests {
		result := sinaFormatSymbols(tc.input)
		if result != tc.want {
			t.Errorf("sinaFormatSymbols(%q): got %s, want %s", tc.input, result, tc.want)
		}
	}
}

func TestPeriodToScale(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"m1", "1"},
		{"5m", "5"},
		{"15min", "15"},
		{"60m", "60"},
		{"d", "240"},
		{"w", "10080"},
		{"M", "1440"},
		{"invalid", "240"},
	}

	for _, tc := range tests {
		result := periodToScale(tc.input)
		if result != tc.want {
			t.Errorf("periodToScale(%q): got %s, want %s", tc.input, result, tc.want)
		}
	}
}
