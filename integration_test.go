package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/electkismet/axdata-go/core/collector"
	"github.com/electkismet/axdata-go/core/config"
	"github.com/electkismet/axdata-go/core/query"
	"github.com/electkismet/axdata-go/core/schema"
	"github.com/electkismet/axdata-go/core/source"
	"github.com/electkismet/axdata-go/core/storage"
	cls "github.com/electkismet/axdata-source-cls"
	cninfo "github.com/electkismet/axdata-source-cninfo"
	eastmoney "github.com/electkismet/axdata-source-eastmoney"
	kph "github.com/electkismet/axdata-source-kph"
	mock "github.com/electkismet/axdata-source-mock"
	sina "github.com/electkismet/axdata-source-sina"
	tdx "github.com/electkismet/axdata-source-tdx"
	tencent "github.com/electkismet/axdata-source-tencent"
	ths "github.com/electkismet/axdata-source-ths"
	wencai "github.com/electkismet/axdata-source-wencai"
	"go.uber.org/zap"
)

func TestIntegration_AdapterToStorage(t *testing.T) {
	tmpDir := "/tmp/test-axdata-integration-" + t.Name()
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	cfg := config.DefaultConfig(tmpDir)
	store := storage.NewStore(cfg)
	logger := zap.NewNop()
	collectorInst, err := collector.NewCollector(cfg, store, logger)
	if err != nil {
		t.Fatalf("NewCollector failed: %v", err)
	}

	task, err := collectorInst.AddTask(
		"tdx-daily-integration",
		"tdx",
		"stock_kline_daily_tdx",
		"daily",
		"core",
		map[string]interface{}{
			"codes": "000001.SZ,000002.SZ",
		},
	)
	if err != nil {
		t.Fatalf("AddTask failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = collectorInst.RunTask(ctx, task.ID)
	if err != nil {
		t.Logf("RunTask returned expected error (no TDX server): %v", err)
	}

	retrieved, ok := collectorInst.GetTask(task.ID)
	if !ok {
		t.Fatalf("GetTask failed for ID %s", task.ID)
	}
	if retrieved.ID != task.ID {
		t.Errorf("Retrieved task ID: got %s, want %s", retrieved.ID, task.ID)
	}
}

func TestIntegration_MockAdapterFullPipeline(t *testing.T) {
	source.Register(mock.NewMockAdapter())

	tmpDir := "/tmp/test-axdata-integration-fullpipe-" + t.Name()
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	cfg := config.DefaultConfig(tmpDir)
	store := storage.NewStore(cfg)
	logger := zap.NewNop()

	collectorInst, err := collector.NewCollector(cfg, store, logger)
	if err != nil {
		t.Fatalf("NewCollector failed: %v", err)
	}

	task, err := collectorInst.AddTask(
		"mock-daily-pipeline",
		"mock",
		"daily",
		"daily",
		"core",
		map[string]interface{}{
			"codes": "000001.SZ,000002.SZ",
		},
	)
	if err != nil {
		t.Fatalf("AddTask failed: %v", err)
	}

	err = collectorInst.UpdateTask(task.ID, map[string]interface{}{
		"enabled": true,
	})
	if err != nil {
		t.Fatalf("UpdateTask failed: %v", err)
	}

	ctx := context.Background()
	run, err := collectorInst.RunTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("RunTask failed: %v", err)
	}

	if run.Rows != 0 {
		t.Logf("Inserted %d rows", run.Rows)
	}

	// Verify parquet file was created
	if !store.Exists("core", "daily") {
		t.Fatal("daily table not found in store")
	}

	// Query via Querier
	querier, err := query.NewQuerier()
	if err != nil {
		t.Fatalf("NewQuerier failed: %v", err)
	}
	defer querier.Close()

	schemaDef := schema.GetSchema("daily")
	if schemaDef == nil {
		t.Fatal("daily schema not found")
	}

	// Create table and attach parquet
	dbTypes := map[string]string{
		"float64": "DOUBLE",
		"int64":   "BIGINT",
		"int32":   "INTEGER",
		"string":  "VARCHAR",
		"bool":    "BOOLEAN",
	}
	sqlParts := []string{"CREATE TABLE daily ("}
	for i, col := range schemaDef.Columns {
		dbType := dbTypes[col.Type]
		if dbType == "" {
			dbType = "VARCHAR"
		}
		sqlParts = append(sqlParts, col.Name+" "+dbType)
		if i < len(schemaDef.Columns)-1 {
			sqlParts = append(sqlParts, ",")
		}
	}
	sqlParts = append(sqlParts, ")")
	_, err = querier.Execute(ctx, strings.Join(sqlParts, " "))
	if err != nil {
		t.Fatalf("CREATE TABLE failed: %v", err)
	}

	parquetPath := filepath.Join(tmpDir, "data", "core", "daily.parquet")

	// Create table from parquet
	parquetSQL := fmt.Sprintf("CREATE TABLE daily_from_parquet AS SELECT * FROM read_parquet('%s')", parquetPath)
	_, err = querier.Execute(ctx, parquetSQL)
	if err != nil {
		t.Fatalf("CREATE TABLE from parquet failed: %v", err)
	}

	// Count rows
	countRows, err := querier.Execute(ctx, "SELECT COUNT(*) as cnt FROM daily_from_parquet")
	if err != nil {
		t.Fatalf("COUNT query failed: %v", err)
	}
	if len(countRows) == 0 || len(countRows[0]) == 0 {
		t.Fatal("COUNT returned no rows")
	}
	cnt := countRows[0][0]
	var cntInt int64
	switch v := cnt.(type) {
	case int64:
		cntInt = v
	case int:
		cntInt = int64(v)
	case float64:
		cntInt = int64(v)
	default:
		t.Fatalf("Unexpected COUNT type: %T", cnt)
	}
	if cntInt < 1 {
		t.Fatalf("Expected at least 1 row in parquet, got %d", cntInt)
	}
	t.Logf("Queried %d rows from parquet daily table", cntInt)
}

func TestIntegration_ProviderRegistryToSchema(t *testing.T) {
	for name, pi := range source.ProviderRegistry {
		if pi.Table == "" {
			t.Errorf("ProviderRegistry %s has no table", name)
		}
		schemaDef := schema.GetSchema(pi.Table)
		if schemaDef == nil {
			t.Errorf("ProviderRegistry %s targets non-existent table: %s", name, pi.Table)
		}
	}
}

func TestIntegration_Config(t *testing.T) {
	tmpDir := "/tmp/test-axdata-integration3-" + t.Name()
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	cfg := config.DefaultConfig(tmpDir)

	if cfg.DataRoot == "" {
		t.Fatal("DataRoot is empty")
	}
	if cfg.LogLevel == "" {
		t.Fatal("LogLevel is empty")
	}
}

func TestIntegration_QueryWithSchema(t *testing.T) {
	tmpDir := "/tmp/test-axdata-integration4-" + t.Name()
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	querier, err := query.NewQuerier()
	if err != nil {
		t.Fatalf("NewQuerier failed: %v", err)
	}
	defer querier.Close()

	// Create a daily table from schema
	schemaDef := schema.GetSchema("daily")
	if schemaDef == nil {
		t.Fatal("daily schema not found")
	}

	// Build CREATE TABLE SQL from schema (map Go types to DuckDB types)
	sqlParts := []string{"CREATE TABLE daily ("}
	for i, col := range schemaDef.Columns {
		dbType := col.Type
		switch dbType {
		case "float64":
			dbType = "DOUBLE"
		case "int64":
			dbType = "BIGINT"
		case "int32":
			dbType = "INTEGER"
		case "string":
			dbType = "VARCHAR"
		case "bool":
			dbType = "BOOLEAN"
		}
		sqlParts = append(sqlParts, col.Name+" "+dbType)
		if i < len(schemaDef.Columns)-1 {
			sqlParts = append(sqlParts, ",")
		}
	}

	sqlParts = append(sqlParts, ")")
	createSQL := strings.Join(sqlParts, " ")

	_, err = querier.Execute(context.Background(), createSQL)
	if err != nil {
		t.Fatalf("Execute CREATE TABLE failed: %v", err)
	}

	tables, err := querier.ListTables(context.Background())
	if err != nil {
		t.Fatalf("ListTables failed: %v", err)
	}
	if len(tables) < 1 {
		t.Fatal("Expected at least 1 table after creating 'daily'")
	}
	t.Logf("Found %d tables after creating daily", len(tables))
}

func TestIntegration_AllSourcesCompile(t *testing.T) {
	_ = tdx.NewDefaultTDXAdapter()
	_ = cls.NewCLSAdapter()
	_ = eastmoney.NewEastMoneyAdapter()
	_ = cninfo.NewCNINFOAdapter()
	_ = tencent.NewTencentAdapter()
	_ = sina.NewSinaAdapter()
	_ = kph.NewKPHAdapter()
	_ = ths.NewTHSAdapter()
	_ = wencai.NewWencaiAdapter()
	_ = mock.NewMockAdapter()
	_ = zap.NewNop()

	t.Log("All source adapters instantiated successfully")
}

func TestIntegration_ProviderRegistryWriteModeConsistency(t *testing.T) {
	validModes := map[string]bool{
		"append":              true,
		"snapshot":            true,
		"overwrite_partition": true,
		"replace_range":       true,
		"upsert_by_key":       true,
	}

	dualModeExceptions := map[string]bool{
		"stock_hot_rank":        true,
		"stock_basic_exchange":  true,
		"sector_emotion_detail": true,
		"stock_limit_ladder":    true,
		"stock_suspension":      true,
	}

	for name, pi := range source.ProviderRegistry {
		if pi.WriteMode == "" {
			t.Errorf("ProviderRegistry %s has empty WriteMode", name)
			continue
		}
		if !validModes[pi.WriteMode] {
			t.Errorf("ProviderRegistry %s has invalid WriteMode: %s", name, pi.WriteMode)
			continue
		}

		schemaDef := schema.GetSchema(pi.Table)
		if schemaDef == nil {
			continue
		}
		if dualModeExceptions[pi.Table] {
			continue
		}
		if schemaDef.WriteMode != pi.WriteMode && schemaDef.WriteMode != "" {
			t.Errorf("ProviderRegistry %s WriteMode (%s) != Schema %s WriteMode (%s)",
				name, pi.WriteMode, pi.Table, schemaDef.WriteMode)
		}
	}

	modeCounts := map[string]int{}
	for _, pi := range source.ProviderRegistry {
		modeCounts[pi.WriteMode]++
	}
	t.Logf("ProviderRegistry WriteMode distribution: %v", modeCounts)
}
