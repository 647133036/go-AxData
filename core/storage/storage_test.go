package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/electkismet/axdata-go/core/config"
	"github.com/electkismet/axdata-go/core/schema"
)

func TestStorageBasic(t *testing.T) {
	tmpDir := "/tmp/test-axdata-store-" + t.Name()
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	cfg := config.DefaultConfig(tmpDir)
	store := NewStore(cfg)

	record := schema.DailyRecord{
		TsCode:    "000001.SZ",
		TradeDate: "2024-01-15",
		Open:      10.0,
		High:      11.0,
		Low:       9.0,
		Close:     10.5,
	}

	err := store.Write("core", "daily", []interface{}{record})
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	parquetPath := filepath.Join(tmpDir, "data/core/daily.parquet")
	if _, err := os.Stat(parquetPath); os.IsNotExist(err) {
		t.Fatalf("Expected parquet file at %s not found", parquetPath)
	}
	t.Logf("Parquet written to %s", parquetPath)
}

func TestStorageAppend(t *testing.T) {
	tmpDir := "/tmp/test-axdata-store-2-" + t.Name()
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	cfg := config.DefaultConfig(tmpDir)
	store := NewStore(cfg)

	records := []interface{}{
		schema.DailyRecord{TsCode: "000001.SZ", TradeDate: "2024-01-10"},
		schema.DailyRecord{TsCode: "000001.SZ", TradeDate: "2024-01-11"},
	}

	err := store.Write("core", "daily", records)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	more := []interface{}{
		schema.DailyRecord{TsCode: "000001.SZ", TradeDate: "2024-01-12"},
	}
	err = store.Append("core", "daily", more)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}
	t.Log("Append succeeded")
}
