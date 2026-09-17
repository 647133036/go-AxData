package storage

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/electkismet/axdata-go/core/config"
	"github.com/electkismet/axdata-go/core/schema"
	parquet "github.com/parquet-go/parquet-go"
)

// Store provides Parquet-based data storage.
type Store struct {
	config *config.Config

	// locks serializes writes to a given layer/table. Both append and snapshot
	// overwrite rewrite the whole file, so two concurrent writers would each
	// read the pre-merge rows and the later rename would discard the earlier
	// writer's data. The locks map holds a stable mutex per table key.
	locks   map[string]*sync.Mutex
	locksMu sync.Mutex
}

// NewStore creates a new Store instance.
func NewStore(cfg *config.Config) *Store {
	return &Store{config: cfg, locks: make(map[string]*sync.Mutex)}
}

// lockTable returns the mutex that serializes writes to one layer/table pair.
// Keys are never released: the number of tables is bounded by the schema
// registry, so the map cannot grow without limit.
//
// Locks are per-Store, so two separate Store instances sharing a data root
// still race. Serializing a single data root across processes needs an
// external lock.
func (s *Store) lockTable(layer, table string) *sync.Mutex {
	s.locksMu.Lock()
	defer s.locksMu.Unlock()
	key := layer + "/" + table
	m, ok := s.locks[key]
	if !ok {
		m = &sync.Mutex{}
		s.locks[key] = m
	}
	return m
}

// Record represents a generic record as a map of column values.
type Record struct {
	Columns map[string]string
}

// Write writes records to a Parquet file in the specified layer.
func (s *Store) Write(layer, table string, records []interface{}) error {
	dir := s.config.DataDir(layer)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	if len(records) == 0 {
		return nil
	}

	schemaDef := schema.TableRegistry[table]
	if schemaDef == nil {
		return fmt.Errorf("unknown table: %s", table)
	}

	// Convert to typed structs based on table type
	typedRecords := convertRecords(records, table)

	if len(typedRecords) == 0 {
		return nil
	}

	// For append-mode tables, use Append (preserves existing data).
	// For overwrite/snapshot tables, use Write (creates new file).
	if schemaDef.WriteMode == "append" || schemaDef.WriteMode == "upsert_by_key" {
		// Append takes the table lock; taking it here too would deadlock, since
		// sync.Mutex is not reentrant.
		return s.Append(layer, table, typedRecords)
	}

	// Snapshot overwrites still rewrite the whole file, so two concurrent
	// writers to the same table would race on the rename.
	mu := s.lockTable(layer, table)
	mu.Lock()
	defer mu.Unlock()

	// Overwrite mode: write as new file
	path := filepath.Join(dir, table+".parquet")
	if err := writeParquetAtomic(dir, path, table, typedRecords); err != nil {
		return err
	}

	return writeCount(path, len(typedRecords))
}

// isTyped reports whether the converted records are typed structs rather than
// the generic map fallback.
func isTyped(records []interface{}) bool {
	if len(records) == 0 {
		return false
	}
	_, ok := records[0].(*Record)
	return !ok
}

// writeFileGeneric writes records whose table has no typed struct. It builds a
// schema from the union of observed column names so that columns survive a
// round-trip instead of collapsing into a single map blob.
func writeGeneric(f *os.File, table string, records []interface{}) error {
	if len(records) == 0 {
		return nil
	}

	names := map[string]bool{}
	for _, rec := range records {
		for name := range toMap(rec) {
			names[name] = true
		}
	}
	if len(names) == 0 {
		return nil
	}

	columns := make([]string, 0, len(names))
	for name := range names {
		columns = append(columns, name)
	}
	sort.Strings(columns)

	group := make(parquet.Group, len(columns))
	for _, name := range columns {
		group[name] = parquet.Optional(parquet.String())
	}
	ps := parquet.NewSchema(table, group)

	rows := make([]parquet.Row, 0, len(records))
	for _, rec := range records {
		m := toMap(rec)
		row := make([]parquet.Value, len(columns))
		for i, name := range columns {
			row[i] = parquet.ValueOf(m[name])
		}
		rows = append(rows, row)
	}

	pw := parquet.NewWriter(f, ps)
	if _, err := pw.WriteRows(rows); err != nil {
		pw.Close()
		return fmt.Errorf("write rows: %w", err)
	}
	if err := pw.Close(); err != nil {
		return fmt.Errorf("close parquet writer: %w", err)
	}
	return nil
}

// convertRecords converts interface records to typed struct pointers.
func convertRecords(records []interface{}, table string) []interface{} {
	var typed []interface{}

	for _, rec := range records {
		switch table {
		case "daily":
			m := toMap(rec)
			typed = append(typed, &schema.DailyRecord{
				TsCode:    m["ts_code"],
				TradeDate: m["trade_date"],
				Open:      parseFloat(m["open"]),
				High:      parseFloat(m["high"]),
				Low:       parseFloat(m["low"]),
				Close:     parseFloat(m["close"]),
				PreClose:  parseFloat(m["pre_close"]),
				Change:    parseFloat(m["change"]),
				PctChg:    parseFloat(m["pct_chg"]),
				Vol:       parseFloat(m["vol"]),
				Amount:    parseFloat(m["amount"]),
			})

		case "adj_factor":
			m := toMap(rec)
			typed = append(typed, &schema.AdjFactorRecord{
				TsCode:    m["ts_code"],
				TradeDate: m["trade_date"],
				AdjFactor: parseFloat(m["adj_factor"]),
			})

		case "trade_cal":
			m := toMap(rec)
			var isOpen int64
			if v, err := strconv.ParseInt(m["is_open"], 10, 64); err == nil {
				isOpen = v
			}
			typed = append(typed, &schema.TradeCalRecord{
				Exchange:     m["exchange"],
				CalDate:      m["cal_date"],
				IsOpen:       isOpen,
				PretradeDate: m["pretrade_date"],
			})

		case "stock_basic_exchange":
			m := toMap(rec)
			typed = append(typed, &schema.StockBasicRecord{
				InstrumentID:  m["instrument_id"],
				Symbol:        m["symbol"],
				Exchange:      m["exchange"],
				Name:          m["name"],
				Market:        m["market"],
				Region:        m["region"],
				Industry:      m["industry"],
				TotalShare:    parseFloat(m["total_share"]),
				FloatShare:    parseFloat(m["float_share"]),
				ListDate:      m["list_date"],
				DelistDate:    m["delist_date"],
				ListingStatus: m["listing_status"],
			})

		case "fin_income":
			m := toMap(rec)
			typed = append(typed, &schema.IncomeRecord{
				TsCode:           m["ts_code"],
				Symbol:           m["symbol"],
				Exchange:         m["exchange"],
				Name:             m["name"],
				Industry:         m["industry"],
				ReportDate:       m["report_date"],
				NoticeDate:       m["notice_date"],
				ReportType:       m["report_type"],
				Revenue:          parseFloat(m["revenue"]),
				OperatingCost:    parseFloat(m["operating_cost"]),
				TotalOperateCost: parseFloat(m["total_operate_cost"]),
				NetProfit:        parseFloat(m["net_profit"]),
				NetProfitDeduct:  parseFloat(m["net_profit_deduct"]),
				OperatingProfit:  parseFloat(m["operating_profit"]),
				TotalProfit:      parseFloat(m["total_profit"]),
				Tax:              parseFloat(m["tax"]),
				GrossMargin:      parseFloat(m["gross_margin"]),
			})

		case "fin_balance":
			m := toMap(rec)
			typed = append(typed, &schema.BalanceRecord{
				TsCode:           m["ts_code"],
				Symbol:           m["symbol"],
				Exchange:         m["exchange"],
				Name:             m["name"],
				Industry:         m["industry"],
				ReportDate:       m["report_date"],
				NoticeDate:       m["notice_date"],
				TotalAssets:      parseFloat(m["total_assets"]),
				TotalLiabilities: parseFloat(m["total_liabilities"]),
				TotalEquity:      parseFloat(m["total_equity"]),
				EquityRatio:      parseFloat(m["equity_ratio"]),
				DebtAssetRatio:   parseFloat(m["debt_asset_ratio"]),
				LeverageRatio:    parseFloat(m["leverage_ratio"]),
			})

		case "fin_cashflow":
			m := toMap(rec)
			typed = append(typed, &schema.CashflowRecord{
				TsCode:          m["ts_code"],
				Symbol:          m["symbol"],
				Exchange:        m["exchange"],
				Name:            m["name"],
				Industry:        m["industry"],
				ReportDate:      m["report_date"],
				NoticeDate:      m["notice_date"],
				OCF:             parseFloat(m["ocf"]),
				OCFRatio:        parseFloat(m["ocf_ratio"]),
				InvestCashflow:  parseFloat(m["invest_cashflow"]),
				FinanceCashflow: parseFloat(m["finance_cashflow"]),
				Capex:           parseFloat(m["capex"]),
				FreeCashFlow:    parseFloat(m["free_cash_flow"]),
				CashEnd:         parseFloat(m["cash_end"]),
				CashBegin:       parseFloat(m["cash_begin"]),
			})

		case "business_scope":
			m := toMap(rec)
			typed = append(typed, &schema.BusinessScopeRecord{
				TsCode:           m["ts_code"],
				Symbol:           m["symbol"],
				Exchange:         m["exchange"],
				ReportDate:       m["report_date"],
				MainopType:       m["mainop_type"],
				ItemName:         m["item_name"],
				Income:           parseFloat(m["income"]),
				IncomeRatio:      parseFloat(m["income_ratio"]),
				Cost:             parseFloat(m["cost"]),
				Profit:           parseFloat(m["profit"]),
				GrossProfitRatio: parseFloat(m["gross_profit_ratio"]),
				Rank:             parseInt64(m["rank"]),
			})

		case "earnings_forecast":
			m := toMap(rec)
			typed = append(typed, &schema.EarningsForecastRecord{
				TsCode:       m["ts_code"],
				Symbol:       m["symbol"],
				Exchange:     m["exchange"],
				Name:         m["name"],
				NoticeDate:   m["notice_date"],
				ReportDate:   m["report_date"],
				ForecastItem: m["forecast_item"],
				ForecastType: m["forecast_type"],
				AmountLower:  parseFloat(m["amount_lower"]),
				AmountUpper:  parseFloat(m["amount_upper"]),
				ChangeLower:  parseFloat(m["change_lower"]),
				ChangeUpper:  parseFloat(m["change_upper"]),
				Reason:       m["reason"],
				ReportPeriod: m["report_period"],
			})

		case "valuation_snapshot":
			m := toMap(rec)
			typed = append(typed, &schema.ValuationSnapshotRecord{
				TsCode:         m["ts_code"],
				Symbol:         m["symbol"],
				Exchange:       m["exchange"],
				Name:           m["name"],
				Industry:       m["industry"],
				TradeDate:      m["trade_date"],
				ClosePrice:     parseFloat(m["close_price"]),
				ChangePct:      parseFloat(m["change_pct"]),
				TotalMarketCap: parseFloat(m["total_market_cap"]),
				FreeMarketCap:  parseFloat(m["free_market_cap"]),
				TotalShares:    parseFloat(m["total_shares"]),
				FreeShares:     parseFloat(m["free_shares"]),
				PERatio:        parseFloat(m["pe_ttm"]),
				PELAR:          parseFloat(m["pe_lar"]),
				PBRatio:        parseFloat(m["pb"]),
				PSRatio:        parseFloat(m["ps_ttm"]),
				PCRatio:        parseFloat(m["pcf_ttm"]),
				PEG:            parseFloat(m["peg"]),
			})

		default:
			m := toMap(rec)
			typed = append(typed, &Record{Columns: m})
		}
	}

	return typed
}

// toMap converts a record to a map of column names to string values.
// Handles nil values by returning empty strings.
func toMap(rec interface{}) map[string]string {
	switch v := rec.(type) {
	case map[string]interface{}:
		return mapInterfaceToString(v)
	case map[string]string:
		result := make(map[string]string, len(v))
		for k, val := range v {
			result[k] = val
		}
		return result
	case *Record:
		if v.Columns == nil {
			return nil
		}
		result := make(map[string]string, len(v.Columns))
		for k, val := range v.Columns {
			result[k] = val
		}
		return result
	}
	if m, ok := toMapOfStruct(rec); ok {
		return m
	}
	return nil
}

// mapInterfaceToString renders interface values as strings, mapping nil to "".
func mapInterfaceToString(m map[string]interface{}) map[string]string {
	result := make(map[string]string, len(m))
	for k, v := range m {
		if v == nil {
			result[k] = ""
			continue
		}
		result[k] = scalarString(reflect.ValueOf(v))
	}
	return result
}

// toMapOfStruct flattens a struct into a column map using its parquet tags.
// Storage callers pass typed schema records rather than maps, so without this
// path every field would read back as zero.
func toMapOfStruct(rec interface{}) (map[string]string, bool) {
	v := reflect.ValueOf(rec)
	for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil, false
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil, false
	}

	t := v.Type()
	result := make(map[string]string, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		name := field.Tag.Get("parquet")
		if name == "-" {
			continue
		}
		if name == "" {
			name = field.Name
		}
		result[name] = scalarString(v.Field(i))
	}
	return result, true
}

// scalarString renders a scalar field value without losing float precision.
func scalarString(v reflect.Value) string {
	if !v.IsValid() {
		return ""
	}
	if v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return ""
		}
		return scalarString(v.Elem())
	}
	switch v.Kind() {
	case reflect.String:
		return v.String()
	case reflect.Bool:
		return strconv.FormatBool(v.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(v.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'g', -1, 64)
	case reflect.Map, reflect.Slice, reflect.Array:
		if v.IsNil() || v.Len() == 0 {
			return ""
		}
		return fmt.Sprintf("%v", v.Interface())
	case reflect.Struct:
		if tm, ok := v.Interface().(time.Time); ok {
			return tm.UTC().Format("2006-01-02 15:04:05")
		}
		return fmt.Sprintf("%v", v.Interface())
	default:
		return fmt.Sprintf("%v", v.Interface())
	}
}

func parseFloat(s string) float64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

func parseInt64(s string) int64 {
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return i
}

// Count returns the number of records in a table.
func (s *Store) Count(layer, table string) (int, error) {
	dir := s.config.DataDir(layer)
	countPath := filepath.Join(dir, table+".parquet.count")

	data, err := os.ReadFile(countPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read count metadata: %w", err)
	}

	return strconv.Atoi(string(data))
}

// Exists checks if a table exists in the given layer.
func (s *Store) Exists(layer, table string) bool {
	dir := s.config.DataDir(layer)
	_, err := os.Stat(filepath.Join(dir, table+".parquet"))
	return !os.IsNotExist(err)
}

// ListTables returns all tables in a given layer.
func (s *Store) ListTables(layer string) []string {
	dir := s.config.DataDir(layer)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var tables []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".parquet") {
			tables = append(tables, strings.TrimSuffix(e.Name(), ".parquet"))
		}
	}
	return tables
}

// Append merges records into an existing Parquet file.
//
// Parquet has a footer at the end of the file, so bytes cannot be appended in
// place. Merging means reading the current rows, combining them with the new
// ones, and replacing the file atomically. Writing over an existing file
// without truncating it leaves overlapping footers and produces a file that
// neither parquet-go nor DuckDB can open.
func (s *Store) Append(layer string, table string, records []interface{}) error {
	dir := s.config.DataDir(layer)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	if len(records) == 0 {
		return nil
	}

	schemaDef := schema.TableRegistry[table]
	if schemaDef == nil {
		return fmt.Errorf("unknown table: %s", table)
	}

	// The read-merge-write below is not atomic by itself: two concurrent
	// appends would both read the same pre-merge rows, and whichever rename
	// lands second wins, discarding the other's rows. Hold the table lock for
	// the whole region. Validation stays outside the critical section.
	mu := s.lockTable(layer, table)
	mu.Lock()
	defer mu.Unlock()

	path := filepath.Join(dir, table+".parquet")

	existing, err := readParquet(path, table)
	if err != nil {
		return fmt.Errorf("read existing rows: %w", err)
	}

	merged := make([]interface{}, 0, len(existing)+len(records))
	merged = append(merged, existing...)
	merged = append(merged, records...)

	if schemaDef.WriteMode == "upsert_by_key" && len(schemaDef.PrimaryKeys) > 0 {
		merged = dedupeByKeys(merged, schemaDef.PrimaryKeys)
	}

	if err := writeParquetAtomic(dir, path, table, merged); err != nil {
		return err
	}

	return writeCount(path, len(merged))
}

// readParquet reads every row of an existing file back into records of the same
// representation that convertRecords produces. A missing file yields no rows.
func readParquet(path, table string) ([]interface{}, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	r := parquet.NewReader(f)
	defer r.Close()

	if r.NumRows() == 0 {
		return nil, nil
	}

	out := make([]interface{}, 0, r.NumRows())
	for {
		var rec interface{}
		if typed := newTypedRecord(table); typed != nil {
			if err := r.Read(typed); err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return nil, err
			}
			rec = typed
		} else {
			// Generic tables are stored as a string column group. Reconstructing
			// into map[string]string fails on numeric columns, so read into a
			// pre-initialised map and normalise to strings. A nil map panics
			// inside parquet-go's reflection path.
			m := map[string]interface{}{}
			if err := r.Read(&m); err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return nil, err
			}
			rec = &Record{Columns: stringMap(m)}
		}
		out = append(out, rec)
	}
	return out, nil
}

// newTypedRecord returns a fresh pointer of the record type used for a table,
// or nil when the table is stored generically.
func newTypedRecord(table string) interface{} {
	switch table {
	case "daily":
		return &schema.DailyRecord{}
	case "adj_factor":
		return &schema.AdjFactorRecord{}
	case "trade_cal":
		return &schema.TradeCalRecord{}
	case "stock_basic_exchange":
		return &schema.StockBasicRecord{}
	case "fin_income":
		return &schema.IncomeRecord{}
	case "fin_balance":
		return &schema.BalanceRecord{}
	case "fin_cashflow":
		return &schema.CashflowRecord{}
	case "business_scope":
		return &schema.BusinessScopeRecord{}
	case "earnings_forecast":
		return &schema.EarningsForecastRecord{}
	case "valuation_snapshot":
		return &schema.ValuationSnapshotRecord{}
	default:
		return nil
	}
}

// dedupeByKeys keeps the last record seen for each primary key combination,
// which is the upsert semantics recorded in the table registry.
func dedupeByKeys(records []interface{}, keys []string) []interface{} {
	index := make(map[string]int, len(records))
	out := make([]interface{}, 0, len(records))
	for _, rec := range records {
		m := toMap(rec)
		if m == nil {
			out = append(out, rec)
			continue
		}
		parts := make([]string, 0, len(keys))
		for _, col := range keys {
			parts = append(parts, m[col])
		}
		k := strings.Join(parts, "\x00")
		if i, ok := index[k]; ok {
			out[i] = rec
			continue
		}
		index[k] = len(out)
		out = append(out, rec)
	}
	return out
}

// writeParquetAtomic writes records to a temporary file in the destination
// directory, then renames it over the target. Renaming is atomic on the same
// filesystem, so a reader never observes a partially written table.
func writeParquetAtomic(dir, path, table string, records []interface{}) error {
	if len(records) == 0 {
		return nil
	}

	tmp, err := os.CreateTemp(dir, table+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		tmp.Close()
		os.Remove(tmpName)
	}

	var writeErr error
	if isTyped(records) {
		writeErr = writeTyped(tmp, records)
	} else {
		writeErr = writeGeneric(tmp, table, records)
	}
	if writeErr != nil {
		cleanup()
		return writeErr
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("replace parquet file: %w", err)
	}
	return nil
}

// writeTyped writes struct records using a schema derived from the struct tags.
func writeTyped(f *os.File, records []interface{}) error {
	ps := parquet.SchemaOf(records[0])
	pw := parquet.NewWriter(f, ps)

	for _, rec := range records {
		if err := pw.Write(rec); err != nil {
			pw.Close()
			return fmt.Errorf("write record: %w", err)
		}
	}

	if err := pw.Close(); err != nil {
		return fmt.Errorf("close parquet writer: %w", err)
	}
	return nil
}

// writeCount persists the row count metadata that Count reads back.
func writeCount(path string, n int) error {
	if err := os.WriteFile(path+".count", []byte(strconv.Itoa(n)), 0644); err != nil {
		return fmt.Errorf("write count metadata: %w", err)
	}
	return nil
}

// stringMap converts parsed values to strings for the generic record shape.
func stringMap(m map[string]interface{}) map[string]string {
	result := make(map[string]string, len(m))
	for k, v := range m {
		if v == nil {
			result[k] = ""
			continue
		}
		result[k] = scalarString(reflect.ValueOf(v))
	}
	return result
}
