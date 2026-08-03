package kph

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewKPHAdapter(t *testing.T) {
	a := NewKPHAdapter()
	if a == nil {
		t.Fatal("Adapter is nil")
	}
	if a.Name() != "kph" {
		t.Errorf("Name: got %s, want kph", a.Name())
	}
}

func TestKPHRequestMissingInterface(t *testing.T) {
	a := NewKPHAdapter()
	_, err := a.Request(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Fatal("Expected error for missing interface")
	}
}

func TestKPHRequestUnknownInterface(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"0","data":{}}`))
	}))
	defer server.Close()

	origRealtimeURL := KPH_REALTIME_URL
	KPH_REALTIME_URL = server.URL
	defer func() { KPH_REALTIME_URL = origRealtimeURL }()

	a := NewKPHAdapter()
	_, err := a.Request(context.Background(), map[string]interface{}{
		"interface": "unknown_interface",
	})
	if err == nil {
		t.Fatal("Expected error for unknown interface")
	}
}

func TestSectorTypeMap(t *testing.T) {
	tests := map[string]string{
		"selected": "7",
		"industry": "4",
		"region":   "6",
	}
	for k, v := range tests {
		if sectorTypeMap[k] != v {
			t.Errorf("sectorTypeMap[%s]: got %s, want %s", k, sectorTypeMap[k], v)
		}
	}
}

func TestLimitPID(t *testing.T) {
	tests := map[string]string{
		"up":        "4",
		"down":      "3",
		"wind_vane": "6",
	}
	for k, v := range tests {
		if limitPID[k] != v {
			t.Errorf("limitPID[%s]: got %s, want %s", k, limitPID[k], v)
		}
	}
}
