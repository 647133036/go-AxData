package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/electkismet/axdata-go/core/config"
	"github.com/electkismet/axdata-go/core/schema"
	parquet "github.com/parquet-go/parquet-go"
)

// Store provides Parquet-based data storage.
type Store struct {
	config *config.Config
}

// NewStore creates a new Store instance.
func NewStore(cfg *config.Config) *Store {
	return &Store{config: cfg}
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
		return s.Append(layer, table, typedRecords)
	}

	// Overwrite mode: write as new file
	path := filepath.Join(dir, table+".parquet")
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create parquet file: %w", err)
	}

	// Use parquet-go with typed struct
	if isTyped(typedRecords) {
		ps := parquet.SchemaOf(typedRecords[0])
		pw := parquet.NewWriter(f, ps)
		for _, rec := range typedRecords {
			if err := pw.Write(rec); err != nil {
				pw.Close()
				f.Close()
				return fmt.Errorf("write record: %w", err)
			}
		}
		if err := pw.Close(); err != nil {
			f.Close()
			return fmt.Errorf("close parquet writer: %w", err)
		}
	} else {
		if err := s.writeFileGeneric(f, table, records); err != nil {
			f.Close()
			return err
		}
	}
	f.Close()

	// Store row count metadata
	os.WriteFile(path+".count", []byte(strconv.Itoa(len(records))), 0644)

	return nil
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
func (s *Store) writeFileGeneric(f *os.File, table string, records []interface{}) error {
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
	if m, ok := rec.(map[string]interface{}); ok {
		result := make(map[string]string)
		for k, v := range m {
			if v == nil {
				result[k] = ""
				continue
			}
			result[k] = fmt.Sprintf("%v", v)
		}
		return result
	}
	return nil
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

// Append writes records to an existing Parquet file.
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

	path := filepath.Join(dir, table+".parquet")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}

	ps := parquet.SchemaOf(records[0])
	pw := parquet.NewWriter(f, ps)

	for _, rec := range records {
		if err := pw.Write(rec); err != nil {
			pw.Close()
			f.Close()
			return fmt.Errorf("write record: %w", err)
		}
	}

	if err := pw.Close(); err != nil {
		f.Close()
		return fmt.Errorf("close parquet writer: %w", err)
	}
	f.Close()

	// Update row count metadata
	countPath := path + ".count"
	countData, err := os.ReadFile(countPath)
	if err == nil {
		count, parseErr := strconv.Atoi(string(countData))
		if parseErr == nil {
			count += len(records)
			os.WriteFile(countPath, []byte(strconv.Itoa(count)), 0644)
		}
	} else {
		os.WriteFile(countPath, []byte(strconv.Itoa(len(records))), 0644)
	}

	return nil
}
