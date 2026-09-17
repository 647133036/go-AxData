package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	c := DefaultConfig("/tmp/axdata-test")
	if c.DataRoot != "/tmp/axdata-test" {
		t.Errorf("DataRoot: got %s, want /tmp/axdata-test", c.DataRoot)
	}
	if c.LogLevel != "info" {
		t.Errorf("LogLevel: got %s, want info", c.LogLevel)
	}
	if c.APIPort != 8080 {
		t.Errorf("APIPort: got %d, want 8080", c.APIPort)
	}
	if c.Collector.MaxConcurrentTasks != 4 {
		t.Errorf("MaxConcurrentTasks: got %d, want 4", c.Collector.MaxConcurrentTasks)
	}
	if c.Collector.TimeoutMs != 10000 {
		t.Errorf("TimeoutMs: got %d, want 10000", c.Collector.TimeoutMs)
	}
}

func TestConfig_resolvePaths(t *testing.T) {
	c := DefaultConfig("/tmp/axdata-test")
	if c.DataLayers.Core != filepath.Join("/tmp/axdata-test", "data", "core") {
		t.Errorf("DataLayers.Core: got %s", c.DataLayers.Core)
	}
	if c.Metadata.DBPath != filepath.Join("/tmp/axdata-test", "metadata", "axdata.sqlite") {
		t.Errorf("DBPath: got %s", c.Metadata.DBPath)
	}
}

func TestConfig_DataDir(t *testing.T) {
	c := DefaultConfig("/tmp/axdata-test")
	tests := []struct {
		layer string
		want  string
	}{
		{"raw", filepath.Join("/tmp/axdata-test", "data", "raw")},
		{"staging", filepath.Join("/tmp/axdata-test", "data", "staging")},
		{"factor", filepath.Join("/tmp/axdata-test", "data", "factor")},
		{"unknown", filepath.Join("/tmp/axdata-test", "data", "core")},
	}
	for _, tt := range tests {
		if got := c.DataDir(tt.layer); got != tt.want {
			t.Errorf("DataDir(%q) = %s, want %s", tt.layer, got, tt.want)
		}
	}
}

func TestConfig_CorePath(t *testing.T) {
	c := DefaultConfig("/tmp/axdata-test")
	want := filepath.Join("/tmp/axdata-test", "data", "core", "daily.parquet")
	if got := c.CorePath("daily"); got != want {
		t.Errorf("CorePath: got %s, want %s", got, want)
	}
}

func TestConfig_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	c1 := DefaultConfig(tmpDir)
	c1.LogLevel = "debug"
	c1.APIPort = 9090
	if err := c1.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	c2, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if c2.LogLevel != "debug" {
		t.Errorf("LogLevel: got %s, want debug", c2.LogLevel)
	}
	if c2.APIPort != 9090 {
		t.Errorf("APIPort: got %d, want 9090", c2.APIPort)
	}
}

func TestConfig_LoadNoFile(t *testing.T) {
	tmpDir := t.TempDir()
	c, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if c.LogLevel != "info" {
		t.Errorf("LogLevel: got %s, want info", c.LogLevel)
	}
}

func TestConfig_LoadInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	dir := filepath.Join(tmpDir, "metadata")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "config.json"), []byte("{invalid"), 0644)
	_, err := Load(tmpDir)
	if err == nil {
		t.Fatal("Expected error for invalid JSON")
	}
}

func TestConfig_LoadSnakeCaseKeys(t *testing.T) {
	dir := t.TempDir()
	meta := filepath.Join(dir, "metadata")
	if err := os.MkdirAll(meta, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"scheduler":{"enabled":true,"poll_interval_ms":250},
		"collector":{"max_concurrent_tasks":7,"request_interval_ms":1500},
		"data_root":"` + dir + `","log_level":"debug","api_port":9999,
		"metadata":{"db_path":"/tmp/x.duckdb","collector_path":"/tmp/c.json","plugins_path":"/tmp/p.json"}}`
	if err := os.WriteFile(filepath.Join(meta, "config.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Underscore keys were silently dropped before the json tags existed, so
	// every one of these used to come back as the default.
	if !cfg.Scheduler.Enabled {
		t.Error("scheduler.enabled was not loaded")
	}
	if cfg.Scheduler.PollIntervalMs != 250 {
		t.Errorf("poll_interval_ms = %d, want 250", cfg.Scheduler.PollIntervalMs)
	}
	if cfg.Collector.MaxConcurrentTasks != 7 {
		t.Errorf("max_concurrent_tasks = %d, want 7", cfg.Collector.MaxConcurrentTasks)
	}
	if cfg.Collector.RequestIntervalMs != 1500 {
		t.Errorf("request_interval_ms = %d, want 1500", cfg.Collector.RequestIntervalMs)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("log_level = %q, want debug", cfg.LogLevel)
	}
	if cfg.APIPort != 9999 {
		t.Errorf("api_port = %d, want 9999", cfg.APIPort)
	}
	if cfg.Metadata.CollectorPath != "/tmp/c.json" {
		t.Errorf("collector_path = %q, want /tmp/c.json", cfg.Metadata.CollectorPath)
	}
}

func TestConfig_LoadLegacyPascalCaseKeys(t *testing.T) {
	dir := t.TempDir()
	meta := filepath.Join(dir, "metadata")
	if err := os.MkdirAll(meta, 0o755); err != nil {
		t.Fatal(err)
	}
	// Written before Config had json tags, when Save marshalled Go field names.
	body := `{"Scheduler":{"Enabled":true,"PollIntervalMs":300},
		"Collector":{"MaxConcurrentTasks":5,"TimeoutMs":42},
		"APIPort":7777,"LogLevel":"warn"}`
	if err := os.WriteFile(filepath.Join(meta, "config.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Scheduler.Enabled || cfg.Scheduler.PollIntervalMs != 300 {
		t.Errorf("scheduler = %+v, want enabled with 300ms", cfg.Scheduler)
	}
	if cfg.Collector.MaxConcurrentTasks != 5 || cfg.Collector.TimeoutMs != 42 {
		t.Errorf("collector = %+v, want 5 / 42", cfg.Collector)
	}
	if cfg.APIPort != 7777 || cfg.LogLevel != "warn" {
		t.Errorf("APIPort=%d LogLevel=%q", cfg.APIPort, cfg.LogLevel)
	}
}

func TestToSnakeCase(t *testing.T) {
	tests := map[string]string{
		"DataRoot":           "data_root",
		"APIPort":            "api_port",
		"PollIntervalMs":     "poll_interval_ms",
		"DBPath":             "db_path",
		"CollectorPath":      "collector_path",
		"MaxConcurrentTasks": "max_concurrent_tasks",
		"enabled":            "enabled",
		"data_root":          "data_root",
	}
	for in, want := range tests {
		if got := toSnakeCase(in); got != want {
			t.Errorf("toSnakeCase(%q) = %q, want %q", in, got, want)
		}
	}
}
