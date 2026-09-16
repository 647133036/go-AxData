package cache

import (
	"context"
	"os"
	"testing"

	"github.com/electkismet/axdata-go/core/config"
	"github.com/electkismet/axdata-go/core/query"
	"github.com/electkismet/axdata-go/core/storage"
	"go.uber.org/zap"
)

func newTestGetter(t *testing.T) (*Getter, *query.Querier) {
	t.Helper()
	tmpDir := "/tmp/test-axdata-cache-" + t.Name()
	os.RemoveAll(tmpDir)
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	q, err := query.NewQuerier()
	if err != nil {
		t.Fatalf("NewQuerier: %v", err)
	}
	t.Cleanup(func() { _ = q.Close() })

	cfg := config.DefaultConfig(tmpDir)
	store := storage.NewStore(cfg)
	return New(cfg, store, q, zap.NewNop()), q
}

func finIncomeRow(code, reportDate string, revenue float64) map[string]interface{} {
	return map[string]interface{}{
		"ts_code":      code,
		"symbol":       code[:6],
		"exchange":     "TEST",
		"name":         "名称",
		"report_date":  reportDate,
		"revenue":      revenue,
		"net_profit":   revenue / 2,
		"gross_margin": 0.5,
	}
}

// TestFetchTableIsolatesKeys guards against cross-security cache bleed.
// Snapshot tables are overwritten wholesale, so a key-blind fetch lets the last
// security written shadow every other one already in the cache.
func TestFetchTableIsolatesKeys(t *testing.T) {
	g, _ := newTestGetter(t)
	ctx := context.Background()
	whereA := map[string]string{"ts_code": "600519.SH"}
	whereB := map[string]string{"ts_code": "000001.SZ"}

	_, err := g.FetchTable(ctx, "fin_income", whereA, 1, func(context.Context) ([]map[string]interface{}, error) {
		return []map[string]interface{}{finIncomeRow("600519.SH", "20260630", 9.2e10)}, nil
	})
	if err != nil {
		t.Fatalf("fetch A: %v", err)
	}

	rowsB, err := g.FetchTable(ctx, "fin_income", whereB, 1, func(context.Context) ([]map[string]interface{}, error) {
		return []map[string]interface{}{finIncomeRow("000001.SZ", "20260630", 7.06e10)}, nil
	})
	if err != nil {
		t.Fatalf("fetch B: %v", err)
	}

	if len(rowsB) != 1 || rowsB[0].Str("ts_code") != "000001.SZ" {
		t.Fatalf("fetch B returned %d rows, got %v", len(rowsB), rowsB)
	}

	// The earlier security must still be intact after B was written.
	rowsA2, err := g.FetchTable(ctx, "fin_income", whereA, 1, func(context.Context) ([]map[string]interface{}, error) {
		t.Fatal("fetch A should have been served from cache, not the source")
		return nil, nil
	})
	if err != nil {
		t.Fatalf("refetch A: %v", err)
	}
	if len(rowsA2) != 1 || rowsA2[0].Str("ts_code") != "600519.SH" {
		t.Fatalf("fetch A after B returned %d rows, got %v", len(rowsA2), rowsA2)
	}
	if got := rowsA2[0].F64("revenue"); got != 9.2e10 {
		t.Fatalf("A revenue = %v, want %v", got, 9.2e10)
	}
}

func TestFilterRows(t *testing.T) {
	rows := []Row{
		{Values: map[string]interface{}{"ts_code": "A", "v": float64(1)}},
		{Values: map[string]interface{}{"ts_code": "B", "v": float64(2)}},
	}
	kept := filterRows(rows, map[string]string{"ts_code": "B"})
	if len(kept) != 1 || kept[0].Str("ts_code") != "B" {
		t.Fatalf("filterRows = %v", kept)
	}
	if got := filterRows(rows, nil); len(got) != 2 {
		t.Fatalf("filterRows with empty where must keep all, got %d", len(got))
	}
}

func TestMergeRowsKeepsOtherKeys(t *testing.T) {
	g, _ := newTestGetter(t)
	ctx := context.Background()
	where := map[string]string{"ts_code": "000001.SZ"}

	if err := g.Store.Write("core", "fin_income", []interface{}{
		finIncomeRow("600519.SH", "20260630", 9.2e10),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	merged := g.mergeRows(ctx, "fin_income", where, []map[string]interface{}{
		finIncomeRow("000001.SZ", "20260630", 7.06e10),
	})
	if len(merged) != 2 {
		t.Fatalf("mergeRows kept %d rows, want 2", len(merged))
	}

	// Replacing the same key must not accumulate duplicates.
	merged = g.mergeRows(ctx, "fin_income", where, []map[string]interface{}{
		finIncomeRow("000001.SZ", "20260630", 8.0e10),
	})
	if len(merged) != 2 {
		t.Fatalf("mergeRows must replace matching rows, got %d", len(merged))
	}
}
