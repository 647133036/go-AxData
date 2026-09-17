package storage

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
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

// TestAppendPreservesExistingRows reproduces the append regression: the writer
// reopened the file without O_TRUNC or O_APPEND and rewrote from offset zero,
// leaving overlapping parquet footers. The file still existed and the count
// metadata was right, but neither parquet-go nor DuckDB could open it. The
// original TestStorageAppend only checked that Append returned nil, which this
// bug satisfied.
func TestAppendPreservesExistingRows(t *testing.T) {
	tmpDir := "/tmp/test-axdata-store-append-" + t.Name()
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	store := NewStore(config.DefaultConfig(tmpDir))

	batches := [][]interface{}{
		{
			schema.DailyRecord{TsCode: "000001.SZ", TradeDate: "2024-01-10", Close: 10.5},
			schema.DailyRecord{TsCode: "000001.SZ", TradeDate: "2024-01-11", Close: 11.0},
		},
		{
			schema.DailyRecord{TsCode: "000001.SZ", TradeDate: "2024-01-12", Close: 11.5},
		},
		{
			schema.DailyRecord{TsCode: "000001.SZ", TradeDate: "2024-01-15", Close: 12.0},
			schema.DailyRecord{TsCode: "000001.SZ", TradeDate: "2024-01-16", Close: 12.5},
			schema.DailyRecord{TsCode: "000001.SZ", TradeDate: "2024-01-17", Close: 13.0},
		},
	}

	for i, batch := range batches {
		err := store.Append("core", "daily", batch)
		if err != nil {
			t.Fatalf("append %d failed: %v", i, err)
		}
	}

	path := filepath.Join(tmpDir, "data/core/daily.parquet")
	wantRows := 6
	got, numRows, err := readDailyRows(path)
	if err != nil {
		t.Fatalf("parquet file unreadable after %d appends: %v", len(batches), err)
	}
	if numRows != wantRows {
		t.Fatalf("NumRows = %d, want %d", numRows, wantRows)
	}
	if len(got) != wantRows {
		t.Fatalf("read back %d rows, want %d", len(got), wantRows)
	}

	want := map[string]float64{
		"2024-01-10": 10.5, "2024-01-11": 11.0, "2024-01-12": 11.5,
		"2024-01-15": 12.0, "2024-01-16": 12.5, "2024-01-17": 13.0,
	}
	seen := make(map[string]float64, len(got))
	for _, r := range got {
		if r.TsCode == "" {
			t.Errorf("row %v has empty ts_code", r)
		}
		seen[r.TradeDate] = r.Close
	}
	for date, closePrice := range want {
		if seen[date] != closePrice {
			t.Errorf("close[%s] = %v, want %v", date, seen[date], closePrice)
		}
	}

	count, err := store.Count("core", "daily")
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != wantRows {
		t.Errorf("Count = %d, want %d", count, wantRows)
	}
}

// TestWriteTypedStructSurvivesWrite covers the typed input path: records that
// arrive as schema structs rather than maps used to be flattened through a
// map[string]interface{} assertion that only accepted maps, so every field read
// back as "" or 0. Map input was unaffected, which is why this never surfaced.
func TestWriteTypedStructSurvivesWrite(t *testing.T) {
	tmpDir := "/tmp/test-axdata-store-typed-" + t.Name()
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	store := NewStore(config.DefaultConfig(tmpDir))

	rec := schema.DailyRecord{
		TsCode: "600519.SH", TradeDate: "2024-06-28",
		Open: 1258.0, High: 1269.0, Low: 1249.0, Close: 1258.0,
		Vol: 31000, Amount: 3.9e10,
	}
	if err := store.Write("core", "daily", []interface{}{rec}); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	path := filepath.Join(tmpDir, "data/core/daily.parquet")
	got, _, err := readDailyRows(path)
	if err != nil {
		t.Fatalf("read back failed: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("rows = %d, want 1", len(got))
	}
	r := got[0]
	if r.TsCode != rec.TsCode {
		t.Errorf("TsCode = %q, want %q", r.TsCode, rec.TsCode)
	}
	if r.TradeDate != rec.TradeDate {
		t.Errorf("TradeDate = %q, want %q", r.TradeDate, rec.TradeDate)
	}
	if r.Close != rec.Close {
		t.Errorf("Close = %v, want %v", r.Close, rec.Close)
	}
	if r.Amount != rec.Amount {
		t.Errorf("Amount = %v, want %v", r.Amount, rec.Amount)
	}
}

// TestUpsertByKeyReplacesInsteadOfDuplicates verifies upsert semantics on a
// table whose registry declares upsert_by_key: a second batch carrying the same
// primary key must overwrite the earlier row instead of duplicating it. stock
// basic is the only typed table with this mode, keyed by instrument_id.
func TestUpsertByKeyReplacesInsteadOfDuplicates(t *testing.T) {
	tmpDir := "/tmp/test-axdata-store-upsert-" + t.Name()
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	store := NewStore(config.DefaultConfig(tmpDir))

	first := []interface{}{
		schema.StockBasicRecord{InstrumentID: "600519.SH", Name: "Kweichow Moutai", Industry: "Liquor"},
		schema.StockBasicRecord{InstrumentID: "000858.SZ", Name: "Wuliangye", Industry: "Liquor"},
		schema.StockBasicRecord{InstrumentID: "000001.SZ", Name: "Ping An Bank", Industry: "Banking"},
	}
	if err := store.Append("core", "stock_basic_exchange", first); err != nil {
		t.Fatalf("first append failed: %v", err)
	}

	updated := []interface{}{
		schema.StockBasicRecord{InstrumentID: "600519.SH", Name: "Kweichow Moutai", Industry: "Baijiu"},
		schema.StockBasicRecord{InstrumentID: "300750.SZ", Name: "CATL", Industry: "Batteries"},
	}
	if err := store.Append("core", "stock_basic_exchange", updated); err != nil {
		t.Fatalf("second append failed: %v", err)
	}

	path := filepath.Join(tmpDir, "data/core/stock_basic_exchange.parquet")
	got, err := readStockBasicRows(path)
	if err != nil {
		t.Fatalf("read back failed: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("rows = %d, want 4 (3 initial plus 1 new key)", len(got))
	}

	byID := make(map[string]schema.StockBasicRecord, len(got))
	for _, r := range got {
		byID[r.InstrumentID] = r
	}
	if got := byID["600519.SH"].Industry; got != "Baijiu" {
		t.Errorf("upserted industry = %q, want %q", got, "Baijiu")
	}
	if got := byID["000858.SZ"].Name; got != "Wuliangye" {
		t.Errorf("untouched name = %q, want %q", got, "Wuliangye")
	}
	if _, ok := byID["300750.SZ"]; !ok {
		t.Error("new key 300750.SZ missing after upsert")
	}

	count, err := store.Count("core", "stock_basic_exchange")
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 4 {
		t.Errorf("Count = %d, want 4", count)
	}
}

// readStockBasicRows decodes every row of a stock_basic_exchange file.
func readStockBasicRows(path string) ([]schema.StockBasicRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := parquet.NewReader(f)
	defer r.Close()

	n := int(r.NumRows())
	rows := make([]schema.StockBasicRecord, 0, n)
	for {
		rec := schema.StockBasicRecord{}
		if err := r.Read(&rec); err != nil {
			if errors.Is(err, io.EOF) {
				return rows, nil
			}
			return nil, err
		}
		rows = append(rows, rec)
	}
}

// readDailyRows reads every row of a daily parquet file back through parquet-go,
// returning both the decoded records and the file's own row count.
func readDailyRows(path string) ([]schema.DailyRecord, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	r := parquet.NewReader(f)
	defer r.Close()

	n := int(r.NumRows())
	rows := make([]schema.DailyRecord, 0, n)
	for {
		rec := schema.DailyRecord{}
		if err := r.Read(&rec); err != nil {
			if errors.Is(err, io.EOF) {
				return rows, n, nil
			}
			return nil, n, err
		}
		rows = append(rows, rec)
	}
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

// TestConcurrentAppendToSameTable loses rows when it fails. Append is a
// read-merge-atomic-rename with no locking, so concurrent appends each read the
// same pre-merge rows; whichever rename lands last wins and the other writers'
// rows vanish. Concurrent collection is the normal case here, since many tasks
// write to one table (daily) for different symbols.
func TestConcurrentAppendToSameTable(t *testing.T) {
	tmpDir := "/tmp/test-axdata-store-concurrent-" + t.Name()
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	store := NewStore(config.DefaultConfig(tmpDir))

	const writers = 8
	const perWriter = 12
	var all []schema.DailyRecord
	for w := 0; w < writers; w++ {
		for i := 0; i < perWriter; i++ {
			all = append(all, schema.DailyRecord{
				TsCode:    fmt.Sprintf("%06d.SZ", w*100+i),
				TradeDate: fmt.Sprintf("2024-02-%02d", i+1),
				Close:     float64(w*100 + i),
			})
		}
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			<-start
			batch := make([]interface{}, perWriter)
			for i := 0; i < perWriter; i++ {
				batch[i] = all[w*perWriter+i]
			}
			if err := store.Append("core", "daily", batch); err != nil {
				t.Errorf("writer %d: %v", w, err)
			}
		}(w)
	}
	close(start)
	wg.Wait()

	path := filepath.Join(tmpDir, "data/core/daily.parquet")
	got, numRows, err := readDailyRows(path)
	if err != nil {
		t.Fatalf("parquet unreadable after concurrent appends: %v", err)
	}
	wantRows := writers * perWriter
	if numRows != wantRows {
		t.Fatalf("NumRows = %d, want %d (concurrent appends lost rows)", numRows, wantRows)
	}
	if len(got) != wantRows {
		t.Fatalf("read back %d rows, want %d", len(got), wantRows)
	}

	seen := make(map[string]bool, len(got))
	for _, r := range got {
		seen[fmt.Sprintf("%s %s", r.TsCode, r.TradeDate)] = true
	}
	missing := 0
	for _, r := range all {
		if !seen[fmt.Sprintf("%s %s", r.TsCode, r.TradeDate)] {
			missing++
		}
	}
	if missing > 0 {
		t.Errorf("%d of %d rows missing after concurrent append", missing, wantRows)
	}
}

// TestTypedRecordCoversSchemaColumns catches schema/struct drift. TableRegistry
// is the contract a source adapter and a SQL query both rely on, while
// convertRecords decides what actually reaches the file. When the two diverge a
// column is silently dropped at the storage boundary and no single package's
// tests see it: the schema test sees the declared column, the adapter test sees
// the emitted field, and only a round trip reveals the loss.
func TestTypedRecordCoversSchemaColumns(t *testing.T) {
	for table, schemaDef := range schema.TableRegistry {
		if newTypedRecord(table) == nil {
			continue
		}

		typed := reflect.TypeOf(newTypedRecord(table)).Elem()
		declared := make(map[string]bool, typed.NumField())
		for i := 0; i < typed.NumField(); i++ {
			field := typed.Field(i)
			tag, _ := field.Tag.Lookup("parquet")
			if tag == "" {
				t.Errorf("%s: field %s has no parquet tag, so it is never read back", table, field.Name)
				continue
			}
			declared[tag] = true
		}

		for _, col := range schemaDef.Columns {
			if !declared[col.Name] {
				t.Errorf("%s declares column %q but %s has no matching parquet field: the value is dropped at the storage boundary",
					table, col.Name, typed.Name())
			}
		}
	}
}
