package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/electkismet/axdata-go/core/config"
	"github.com/electkismet/axdata-go/core/plugin"
	"github.com/electkismet/axdata-go/core/storage"
	"go.uber.org/zap"
)

func TestHealthHandler(t *testing.T) {
	s := &APIServer{
		cfg:    config.DefaultConfig("/tmp/test"),
		logger: zap.NewNop(),
	}

	req := httptest.NewRequest("GET", "/v1/health", nil)
	rw := httptest.NewRecorder()
	s.healthHandler(rw, req)

	resp := rw.Result()
	if resp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)

	if body["status"] != "ok" {
		t.Errorf("status: got %v, want ok", body["status"])
	}
	if body["version"] != "1.0.0" {
		t.Errorf("version: got %v, want 1.0.0", body["version"])
	}
}

func TestSchemaHandler(t *testing.T) {
	s := &APIServer{
		cfg:    config.DefaultConfig("/tmp/test"),
		logger: zap.NewNop(),
	}

	req := httptest.NewRequest("GET", "/v1/schema", nil)
	rw := httptest.NewRecorder()
	s.schemaHandler(rw, req)

	resp := rw.Result()
	if resp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d", resp.StatusCode)
	}

	var schemas map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&schemas)

	if len(schemas) == 0 {
		t.Fatal("Expected non-empty schemas")
	}

	// Check that "daily" exists
	if _, ok := schemas["daily"]; !ok {
		t.Error("Missing 'daily' in schemas")
	}
}

func TestTablesHandler(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.DefaultConfig(tmpDir)
	store := storage.NewStore(cfg)

	s := &APIServer{
		cfg:    cfg,
		store:  store,
		logger: zap.NewNop(),
	}

	req := httptest.NewRequest("GET", "/v1/tables", nil)
	rw := httptest.NewRecorder()
	s.tablesHandler(rw, req)

	resp := rw.Result()
	if resp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d", resp.StatusCode)
	}

	var tables []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&tables)

	if len(tables) == 0 {
		t.Fatal("Expected non-empty tables")
	}

	// Each table should have "name" field
	for _, table := range tables {
		if _, ok := table["name"]; !ok {
			t.Error("Table missing 'name' field")
		}
	}
}

func TestNewAPIServer(t *testing.T) {
	cfg := config.DefaultConfig("/tmp/test")
	pm := plugin.NewPluginManager("/tmp/test")

	s := NewAPIServer(cfg, nil, nil, nil, zap.NewNop(), pm)
	if s == nil {
		t.Fatal("NewAPIServer returned nil")
	}
	if s.cfg != cfg {
		t.Error("cfg not set")
	}
}

func TestRegisterRoutes(t *testing.T) {
	cfg := config.DefaultConfig("/tmp/test")
	pm := plugin.NewPluginManager("/tmp/test")
	s := NewAPIServer(cfg, nil, nil, nil, zap.NewNop(), pm)

	mux := http.NewServeMux()
	s.RegisterRoutes(mux)

	// Verify health route is registered
	req := httptest.NewRequest("GET", "/v1/health", nil)
	rw := httptest.NewRecorder()
	mux.ServeHTTP(rw, req)

	resp := rw.Result()
	if resp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Fatalf("Expected status ok, got %v", body["status"])
	}
}
