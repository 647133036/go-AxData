package collector

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/electkismet/axdata-go/core/config"
	"github.com/electkismet/axdata-go/core/schema"
	"github.com/electkismet/axdata-go/core/source"
	"github.com/electkismet/axdata-go/core/storage"
	"go.uber.org/zap"
)

func TestApplyFieldMapping(t *testing.T) {
	tests := []struct {
		name      string
		tableName string
		data      map[string]interface{}
		wantKeys  []string
	}{
		{
			name:      "tencent daily direct keys",
			tableName: "daily",
			data: map[string]interface{}{
				"ts_code": "000001.SZ",
				"vol":     float64(1000.0),
				"pct_chg": float64(2.5),
			},
			wantKeys: []string{"ts_code", "vol", "pct_chg"},
		},
		{
			name:      "direct column match",
			tableName: "stock_basic_exchange",
			data: map[string]interface{}{
				"instrument_id": "000001.SZ",
				"symbol":        "000001",
			},
			wantKeys: []string{"instrument_id", "symbol"},
		},
		{
			name:      "no mapping needed",
			tableName: "stock_hot_rank",
			data: map[string]interface{}{
				"source": "mock",
				"rank":   int64(1),
			},
			wantKeys: []string{"source", "rank"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := applyFieldMapping(tc.data, tc.tableName)
			for _, key := range tc.wantKeys {
				if _, ok := result[key]; !ok {
					t.Errorf("Missing expected key: %s in result: %v", key, result)
				}
			}
		})
	}
}

func TestBuildDailyRecord(t *testing.T) {
	// ProviderRegistry "daily" entries may include field mappings (e.g., tdx maps
	// instrument_id->ts_code, change_pct->pct_chg, volume->vol). If fieldMapping
	// exists but none of its keys match the input data, the data won't be mapped.
	// Pass ts_code/pct_chg/vol directly so they pass through as schema columns.
	data := map[string]interface{}{
		"ts_code":  "000001.SZ",
		"vol":      float64(1000.0),
		"pct_chg":  float64(2.5),
		"open":     float64(10.0),
	}
	result := buildDailyRecord(data)
	dr, ok := result.(schema.DailyRecord)
	if !ok {
		t.Fatalf("Expected DailyRecord, got %T", result)
	}
	if dr.TsCode != "000001.SZ" {
		t.Errorf("TsCode: got %s, want 000001.SZ", dr.TsCode)
	}
	if dr.Vol != 1000.0 {
		t.Errorf("Vol: got %v, want 1000.0", dr.Vol)
	}
	if dr.PctChg != 2.5 {
		t.Errorf("PctChg: got %v, want 2.5", dr.PctChg)
	}
}

func TestBuildGenericRecord(t *testing.T) {
	data := map[string]interface{}{
		"source": "mock",
		"rank":   int64(1),
	}
	result := buildGenericRecord(data, "stock_hot_rank")
	if result == nil {
		t.Fatal("buildGenericRecord returned nil")
	}
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected map, got %T", result)
	}
	if m["source"] != "mock" {
		t.Errorf("source: got %v, want mock", m["source"])
	}
}

func TestBuildRecord(t *testing.T) {
	dailyData := map[string]interface{}{
		"instrument_id": "000001.SZ",
		"vol":           float64(100.0),
	}
	result := buildRecord(dailyData, "daily")
	if _, ok := result.(schema.DailyRecord); !ok {
		t.Errorf("buildRecord for daily should return DailyRecord, got %T", result)
	}

	unknownData := map[string]interface{}{
		"foo": "bar",
	}
	result = buildRecord(unknownData, "nonexistent_table")
	if _, ok := result.(map[string]interface{}); !ok {
		t.Errorf("buildRecord for unknown table should return map, got %T", result)
	}
}

func TestGetString(t *testing.T) {
	data := map[string]interface{}{
		"str":  "hello",
		"num":  int64(42),
	}
	if getString(data, "str") != "hello" {
		t.Errorf("getString(str): got %s, want hello", getString(data, "str"))
	}
	if getString(data, "num") != "42" {
		t.Errorf("getString(num): got %s, want 42", getString(data, "num"))
	}
	if getString(data, "missing") != "" {
		t.Errorf("getString(missing): got %s, want empty", getString(data, "missing"))
	}
}

func TestGetFloat(t *testing.T) {
	data := map[string]interface{}{
		"f": float64(3.14),
		"i": int64(42),
		"s": "2.5",
	}
	if getFloat(data, "f") != 3.14 {
		t.Errorf("getFloat(f): got %v, want 3.14", getFloat(data, "f"))
	}
	if getFloat(data, "i") != 42.0 {
		t.Errorf("getFloat(i): got %v, want 42.0", getFloat(data, "i"))
	}
	if getFloat(data, "s") != 2.5 {
		t.Errorf("getFloat(s): got %v, want 2.5", getFloat(data, "s"))
	}
}

func TestGetInt64(t *testing.T) {
	data := map[string]interface{}{
		"i": int64(100),
		"f": float64(42.5),
		"s": "123",
	}
	if getInt64(data, "i") != 100 {
		t.Errorf("getInt64(i): got %v, want 100", getInt64(data, "i"))
	}
	if getInt64(data, "f") != 42 {
		t.Errorf("getInt64(f): got %v, want 42", getInt64(data, "f"))
	}
	if getInt64(data, "s") != 123 {
		t.Errorf("getInt64(s): got %v, want 123", getInt64(data, "s"))
	}
}

func TestCollectorTaskCreation(t *testing.T) {
	tmpDir := "/tmp/test-axdata-" + t.Name()
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	cfg := config.DefaultConfig(tmpDir)
	store := storage.NewStore(cfg)
	logger, _ := zap.NewDevelopment()
	collector, err := NewCollector(cfg, store, logger)
	if err != nil {
		t.Fatalf("NewCollector failed: %v", err)
	}

	task, err := collector.AddTask("test-task", "tdx", "daily", "daily", "core", map[string]interface{}{"code": "000001.SZ"})
	if err != nil {
		t.Fatalf("AddTask failed: %v", err)
	}
	if task.Name != "test-task" {
		t.Errorf("Task name: got %s, want test-task", task.Name)
	}
	if task.Source != "tdx" {
		t.Errorf("Task source: got %s, want tdx", task.Source)
	}

	retrieved, ok := collector.GetTask(task.ID)
	if !ok {
		t.Fatalf("GetTask failed for ID %s", task.ID)
	}
	if retrieved.Name != "test-task" {
		t.Errorf("Retrieved task name: got %s, want test-task", retrieved.Name)
	}
}

func TestExecuteTask_UnknownSource(t *testing.T) {
	tmpDir := "/tmp/test-axdata-" + t.Name()
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	cfg := config.DefaultConfig(tmpDir)
	store := storage.NewStore(cfg)
	logger := zap.NewNop()
	collector, err := NewCollector(cfg, store, logger)
	if err != nil {
		t.Fatalf("NewCollector failed: %v", err)
	}

	task, err := collector.AddTask("test-unknown-source", "nonexistent_source_xyz", "daily", "daily", "core", nil)
	if err != nil {
		t.Fatalf("AddTask failed: %v", err)
	}

	ctx := context.Background()
	_, err = collector.executeTask(ctx, task)
	if err == nil {
		t.Fatal("Expected error for unknown source")
	}
	if !strings.Contains(err.Error(), "unknown source") {
		t.Errorf("Expected 'unknown source' in error, got: %v", err)
	}
}

func TestExecuteTask_AdapterError(t *testing.T) {
	tmpDir := "/tmp/test-axdata-" + t.Name()
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	cfg := config.DefaultConfig(tmpDir)
	store := storage.NewStore(cfg)
	logger := zap.NewNop()
	collector, err := NewCollector(cfg, store, logger)
	if err != nil {
		t.Fatalf("NewCollector failed: %v", err)
	}

	adapter := &errorAdapter{
		name: "error-source",
		err:  fmt.Errorf("connection refused"),
	}
	Register(adapter)

	task, err := collector.AddTask("test-adapter-error", "error-source", "daily", "daily", "core", nil)
	if err != nil {
		t.Fatalf("AddTask failed: %v", err)
	}

	ctx := context.Background()
	_, err = collector.executeTask(ctx, task)
	if err == nil {
		t.Fatal("Expected error for adapter failure")
	}
	if !strings.Contains(err.Error(), "source request") {
		t.Errorf("Expected 'source request' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("Expected wrapped 'connection refused' in error, got: %v", err)
	}
}

func TestExecuteTask_EmptyResults(t *testing.T) {
	tmpDir := "/tmp/test-axdata-" + t.Name()
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	cfg := config.DefaultConfig(tmpDir)
	store := storage.NewStore(cfg)
	logger := zap.NewNop()
	collector, err := NewCollector(cfg, store, logger)
	if err != nil {
		t.Fatalf("NewCollector failed: %v", err)
	}

	adapter := &emptyAdapter{name: "empty-source"}
	Register(adapter)

	task, err := collector.AddTask("test-empty", "empty-source", "daily", "daily", "core", nil)
	if err != nil {
		t.Fatalf("AddTask failed: %v", err)
	}

	ctx := context.Background()
	rows, err := collector.executeTask(ctx, task)
	if err != nil {
		t.Fatalf("executeTask with empty results should not error: %v", err)
	}
	if rows != 0 {
		t.Errorf("Expected 0 rows from empty adapter, got %d", rows)
	}
}

func TestExecuteTask_NoOutputTable(t *testing.T) {
	tmpDir := "/tmp/test-axdata-" + t.Name()
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	cfg := config.DefaultConfig(tmpDir)
	store := storage.NewStore(cfg)
	logger := zap.NewNop()
	collector, err := NewCollector(cfg, store, logger)
	if err != nil {
		t.Fatalf("NewCollector failed: %v", err)
	}

	adapter := &mockDataAdapter{name: "mock-data"}
	Register(adapter)

	// Use a non-existent interface name so GetOutputTable returns empty
	task, err := collector.AddTask("test-no-table", "mock-data", "nonexistent_interface", "", "core", nil)
	if err != nil {
		t.Fatalf("AddTask failed: %v", err)
	}

	ctx := context.Background()
	_, err = collector.executeTask(ctx, task)
	if err == nil {
		t.Fatal("Expected error for no output table")
	}
	if !strings.Contains(err.Error(), "no output table") {
		t.Errorf("Expected 'no output table' in error, got: %v", err)
	}
}

func TestExecuteTask_UnknownSchema(t *testing.T) {
	tmpDir := "/tmp/test-axdata-" + t.Name()
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	cfg := config.DefaultConfig(tmpDir)
	store := storage.NewStore(cfg)
	logger := zap.NewNop()
	collector, err := NewCollector(cfg, store, logger)
	if err != nil {
		t.Fatalf("NewCollector failed: %v", err)
	}

	adapter := &mockDataAdapter{name: "mock-data"}
	Register(adapter)

	// Task table points to a schema that doesn't exist
	task, err := collector.AddTask("test-unknown-schema", "mock-data", "daily", "nonexistent_table_xyz", "core", nil)
	if err != nil {
		t.Fatalf("AddTask failed: %v", err)
	}

	ctx := context.Background()
	_, err = collector.executeTask(ctx, task)
	if err == nil {
		t.Fatal("Expected error for unknown table schema")
	}
	if !strings.Contains(err.Error(), "unknown table schema") {
		t.Errorf("Expected 'unknown table schema' in error, got: %v", err)
	}
}

func TestExecuteTask_TaskParamOverrides(t *testing.T) {
	tmpDir := "/tmp/test-axdata-" + t.Name()
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	cfg := config.DefaultConfig(tmpDir)
	adapter := &mockDataAdapter{name: "mock-data"}
	Register(adapter)
	store := storage.NewStore(cfg)
	logger := zap.NewNop()
	collector, err := NewCollector(cfg, store, logger)
	if err != nil {
		t.Fatalf("NewCollector failed: %v", err)
	}

	// Override table via task params — custom table name in ProviderRegistry
	provider := source.ProviderRegistry["daily"]
	if provider == nil {
		t.Skip("daily provider not in registry, skipping")
	}
	// Add a fake interface for this test
	customIface := "custom_mock_daily"
	source.ProviderRegistry[customIface] = &source.ProviderInterface{
		Name:        customIface,
		SourceCode:  "mock-data",
		Table:       "daily",
		Layer:       "core",
		WriteMode:   "append",
	}
	defer func() {
		delete(source.ProviderRegistry, customIface)
	}()

	task, err := collector.AddTask("test-override", "mock-data", customIface, "", "core", nil)
	if err != nil {
		t.Fatalf("AddTask failed: %v", err)
	}
	task.Enabled = true

	ctx := context.Background()
	_, err = collector.RunTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("RunTask failed: %v", err)
	}

	if !store.Exists("core", "daily") {
		t.Fatal("daily table not found in store after task run")
	}
}

// errorAdapter always returns an error on Request.
type errorAdapter struct {
	name string
	err  error
}

func (a *errorAdapter) Name() string           { return a.name }
func (a *errorAdapter) Description() string    { return "error adapter" }
func (a *errorAdapter) Request(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	return nil, a.err
}

// emptyAdapter always returns nil results.
type emptyAdapter struct {
	name string
}

func (a *emptyAdapter) Name() string            { return a.name }
func (a *emptyAdapter) Description() string     { return "empty adapter" }
func (a *emptyAdapter) Request(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	return nil, nil
}

// mockDataAdapter returns simple mock data for testing.
type mockDataAdapter struct {
	name string
}

func (a *mockDataAdapter) Name() string             { return a.name }
func (a *mockDataAdapter) Description() string      { return "mock data adapter" }
func (a *mockDataAdapter) Request(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	return []map[string]interface{}{
		{
			"ts_code":  "000001.SZ",
			"vol":      float64(1000),
			"pct_chg":  float64(2.5),
			"open":     float64(10.0),
			"high":     float64(11.0),
			"low":      float64(9.5),
			"close":    float64(10.8),
		},
		{
			"ts_code":  "000002.SZ",
			"vol":      float64(2000),
			"pct_chg":  float64(-1.2),
			"open":     float64(30.0),
			"high":     float64(31.0),
			"low":      float64(29.5),
			"close":    float64(29.8),
		},
	}, nil
}
