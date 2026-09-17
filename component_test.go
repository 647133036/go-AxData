package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/electkismet/axdata-go/core/collector"
	"github.com/electkismet/axdata-go/core/config"
	"github.com/electkismet/axdata-go/core/query"
	"github.com/electkismet/axdata-go/core/source"
	"github.com/electkismet/axdata-go/core/storage"
	"go.uber.org/zap"
)

// scriptedAdapter serves deterministic rows per interface so the component test
// can exercise the real adapter -> collector -> storage -> query chain without
// a network call. It is a test double at the adapter boundary only: everything
// downstream is production code.
type scriptedAdapter struct {
	name    string
	failure bool
}

func (a *scriptedAdapter) Name() string        { return a.name }
func (a *scriptedAdapter) Description() string { return "scripted test adapter" }

func (a *scriptedAdapter) Request(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	if a.failure {
		return nil, errors.New("scripted failure")
	}
	iface, _ := params["interface"].(string)

	switch iface {
	case "daily_bars":
		return []map[string]interface{}{
			{"ts_code": "000001.SZ", "trade_date": "20260105", "open": 10.0, "high": 10.5, "low": 9.8, "close": 10.2, "vol": 1000, "amount": 10200},
			{"ts_code": "000001.SZ", "trade_date": "20260106", "open": 10.2, "high": 11.0, "low": 10.1, "close": 10.9, "vol": 1500, "amount": 16350},
		}, nil
	case "stock_basic":
		return []map[string]interface{}{
			{"instrument_id": "000001.SZ", "symbol": "000001", "exchange": "SZSE", "name": "平安银行", "market": "主板", "industry": "银行", "total_share": 194.06, "float_share": 194.06, "list_date": "19910403", "listing_status": "listed", "last_price": 10.9},
			{"instrument_id": "000002.SZ", "symbol": "000002", "exchange": "SZSE", "name": "万科A", "market": "主板", "industry": "房地产", "total_share": 116.30, "float_share": 116.30, "list_date": "19910129", "listing_status": "listed", "last_price": 6.4},
		}, nil
	case "stock_basic_update":
		// Same primary key as stock_basic with a changed price, used to prove
		// the upsert keeps the latest value rather than a second row.
		return []map[string]interface{}{
			{"instrument_id": "000001.SZ", "symbol": "000001", "exchange": "SZSE", "name": "平安银行", "market": "主板", "industry": "银行", "total_share": 194.06, "float_share": 194.06, "list_date": "19910403", "listing_status": "listed", "last_price": 11.75},
		}, nil
	default:
		return nil, fmt.Errorf("scripted adapter has no interface %q", iface)
	}
}

func newComponentFixture(t *testing.T) (*collector.Collector, *storage.Store, *query.Querier, string) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "test-axdata-component-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	cfg := config.DefaultConfig(tmpDir)
	store := storage.NewStore(cfg)
	collector, err := collector.NewCollector(cfg, store, zap.NewNop())
	if err != nil {
		t.Fatalf("NewCollector: %v", err)
	}

	querier, err := query.NewQuerier()
	if err != nil {
		t.Fatalf("NewQuerier: %v", err)
	}
	t.Cleanup(func() { querier.Close() })

	return collector, store, querier, tmpDir
}

func addEnabledTask(t *testing.T, c *collector.Collector, name, table, iface string) string {
	t.Helper()
	task, err := c.AddTask(name, "scripted", iface, table, "core", nil, collector.TaskSchedule{})
	if err != nil {
		t.Fatalf("AddTask %s: %v", name, err)
	}
	if err := c.UpdateTask(task.ID, map[string]interface{}{"enabled": true}); err != nil {
		t.Fatalf("enable %s: %v", name, err)
	}
	return task.ID
}

// TestComponent_AdapterThroughCollectorToQuery runs a real adapter through the
// collector and reads the result back through DuckDB, so a regression in field
// mapping, table resolution, write mode or column typing is caught at the seam
// between the modules instead of inside a single package.
func TestComponent_AdapterThroughCollectorToQuery(t *testing.T) {
	c, store, querier, tmpDir := newComponentFixture(t)
	source.Register(&scriptedAdapter{name: "scripted"})

	ctx := context.Background()

	// Append table: the same payload twice must accumulate, not dedupe.
	appendID := addEnabledTask(t, c, "append-bars", "daily", "daily_bars")
	upsertID := addEnabledTask(t, c, "basic-v1", "stock_basic_exchange", "stock_basic")

	runs, err := c.RunTasks(ctx, []string{appendID, upsertID})
	if err != nil {
		t.Fatalf("RunTasks: %v", err)
	}
	if len(runs) != 2 || runs[0] == nil || runs[1] == nil {
		t.Fatalf("RunTasks returned %d runs, want 2", len(runs))
	}

	if got := countQuery(t, querier, filepath.Join(tmpDir, "data/core/daily.parquet")); got != 2 {
		t.Fatalf("daily rows after first run = %d, want 2", got)
	}
	if n, err := store.Count("core", "daily"); err != nil || n != 2 {
		t.Fatalf("store.Count daily = %d, %v; want 2", n, err)
	}

	// Second run of the append table adds rows; the upsert table converges to
	// the latest value at the same row count.
	if _, err := c.RunTask(ctx, appendID); err != nil {
		t.Fatalf("second append run: %v", err)
	}
	updateID := addEnabledTask(t, c, "basic-v2", "stock_basic_exchange", "stock_basic_update")
	if _, err := c.RunTask(ctx, updateID); err != nil {
		t.Fatalf("upsert update run: %v", err)
	}

	if got := countQuery(t, querier, filepath.Join(tmpDir, "data/core/daily.parquet")); got != 4 {
		t.Fatalf("daily rows after two runs = %d, want 4 (append must accumulate)", got)
	}
	if n, err := store.Count("core", "daily"); err != nil || n != 4 {
		t.Fatalf("store.Count daily = %d, %v; want 4", n, err)
	}

	if got := countQuery(t, querier, filepath.Join(tmpDir, "data/core/stock_basic_exchange.parquet")); got != 2 {
		t.Fatalf("stock_basic rows = %d, want 2 (upsert must replace, not append)", got)
	}

	// SQL read-back proves the typed values survived the Parquet round trip.
	rows, err := querier.Execute(ctx,
		"SELECT instrument_id, last_price FROM read_parquet(?) WHERE instrument_id = ?",
		filepath.Join(tmpDir, "data/core/stock_basic_exchange.parquet"), "000001.SZ")
	if err != nil {
		t.Fatalf("query last_price: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("upsert query returned %d rows, want 1", len(rows))
	}
	if got := asFloat(rows[0][1]); got != 11.75 {
		t.Fatalf("last_price = %v, want 11.75: the second run must win", got)
	}
}

// TestComponent_RunFailureLeavesNoData confirms a failing adapter neither
// writes rows nor reports a successful run.
func TestComponent_RunFailureLeavesNoData(t *testing.T) {
	c, store, _, tmpDir := newComponentFixture(t)
	source.Register(&scriptedAdapter{name: "scripted", failure: true})

	id := addEnabledTask(t, c, "failing", "daily", "daily_bars")
	run, err := c.RunTask(context.Background(), id)
	if err == nil {
		t.Fatal("RunTask returned no error for a failing adapter")
	}
	if run == nil || run.Status != "failed" {
		t.Fatalf("run = %+v, want status failed", run)
	}

	if n, err := store.Count("core", "daily"); err != nil || n != 0 {
		t.Fatalf("daily rows after a failed run = %d, %v; want 0", n, err)
	}
	if store.Exists("core", "daily") {
		t.Fatal("daily.parquet must not exist after a failed run")
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "data/core/daily.parquet")); !os.IsNotExist(err) {
		t.Fatalf("daily.parquet exists after a failed run (err=%v)", err)
	}
}

// TestComponent_AppendAndUpsertConcurrentAcrossTables proves the per-table lock
// serializes only within a table: two tasks hitting different tables run in
// parallel without losing rows.
func TestComponent_AppendAndUpsertConcurrentAcrossTables(t *testing.T) {
	c, store, _, _ := newComponentFixture(t)
	source.Register(&scriptedAdapter{name: "scripted"})

	appendID := addEnabledTask(t, c, "c-append", "daily", "daily_bars")
	upsertID := addEnabledTask(t, c, "c-upsert", "stock_basic_exchange", "stock_basic")

	for i := 0; i < 6; i++ {
		runs, err := c.RunTasks(context.Background(), []string{appendID, upsertID})
		if err != nil {
			t.Fatalf("RunTasks iteration %d: %v", i, err)
		}
		for j, run := range runs {
			if run == nil || run.Status != "success" {
				t.Fatalf("iteration %d run %d = %+v, want success", i, j, run)
			}
		}
	}

	// 6 runs x 2 rows each, appended.
	if n, err := store.Count("core", "daily"); err != nil || n != 12 {
		t.Fatalf("daily rows = %d, %v; want 12", n, err)
	}
	// Upsert stays at the 2 distinct primary keys.
	if n, err := store.Count("core", "stock_basic_exchange"); err != nil || n != 2 {
		t.Fatalf("stock_basic rows = %d, %v; want 2", n, err)
	}
}

func countQuery(t *testing.T, q *query.Querier, path string) int {
	t.Helper()
	rows, err := q.Execute(context.Background(), "SELECT count(*) FROM read_parquet(?)", path)
	if err != nil {
		t.Fatalf("count query %s: %v", path, err)
	}
	if len(rows) != 1 {
		t.Fatalf("count query returned %d rows", len(rows))
	}
	n, ok := rows[0][0].(int64)
	if !ok {
		t.Fatalf("count query returned %T, want int64", rows[0][0])
	}
	return int(n)
}

func asFloat(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int64:
		return float64(n)
	case int32:
		return float64(n)
	default:
		return -1
	}
}
