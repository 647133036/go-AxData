package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	parquet "github.com/parquet-go/parquet-go"

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

// TestSnapshotRoundTrip ensures snapshot tables written from generic map records
// survive a read back with every column intact. Without a typed conversion case
// per table, parquet-go collapses a Go map into a single blob column and all
// data is lost on round-trip.
func TestSnapshotRoundTrip(t *testing.T) {
	cases := []struct {
		table    string
		records  []interface{}
		wantCols []string
	}{
		{
			table: "fin_income",
			records: []interface{}{
				map[string]interface{}{
					"ts_code": "000001.SZ", "report_date": "2026-06-30",
					"revenue": 7.0617e10, "net_profit": 2.5696e10,
					"gross_margin": 0.4366,
				},
			},
			wantCols: []string{"ts_code", "report_date", "revenue", "net_profit", "gross_margin"},
		},
		{
			table: "earnings_forecast",
			records: []interface{}{
				map[string]interface{}{
					"ts_code": "000001.SZ", "forecast_item": "归母净利润",
					"forecast_type": "预增", "amount_lower": 3.0e10,
					"change_lower": 60.0, "reason": "主营增长",
				},
			},
			wantCols: []string{"ts_code", "forecast_item", "forecast_type", "amount_lower", "change_lower", "reason"},
		},
		{
			table: "market_mainline_cls",
			records: []interface{}{
				map[string]interface{}{
					"block_key": "AI", "title": "人工智能主线",
					"reason": "算力需求爆发",
				},
			},
			wantCols: []string{"block_key", "title", "reason"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.table, func(t *testing.T) {
			tmpDir := "/tmp/test-axdata-store-snap-" + t.Name()
			t.Cleanup(func() { os.RemoveAll(tmpDir) })

			store := NewStore(config.DefaultConfig(tmpDir))
			if err := store.Write("core", tc.table, tc.records); err != nil {
				t.Fatalf("Write failed: %v", err)
			}

			path := filepath.Join(tmpDir, "data/core", tc.table+".parquet")
			f, err := os.Open(path)
			if err != nil {
				t.Fatalf("Open failed: %v", err)
			}
			defer f.Close()

			r := parquet.NewReader(f)
			defer r.Close()
			if r.NumRows() != 1 {
				t.Fatalf("NumRows = %d, want 1", r.NumRows())
			}

			present := map[string]bool{}
			for _, col := range r.Schema().Columns() {
				present[strings.Join(col, ".")] = true
			}
			for _, want := range tc.wantCols {
				if !present[want] {
					t.Errorf("column %q missing from parquet schema; got %v", want, colNames(r.Schema()))
				}
			}
		})
	}
}

func colNames(ps *parquet.Schema) []string {
	var names []string
	for _, col := range ps.Columns() {
		names = append(names, strings.Join(col, "."))
	}
	return names
}
