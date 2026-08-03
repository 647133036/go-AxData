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
