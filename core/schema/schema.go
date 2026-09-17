// Package schema defines the data models and table schemas for AxData.
// This mirrors the core schema from the original Python project.
package schema

// DailyRecord represents a daily OHLCV record for a stock.
type DailyRecord struct {
	TsCode    string  `parquet:"ts_code"`    // AxData stock ID, e.g., 000001.SZ
	TradeDate string  `parquet:"trade_date"` // YYYYMMDD
	Open      float64 `parquet:"open"`
	High      float64 `parquet:"high"`
	Low       float64 `parquet:"low"`
	Close     float64 `parquet:"close"`
	PreClose  float64 `parquet:"pre_close"`
	Change    float64 `parquet:"change"`
	PctChg    float64 `parquet:"pct_chg"`
	Vol       float64 `parquet:"vol"`
	Amount    float64 `parquet:"amount"`
}

// AdjFactorRecord represents an adjustment factor record.
type AdjFactorRecord struct {
	TsCode    string  `parquet:"ts_code"`
	TradeDate string  `parquet:"trade_date"`
	AdjFactor float64 `parquet:"adj_factor"`
}

// TradeCalRecord represents a trading calendar entry.
type TradeCalRecord struct {
	Exchange        string  `parquet:"exchange"`
	CalDate         string  `parquet:"cal_date"`
	IsOpen          int64   `parquet:"is_open"`
	PretradeDate    string  `parquet:"pretrade_date"`
	MarketSentiment string  `parquet:"market_sentiment"`
	MarketAmount    float64 `parquet:"market_amount"`
}

// StockBasicRecord represents stock basic information.
type StockBasicRecord struct {
	InstrumentID  string  `parquet:"instrument_id"`
	Symbol        string  `parquet:"symbol"`
	Exchange      string  `parquet:"exchange"`
	Name          string  `parquet:"name"`
	Market        string  `parquet:"market"`
	Region        string  `parquet:"region"`
	Industry      string  `parquet:"industry"`
	TotalShare    float64 `parquet:"total_share"`
	FloatShare    float64 `parquet:"float_share"`
	ListDate      string  `parquet:"list_date"`
	DelistDate    string  `parquet:"delist_date"`
	ListingStatus string  `parquet:"listing_status"`
	LastPrice     float64 `parquet:"last_price"`
	PreClose      float64 `parquet:"pre_close"`
	Open          float64 `parquet:"open"`
	High          float64 `parquet:"high"`
	Low           float64 `parquet:"low"`
	Change        float64 `parquet:"change"`
	ChangePct     float64 `parquet:"change_pct"`
	Amount        float64 `parquet:"amount"`
	Volume        float64 `parquet:"volume"`
	TurnoverRate  float64 `parquet:"turnover_rate"`
	VolumeRatio   float64 `parquet:"volume_ratio"`
	TotalMarket   float64 `parquet:"total_market"`
	CircMarket    float64 `parquet:"circ_market"`
	PeTTM         float64 `parquet:"pe_ttm"`
	Pb            float64 `parquet:"pb"`
}

// IncomeRecord represents one period of an income statement.
// Margin and ratio columns are fractions in [0,1].
type IncomeRecord struct {
	TsCode           string  `parquet:"ts_code"`
	Symbol           string  `parquet:"symbol"`
	Exchange         string  `parquet:"exchange"`
	Name             string  `parquet:"name"`
	Industry         string  `parquet:"industry"`
	ReportDate       string  `parquet:"report_date"`
	NoticeDate       string  `parquet:"notice_date"`
	ReportType       string  `parquet:"report_type"`
	Revenue          float64 `parquet:"revenue"`
	OperatingCost    float64 `parquet:"operating_cost"`
	TotalOperateCost float64 `parquet:"total_operate_cost"`
	NetProfit        float64 `parquet:"net_profit"`
	NetProfitDeduct  float64 `parquet:"net_profit_deduct"`
	OperatingProfit  float64 `parquet:"operating_profit"`
	TotalProfit      float64 `parquet:"total_profit"`
	Tax              float64 `parquet:"tax"`
	GrossMargin      float64 `parquet:"gross_margin"`
}

// BalanceRecord represents one period of a balance sheet.
type BalanceRecord struct {
	TsCode           string  `parquet:"ts_code"`
	Symbol           string  `parquet:"symbol"`
	Exchange         string  `parquet:"exchange"`
	Name             string  `parquet:"name"`
	Industry         string  `parquet:"industry"`
	ReportDate       string  `parquet:"report_date"`
	NoticeDate       string  `parquet:"notice_date"`
	TotalAssets      float64 `parquet:"total_assets"`
	TotalLiabilities float64 `parquet:"total_liabilities"`
	TotalEquity      float64 `parquet:"total_equity"`
	EquityRatio      float64 `parquet:"equity_ratio"`
	DebtAssetRatio   float64 `parquet:"debt_asset_ratio"`
	LeverageRatio    float64 `parquet:"leverage_ratio"`
}

// CashflowRecord represents one period of a cash flow statement.
type CashflowRecord struct {
	TsCode          string  `parquet:"ts_code"`
	Symbol          string  `parquet:"symbol"`
	Exchange        string  `parquet:"exchange"`
	Name            string  `parquet:"name"`
	Industry        string  `parquet:"industry"`
	ReportDate      string  `parquet:"report_date"`
	NoticeDate      string  `parquet:"notice_date"`
	OCF             float64 `parquet:"ocf"`
	OCFRatio        float64 `parquet:"ocf_ratio"`
	InvestCashflow  float64 `parquet:"invest_cashflow"`
	FinanceCashflow float64 `parquet:"finance_cashflow"`
	Capex           float64 `parquet:"capex"`
	FreeCashFlow    float64 `parquet:"free_cash_flow"`
	CashEnd         float64 `parquet:"cash_end"`
	CashBegin       float64 `parquet:"cash_begin"`
}

// BusinessScopeRecord represents one line of a main-business breakdown.
// Ratio columns are fractions in [0,1].
type BusinessScopeRecord struct {
	TsCode           string  `parquet:"ts_code"`
	Symbol           string  `parquet:"symbol"`
	Exchange         string  `parquet:"exchange"`
	ReportDate       string  `parquet:"report_date"`
	MainopType       string  `parquet:"mainop_type"`
	ItemName         string  `parquet:"item_name"`
	Income           float64 `parquet:"income"`
	IncomeRatio      float64 `parquet:"income_ratio"`
	Cost             float64 `parquet:"cost"`
	Profit           float64 `parquet:"profit"`
	GrossProfitRatio float64 `parquet:"gross_profit_ratio"`
	Rank             int64   `parquet:"rank"`
}

// EarningsForecastRecord represents one profit pre-announcement.
// ChangeLower and ChangeUpper are percentage points of growth.
type EarningsForecastRecord struct {
	TsCode       string  `parquet:"ts_code"`
	Symbol       string  `parquet:"symbol"`
	Exchange     string  `parquet:"exchange"`
	Name         string  `parquet:"name"`
	NoticeDate   string  `parquet:"notice_date"`
	ReportDate   string  `parquet:"report_date"`
	ForecastItem string  `parquet:"forecast_item"`
	ForecastType string  `parquet:"forecast_type"`
	AmountLower  float64 `parquet:"amount_lower"`
	AmountUpper  float64 `parquet:"amount_upper"`
	ChangeLower  float64 `parquet:"change_lower"`
	ChangeUpper  float64 `parquet:"change_upper"`
	Reason       string  `parquet:"reason"`
	ReportPeriod string  `parquet:"report_period"`
}

// ValuationSnapshotRecord represents one day of valuation metrics.
// ChangePct is in percentage points.
type ValuationSnapshotRecord struct {
	TsCode         string  `parquet:"ts_code"`
	Symbol         string  `parquet:"symbol"`
	Exchange       string  `parquet:"exchange"`
	Name           string  `parquet:"name"`
	Industry       string  `parquet:"industry"`
	TradeDate      string  `parquet:"trade_date"`
	ClosePrice     float64 `parquet:"close_price"`
	ChangePct      float64 `parquet:"change_pct"`
	TotalMarketCap float64 `parquet:"total_market_cap"`
	FreeMarketCap  float64 `parquet:"free_market_cap"`
	TotalShares    float64 `parquet:"total_shares"`
	FreeShares     float64 `parquet:"free_shares"`
	PERatio        float64 `parquet:"pe_ttm"`
	PELAR          float64 `parquet:"pe_lar"`
	PBRatio        float64 `parquet:"pb"`
	PSRatio        float64 `parquet:"ps_ttm"`
	PCRatio        float64 `parquet:"pcf_ttm"`
	PEG            float64 `parquet:"peg"`
}

// TableSchema holds metadata about a data table.
type TableSchema struct {
	Name        string
	Columns     []ColumnSchema
	PrimaryKeys []string
	Description string
	Layer       string // raw, staging, core, factor
	WriteMode   string // append, snapshot, overwrite_partition, replace_range, upsert_by_key
}

// ColumnSchema describes a single column.
type ColumnSchema struct {
	Name        string
	Type        string // string, int64, float64
	Nullable    bool
	Description string
	Unit        string
	IsPK        bool
}

// TableRegistry maps table names to their schemas.
// Extended with new tables for levistock, eastmoney, kph, cls, ths, wencai sources.
var TableRegistry = map[string]*TableSchema{
	"daily": &TableSchema{
		Name:        "daily",
		Description: "Daily OHLCV data for A-share stocks",
		Layer:       "core",
		WriteMode:   "append",
		PrimaryKeys: []string{"ts_code", "trade_date"},
		Columns: []ColumnSchema{
			{Name: "ts_code", Type: "string", IsPK: true, Description: "AxData stock ID"},
			{Name: "trade_date", Type: "string", IsPK: true, Description: "Trading date YYYYMMDD"},
			{Name: "open", Type: "float64", Description: "Opening price (CNY)"},
			{Name: "high", Type: "float64", Description: "Highest price (CNY)"},
			{Name: "low", Type: "float64", Description: "Lowest price (CNY)"},
			{Name: "close", Type: "float64", Description: "Closing price (CNY)"},
			{Name: "pre_close", Type: "float64", Description: "Previous close (CNY)"},
			{Name: "change", Type: "float64", Description: "Price change (CNY)"},
			{Name: "pct_chg", Type: "float64", Description: "Price change percent"},
			{Name: "vol", Type: "float64", Description: "Volume (lots)"},
			{Name: "amount", Type: "float64", Description: "Amount (thousand CNY)"},
		},
	},
	"adj_factor": &TableSchema{
		Name:        "adj_factor",
		Description: "Adjustment factors for dividends/splits",
		Layer:       "core",
		WriteMode:   "append",
		PrimaryKeys: []string{"ts_code", "trade_date"},
		Columns: []ColumnSchema{
			{Name: "ts_code", Type: "string", IsPK: true},
			{Name: "trade_date", Type: "string", IsPK: true},
			{Name: "adj_factor", Type: "float64", Description: "Adjustment factor"},
		},
	},
	"trade_cal": &TableSchema{
		Name:        "trade_cal",
		Description: "Trading calendar",
		Layer:       "core",
		WriteMode:   "append",
		PrimaryKeys: []string{"exchange", "cal_date"},
		Columns: []ColumnSchema{
			{Name: "exchange", Type: "string", IsPK: true},
			{Name: "cal_date", Type: "string", IsPK: true},
			{Name: "is_open", Type: "int64", Description: "1=open, 0=close"},
			{Name: "pretrade_date", Type: "string", Description: "Previous trading date"},
			{Name: "market_sentiment", Type: "string", Description: "Market sentiment text"},
			{Name: "market_amount", Type: "float64", Description: "Market total amount"},
		},
	},
	"stock_basic_exchange": &TableSchema{
		Name:        "stock_basic_exchange",
		Description: "Stock basic information",
		Layer:       "core",
		WriteMode:   "upsert_by_key",
		PrimaryKeys: []string{"instrument_id"},
		Columns: []ColumnSchema{
			{Name: "instrument_id", Type: "string", IsPK: true},
			{Name: "symbol", Type: "string"},
			{Name: "exchange", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "market", Type: "string"},
			{Name: "region", Type: "string"},
			{Name: "industry", Type: "string"},
			{Name: "total_share", Type: "float64", Description: "Total shares (1e8)"},
			{Name: "float_share", Type: "float64", Description: "Float shares (1e8)"},
			{Name: "list_date", Type: "string"},
			{Name: "delist_date", Type: "string"},
			{Name: "listing_status", Type: "string", Description: "listed/delisted/suspended"},
			{Name: "last_price", Type: "float64", Description: "Last price (CNY)"},
			{Name: "pre_close", Type: "float64", Description: "Previous close (CNY)"},
			{Name: "open", Type: "float64", Description: "Open price (CNY)"},
			{Name: "high", Type: "float64", Description: "High price (CNY)"},
			{Name: "low", Type: "float64", Description: "Low price (CNY)"},
			{Name: "change", Type: "float64", Description: "Price change (CNY)"},
			{Name: "change_pct", Type: "float64", Description: "Price change percent"},
			{Name: "amount", Type: "float64", Description: "Trading amount (CNY)"},
			{Name: "volume", Type: "float64", Description: "Trading volume (lots)"},
			{Name: "turnover_rate", Type: "float64", Description: "Turnover rate percent"},
			{Name: "volume_ratio", Type: "float64", Description: "Volume ratio"},
			{Name: "total_market", Type: "float64", Description: "Total market cap (CNY)"},
			{Name: "circ_market", Type: "float64", Description: "Circulating market cap (CNY)"},
			{Name: "pe_ttm", Type: "float64", Description: "P/E TTM"},
			{Name: "pb", Type: "float64", Description: "P/B ratio"},
		},
	},

	// ─── Market Index Tables ───
	"market_index_realtime": &TableSchema{
		Name:        "market_index_realtime",
		Description: "Real-time index snapshot (East Money)",
		Layer:       "core",
		WriteMode:   "upsert_by_key",
		PrimaryKeys: []string{"index_code"},
		Columns: []ColumnSchema{
			{Name: "index_code", Type: "string", IsPK: true},
			{Name: "index_name", Type: "string"},
			{Name: "last_price", Type: "float64"},
			{Name: "pre_close", Type: "float64"},
			{Name: "open", Type: "float64"},
			{Name: "high", Type: "float64"},
			{Name: "low", Type: "float64"},
			{Name: "change", Type: "float64"},
			{Name: "change_pct", Type: "float64"},
			{Name: "volume", Type: "float64"},
			{Name: "amount", Type: "float64"},
			{Name: "timestamp", Type: "string"},
		},
	},
	"market_index_all": &TableSchema{
		Name:        "market_index_all",
		Description: "Full index list (East Money)",
		Layer:       "core",
		WriteMode:   "upsert_by_key",
		PrimaryKeys: []string{"index_code"},
		Columns: []ColumnSchema{
			{Name: "index_code", Type: "string", IsPK: true},
			{Name: "index_name", Type: "string"},
			{Name: "last_price", Type: "float64"},
			{Name: "pre_close", Type: "float64"},
			{Name: "open", Type: "float64"},
			{Name: "high", Type: "float64"},
			{Name: "low", Type: "float64"},
			{Name: "change", Type: "float64"},
			{Name: "change_pct", Type: "float64"},
			{Name: "volume", Type: "float64"},
			{Name: "amount", Type: "float64"},
			{Name: "timestamp", Type: "string"},
		},
	},
	"market_sentiment": &TableSchema{
		Name:        "market_sentiment",
		Description: "Market sentiment/emotion snapshot (KPH/CLS)",
		Layer:       "core",
		WriteMode:   "upsert_by_key",
		PrimaryKeys: []string{"source", "trade_date"},
		Columns: []ColumnSchema{
			{Name: "source", Type: "string", IsPK: true},
			{Name: "trade_date", Type: "string", IsPK: true},
			{Name: "market_sentiment", Type: "string", Description: "Sentiment text (KPH)"},
			{Name: "market_amount", Type: "float64", Description: "Market total amount (KPH)"},
			{Name: "market_degree", Type: "float64", Description: "Market degree score (CLS)"},
		},
	},

	// ─── Sector Tables ───
	"sector_rank": &TableSchema{
		Name:        "sector_rank",
		Description: "Sector/concept ranking (KPH/CLS)",
		Layer:       "core",
		WriteMode:   "upsert_by_key",
		PrimaryKeys: []string{"source", "rank"},
		Columns: []ColumnSchema{
			{Name: "source", Type: "string", IsPK: true},
			{Name: "rank", Type: "int64", IsPK: true},
			{Name: "name", Type: "string"},
			{Name: "symbol", Type: "string"},
			{Name: "concept_id", Type: "string"},
			{Name: "type", Type: "string"},
			{Name: "total_amount", Type: "float64"},
			{Name: "total_volume", Type: "float64"},
			{Name: "avg_change_pct", Type: "float64"},
			{Name: "concept_count", Type: "int64"},
			{Name: "stock_count", Type: "int64"},
			{Name: "total", Type: "float64"},
			{Name: "up_count", Type: "int64"},
			{Name: "down_count", Type: "int64"},
			{Name: "limit_up", Type: "int64"},
			{Name: "limit_down", Type: "int64"},
			{Name: "flat_count", Type: "int64"},
			{Name: "no_change_count", Type: "int64"},
		},
	},
	"sector_rank_detail": &TableSchema{
		Name:        "sector_rank_detail",
		Description: "Sector/concept ranking details (KPH)",
		Layer:       "core",
		WriteMode:   "append",
		Columns: []ColumnSchema{
			{Name: "source", Type: "string"},
			{Name: "rank", Type: "int64"},
			{Name: "name", Type: "string"},
			{Name: "symbol", Type: "string"},
			{Name: "concept_id", Type: "string"},
			{Name: "total_amount", Type: "float64"},
			{Name: "total_volume", Type: "float64"},
			{Name: "avg_change_pct", Type: "float64"},
		},
	},
	"sector_plate_detail": &TableSchema{
		Name:        "sector_plate_detail",
		Description: "Sector plate constituent stocks (KPH)",
		Layer:       "core",
		WriteMode:   "append",
		PrimaryKeys: []string{"concept_id", "trade_date"},
		Columns: []ColumnSchema{
			{Name: "source", Type: "string"},
			{Name: "concept_id", Type: "string", IsPK: true},
			{Name: "concept_name", Type: "string"},
			{Name: "symbol", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "change_pct", Type: "float64"},
			{Name: "trade_date", Type: "string", IsPK: true},
		},
	},
	"sector_realtime_eastmoney": &TableSchema{
		Name:        "sector_realtime_eastmoney",
		Description: "Sector realtime snapshot (Eastmoney)",
		Layer:       "core",
		WriteMode:   "upsert_by_key",
		PrimaryKeys: []string{"sector_code"},
		Columns: []ColumnSchema{
			{Name: "sector_code", Type: "string", IsPK: true},
			{Name: "sector_name", Type: "string"},
			{Name: "sector_type", Type: "string"},
			{Name: "last_price", Type: "float64"},
			{Name: "change", Type: "float64"},
			{Name: "change_pct", Type: "float64"},
			{Name: "volume", Type: "float64"},
			{Name: "amount", Type: "float64"},
			{Name: "amplitude", Type: "float64"},
			{Name: "turnover_rate", Type: "float64"},
			{Name: "total_market_value", Type: "float64"},
			{Name: "main_inflow", Type: "float64"},
			{Name: "lead_stock_name", Type: "string"},
			{Name: "lead_stock_symbol", Type: "string"},
			{Name: "lead_stock_change_pct", Type: "float64"},
			{Name: "up_count", Type: "int64"},
			{Name: "down_count", Type: "int64"},
		},
	},
	"sector_constituents_eastmoney": &TableSchema{
		Name:        "sector_constituents_eastmoney",
		Description: "Sector constituent stocks (Eastmoney)",
		Layer:       "core",
		WriteMode:   "upsert_by_key",
		PrimaryKeys: []string{"sector_code", "instrument_id"},
		Columns: []ColumnSchema{
			{Name: "sector_code", Type: "string", IsPK: true},
			{Name: "instrument_id", Type: "string", IsPK: true},
			{Name: "symbol", Type: "string"},
			{Name: "exchange", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "last_price", Type: "float64"},
			{Name: "change", Type: "float64"},
			{Name: "change_pct", Type: "float64"},
			{Name: "volume", Type: "float64"},
			{Name: "amount", Type: "float64"},
			{Name: "amplitude", Type: "float64"},
			{Name: "turnover_rate", Type: "float64"},
			{Name: "pe_ttm", Type: "float64"},
			{Name: "volume_ratio", Type: "float64"},
			{Name: "high", Type: "float64"},
			{Name: "low", Type: "float64"},
			{Name: "open", Type: "float64"},
			{Name: "pre_close", Type: "float64"},
			{Name: "total_market_value", Type: "float64"},
			{Name: "float_market_value", Type: "float64"},
			{Name: "pb", Type: "float64"},
		},
	},
	"sector_concept_detail": &TableSchema{
		Name:        "sector_concept_detail",
		Description: "Sector concept constituent stocks (KPH)",
		Layer:       "core",
		WriteMode:   "append",
		PrimaryKeys: []string{"concept_id", "trade_date"},
		Columns: []ColumnSchema{
			{Name: "source", Type: "string"},
			{Name: "concept_id", Type: "string", IsPK: true},
			{Name: "concept_name", Type: "string"},
			{Name: "symbol", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "change_pct", Type: "float64"},
			{Name: "trade_date", Type: "string", IsPK: true},
		},
	},
	"sector_emotion_detail": &TableSchema{
		Name:        "sector_emotion_detail",
		Description: "Market emotion detail (KPH)",
		Layer:       "core",
		WriteMode:   "upsert_by_key",
		PrimaryKeys: []string{"source", "trade_date"},
		Columns: []ColumnSchema{
			{Name: "source", Type: "string", IsPK: true},
			{Name: "trade_date", Type: "string", IsPK: true},
			{Name: "total", Type: "float64"},
			{Name: "up_count", Type: "int64"},
			{Name: "down_count", Type: "int64"},
			{Name: "limit_up", Type: "int64"},
			{Name: "limit_down", Type: "int64"},
			{Name: "flat_count", Type: "int64"},
			{Name: "no_change_count", Type: "int64"},
			{Name: "avg_change_pct", Type: "float64"},
		},
	},

	// ─── Stock Ranking Tables ───
	"stock_rank": &TableSchema{
		Name:        "stock_rank",
		Description: "Stock ranking (CLS)",
		Layer:       "core",
		WriteMode:   "upsert_by_key",
		PrimaryKeys: []string{"source", "rank"},
		Columns: []ColumnSchema{
			{Name: "source", Type: "string", IsPK: true},
			{Name: "rank", Type: "int64", IsPK: true},
			{Name: "symbol", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "instrument_id", Type: "string"},
			{Name: "last_price", Type: "float64"},
			{Name: "change_pct", Type: "float64"},
			{Name: "change", Type: "float64"},
			{Name: "amount", Type: "float64"},
			{Name: "volume", Type: "float64"},
			{Name: "tags", Type: "string"},
		},
	},
	"stock_hot_rank": &TableSchema{
		Name:        "stock_hot_rank",
		Description: "Stock hot/ranking list (THS)",
		Layer:       "core",
		WriteMode:   "upsert_by_key",
		PrimaryKeys: []string{"source", "rank"},
		Columns: []ColumnSchema{
			{Name: "source", Type: "string", IsPK: true},
			{Name: "rank", Type: "int64", IsPK: true},
			{Name: "symbol", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "instrument_id", Type: "string"},
			{Name: "last_price", Type: "float64"},
			{Name: "change_pct", Type: "float64"},
			{Name: "change", Type: "float64"},
			{Name: "amount", Type: "float64"},
			{Name: "volume", Type: "float64"},
			{Name: "tags", Type: "string"},
			{Name: "tags_raw", Type: "string"},
		},
	},
	"stock_strategy": &TableSchema{
		Name:        "stock_strategy",
		Description: "Stock selection strategy results (Wencai)",
		Layer:       "core",
		WriteMode:   "append",
		Columns: []ColumnSchema{
			{Name: "strategy_name", Type: "string"},
			{Name: "strategy_type", Type: "string"},
			{Name: "strategy_category", Type: "string"},
			{Name: "symbol", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "instrument_id", Type: "string"},
			{Name: "exchange", Type: "string"},
			{Name: "market", Type: "string"},
			{Name: "last_price", Type: "float64"},
			{Name: "change_pct", Type: "float64"},
			{Name: "trade_date", Type: "string"},
		},
	},

	// ─── TDX Collector Tables (upstream axdata-source-tdx) ───
	"stock_suspension": &TableSchema{
		Name:        "stock_suspension",
		Description: "Stock suspension list (TDX)",
		Layer:       "core",
		WriteMode:   "snapshot",
		PrimaryKeys: []string{"instrument_id"},
		Columns: []ColumnSchema{
			{Name: "instrument_id", Type: "string", IsPK: true},
			{Name: "symbol", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "exchange", Type: "string"},
		},
	},
	"stock_st_list": &TableSchema{
		Name:        "stock_st_list",
		Description: "ST/*ST stock list (TDX)",
		Layer:       "core",
		WriteMode:   "snapshot",
		PrimaryKeys: []string{"instrument_id"},
		Columns: []ColumnSchema{
			{Name: "instrument_id", Type: "string", IsPK: true},
			{Name: "symbol", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "exchange", Type: "string"},
		},
	},
	"stock_daily_share": &TableSchema{
		Name:        "stock_daily_share",
		Description: "Daily share capital (TDX)",
		Layer:       "core",
		WriteMode:   "append",
		PrimaryKeys: []string{"instrument_id", "trade_date"},
		Columns: []ColumnSchema{
			{Name: "instrument_id", Type: "string", IsPK: true},
			{Name: "symbol", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "exchange", Type: "string"},
			{Name: "trade_date", Type: "string", IsPK: true},
			{Name: "total_share", Type: "float64"},
			{Name: "float_share", Type: "float64"},
		},
	},
	"stock_price_limit": &TableSchema{
		Name:        "stock_price_limit",
		Description: "Daily price limit (TDX)",
		Layer:       "core",
		WriteMode:   "append",
		PrimaryKeys: []string{"instrument_id", "trade_date"},
		Columns: []ColumnSchema{
			{Name: "instrument_id", Type: "string", IsPK: true},
			{Name: "symbol", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "exchange", Type: "string"},
			{Name: "trade_date", Type: "string", IsPK: true},
			{Name: "upper_limit", Type: "float64"},
			{Name: "lower_limit", Type: "float64"},
		},
	},
	"stock_limit_ladder": &TableSchema{
		Name:        "stock_limit_ladder",
		Description: "Continuous limit-up ladder (TDX)",
		Layer:       "core",
		WriteMode:   "snapshot",
		Columns: []ColumnSchema{
			{Name: "symbol", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "instrument_id", Type: "string"},
			{Name: "days", Type: "int64"},
			{Name: "change_pct", Type: "float64"},
		},
	},
	"stock_theme_rank": &TableSchema{
		Name:        "stock_theme_rank",
		Description: "Theme strength ranking (TDX)",
		Layer:       "core",
		WriteMode:   "snapshot",
		PrimaryKeys: []string{"rank"},
		Columns: []ColumnSchema{
			{Name: "rank", Type: "int64", IsPK: true},
			{Name: "name", Type: "string"},
			{Name: "change_pct", Type: "float64"},
			{Name: "total_amount", Type: "float64"},
			{Name: "total_volume", Type: "float64"},
		},
	},

	// ─── CNINFO (巨潮资讯) Tables ───
	"cninfo_announcements": &TableSchema{
		Name:        "cninfo_announcements",
		Description: "Company announcement list (CNINFO)",
		Layer:       "core",
		WriteMode:   "upsert_by_key",
		PrimaryKeys: []string{"instrument_id", "announcement_id"},
		Columns: []ColumnSchema{
			{Name: "instrument_id", Type: "string", IsPK: true},
			{Name: "symbol", Type: "string"},
			{Name: "exchange", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "announcement_id", Type: "string", IsPK: true},
			{Name: "title", Type: "string"},
			{Name: "publish_date", Type: "string", Description: "Announcement date YYYYMMDD"},
			{Name: "file_type", Type: "string"},
			{Name: "file_size_kb", Type: "float64"},
			{Name: "download_url", Type: "string"},
		},
	},
	"cninfo_irm": &TableSchema{
		Name:        "cninfo_irm",
		Description: "Investor relations Q&A (CNINFO IRM)",
		Layer:       "core",
		WriteMode:   "append",
		Columns: []ColumnSchema{
			{Name: "instrument_id", Type: "string"},
			{Name: "symbol", Type: "string"},
			{Name: "exchange", Type: "string"},
			{Name: "question_id", Type: "string"},
			{Name: "question", Type: "string"},
			{Name: "questioner", Type: "string"},
			{Name: "questioner_id", Type: "string"},
			{Name: "answer", Type: "string"},
			{Name: "answerer", Type: "string"},
			{Name: "answer_id", Type: "string"},
			{Name: "question_time", Type: "string"},
			{Name: "answer_time", Type: "string"},
			{Name: "update_time", Type: "string"},
		},
	},
	"stock_profile_cninfo": &TableSchema{
		Name:        "stock_profile_cninfo",
		Description: "Company profile (CNINFO)",
		Layer:       "core",
		WriteMode:   "upsert_by_key",
		PrimaryKeys: []string{"symbol"},
		Columns: []ColumnSchema{
			{Name: "symbol", Type: "string", IsPK: true},
			{Name: "instrument_id", Type: "string"},
			{Name: "exchange", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "company_name", Type: "string"},
			{Name: "english_name", Type: "string"},
			{Name: "former_short_name", Type: "string"},
			{Name: "a_share_code", Type: "string"},
			{Name: "a_share_name", Type: "string"},
			{Name: "b_share_code", Type: "string"},
			{Name: "b_share_name", Type: "string"},
			{Name: "h_share_code", Type: "string"},
			{Name: "h_share_name", Type: "string"},
			{Name: "selected_indexes", Type: "string"},
			{Name: "market", Type: "string"},
			{Name: "industry", Type: "string"},
			{Name: "legal_representative", Type: "string"},
			{Name: "registered_capital", Type: "float64"},
			{Name: "founded_date", Type: "string"},
			{Name: "listing_date", Type: "string"},
			{Name: "website", Type: "string"},
			{Name: "email", Type: "string"},
			{Name: "phone", Type: "string"},
			{Name: "fax", Type: "string"},
			{Name: "registered_address", Type: "string"},
			{Name: "office_address", Type: "string"},
			{Name: "postcode", Type: "string"},
			{Name: "main_business", Type: "string"},
			{Name: "business_scope", Type: "string"},
			{Name: "organization_profile", Type: "string"},
		},
	},
	"stock_dividend_cninfo": &TableSchema{
		Name:        "stock_dividend_cninfo",
		Description: "Dividend and bonus plan (CNINFO)",
		Layer:       "core",
		WriteMode:   "upsert_by_key",
		PrimaryKeys: []string{"instrument_id", "record_date"},
		Columns: []ColumnSchema{
			{Name: "instrument_id", Type: "string", IsPK: true},
			{Name: "symbol", Type: "string"},
			{Name: "exchange", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "announcement_date", Type: "string"},
			{Name: "dividend_type", Type: "string"},
			{Name: "bonus_share_ratio", Type: "float64"},
			{Name: "transfer_share_ratio", Type: "float64"},
			{Name: "cash_dividend_ratio", Type: "float64"},
			{Name: "record_date", Type: "string", IsPK: true},
			{Name: "ex_right_date", Type: "string"},
			{Name: "dividend_payment_date", Type: "string"},
			{Name: "share_arrival_date", Type: "string"},
			{Name: "plan_description", Type: "string"},
			{Name: "report_period", Type: "string"},
		},
	},
	"stock_industry_category_cninfo": &TableSchema{
		Name:        "stock_industry_category_cninfo",
		Description: "Industry classification (CNINFO)",
		Layer:       "core",
		WriteMode:   "upsert_by_key",
		PrimaryKeys: []string{"symbol"},
		Columns: []ColumnSchema{
			{Name: "symbol", Type: "string", IsPK: true},
			{Name: "instrument_id", Type: "string"},
			{Name: "exchange", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "parent_code", Type: "string"},
			{Name: "category_code", Type: "string"},
			{Name: "category_name", Type: "string"},
			{Name: "category_name_en", Type: "string"},
			{Name: "end_date", Type: "string"},
			{Name: "industry_type_code", Type: "string"},
			{Name: "industry_type", Type: "string"},
		},
	},
	"stock_hold_change_cninfo": &TableSchema{
		Name:        "stock_hold_change_cninfo",
		Description: "Shareholder holding change (CNINFO)",
		Layer:       "core",
		WriteMode:   "append",
		Columns: []ColumnSchema{
			{Name: "instrument_id", Type: "string"},
			{Name: "symbol", Type: "string"},
			{Name: "exchange", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "announcement_date", Type: "string"},
			{Name: "circulated_share", Type: "float64"},
			{Name: "total_share", Type: "float64"},
			{Name: "trade_market", Type: "string"},
		},
	},
	"stock_hold_control_cninfo": &TableSchema{
		Name:        "stock_hold_control_cninfo",
		Description: "Controlling shareholder (CNINFO)",
		Layer:       "core",
		WriteMode:   "upsert_by_key",
		PrimaryKeys: []string{"symbol", "change_date"},
		Columns: []ColumnSchema{
			{Name: "symbol", Type: "string", IsPK: true},
			{Name: "instrument_id", Type: "string"},
			{Name: "exchange", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "holding_ratio", Type: "float64"},
			{Name: "holding_shares", Type: "float64"},
			{Name: "actual_controller_name", Type: "string"},
			{Name: "direct_controller_name", Type: "string"},
			{Name: "control_type", Type: "string"},
			{Name: "change_date", Type: "string", IsPK: true},
		},
	},
	"stock_hold_num_cninfo": &TableSchema{
		Name:        "stock_hold_num_cninfo",
		Description: "Shareholder count statistics (CNINFO)",
		Layer:       "core",
		WriteMode:   "upsert_by_key",
		PrimaryKeys: []string{"symbol", "change_date"},
		Columns: []ColumnSchema{
			{Name: "symbol", Type: "string", IsPK: true},
			{Name: "instrument_id", Type: "string"},
			{Name: "exchange", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "avg_holding", Type: "float64"},
			{Name: "avg_holding_change_pct", Type: "float64"},
			{Name: "prev_avg_holding", Type: "float64"},
			{Name: "shareholder_count", Type: "float64"},
			{Name: "shareholder_count_change_pct", Type: "float64"},
			{Name: "prev_shareholder_count", Type: "float64"},
			{Name: "change_date", Type: "string", IsPK: true},
		},
	},
	"stock_share_change_cninfo": &TableSchema{
		Name:        "stock_share_change_cninfo",
		Description: "Share capital structure change (CNINFO)",
		Layer:       "core",
		WriteMode:   "append",
		Columns: []ColumnSchema{
			{Name: "instrument_id", Type: "string"},
			{Name: "symbol", Type: "string"},
			{Name: "exchange", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "organization_name", Type: "string"},
			{Name: "announcement_date", Type: "string"},
			{Name: "change_date", Type: "string"},
			{Name: "change_reason_code", Type: "string"},
			{Name: "change_reason", Type: "string"},
			{Name: "total_share", Type: "float64"},
			{Name: "non_circulating_share", Type: "float64"},
			{Name: "circulating_share", Type: "float64"},
			{Name: "a_share", Type: "float64"},
			{Name: "b_share", Type: "float64"},
			{Name: "h_share", Type: "float64"},
			{Name: "restricted_share", Type: "float64"},
			{Name: "executive_share", Type: "float64"},
		},
	},
	"stock_allotment_cninfo": &TableSchema{
		Name:        "stock_allotment_cninfo",
		Description: "Share allotment (CNINFO)",
		Layer:       "core",
		WriteMode:   "upsert_by_key",
		PrimaryKeys: []string{"instrument_id"},
		Columns: []ColumnSchema{
			{Name: "instrument_id", Type: "string", IsPK: true},
			{Name: "symbol", Type: "string"},
			{Name: "exchange", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "allotment_type", Type: "string"},
			{Name: "allotment_date", Type: "string"},
			{Name: "allotment_share", Type: "float64"},
		},
	},
	"cninfo_announcement_detail": &TableSchema{
		Name:        "cninfo_announcement_detail",
		Description: "Announcement detail (CNINFO)",
		Layer:       "core",
		WriteMode:   "append",
		Columns: []ColumnSchema{
			{Name: "announcement_id", Type: "string"},
			{Name: "instrument_id", Type: "string"},
			{Name: "symbol", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "title", Type: "string"},
			{Name: "announcement_date", Type: "string"},
			{Name: "announcement_url", Type: "string"},
		},
	},
	"stock_irm_cninfo": &TableSchema{
		Name:        "stock_irm_cninfo",
		Description: "IRM questions (CNINFO)",
		Layer:       "core",
		WriteMode:   "append",
		Columns: []ColumnSchema{
			{Name: "question_id", Type: "string"},
			{Name: "instrument_id", Type: "string"},
			{Name: "symbol", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "question", Type: "string"},
			{Name: "question_date", Type: "string"},
			{Name: "investor_type", Type: "string"},
			{Name: "up_vote_count", Type: "int64"},
		},
	},
	"stock_irm_ans_cninfo": &TableSchema{
		Name:        "stock_irm_ans_cninfo",
		Description: "IRM answers (CNINFO)",
		Layer:       "core",
		WriteMode:   "append",
		Columns: []ColumnSchema{
			{Name: "answer_id", Type: "string"},
			{Name: "question_id", Type: "string"},
			{Name: "instrument_id", Type: "string"},
			{Name: "symbol", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "answer", Type: "string"},
			{Name: "answer_date", Type: "string"},
			{Name: "answerer", Type: "string"},
		},
	},
	"stock_zh_a_disclosure_report_cninfo": &TableSchema{
		Name:        "stock_zh_a_disclosure_report_cninfo",
		Description: "Disclosure reports (CNINFO)",
		Layer:       "core",
		WriteMode:   "append",
		Columns: []ColumnSchema{
			{Name: "instrument_id", Type: "string"},
			{Name: "symbol", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "report_type", Type: "string"},
			{Name: "report_date", Type: "string"},
			{Name: "title", Type: "string"},
			{Name: "announcement_id", Type: "string"},
		},
	},
	"stock_zh_a_disclosure_relation_cninfo": &TableSchema{
		Name:        "stock_zh_a_disclosure_relation_cninfo",
		Description: "Related disclosure (CNINFO)",
		Layer:       "core",
		WriteMode:   "append",
		Columns: []ColumnSchema{
			{Name: "instrument_id", Type: "string"},
			{Name: "symbol", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "related_party", Type: "string"},
			{Name: "relation_type", Type: "string"},
			{Name: "disclosure_date", Type: "string"},
			{Name: "announcement_id", Type: "string"},
		},
	},
	"stock_new_gh_cninfo": &TableSchema{
		Name:        "stock_new_gh_cninfo",
		Description: "New stock issuance (CNINFO)",
		Layer:       "core",
		WriteMode:   "upsert_by_key",
		PrimaryKeys: []string{"instrument_id"},
		Columns: []ColumnSchema{
			{Name: "instrument_id", Type: "string", IsPK: true},
			{Name: "symbol", Type: "string"},
			{Name: "exchange", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "issuance_type", Type: "string"},
			{Name: "issuance_date", Type: "string"},
			{Name: "issuance_price", Type: "float64"},
			{Name: "issuance_quantity", Type: "float64"},
			{Name: "underwriter", Type: "string"},
		},
	},
	"stock_new_ipo_cninfo": &TableSchema{
		Name:        "stock_new_ipo_cninfo",
		Description: "IPO information (CNINFO)",
		Layer:       "core",
		WriteMode:   "upsert_by_key",
		PrimaryKeys: []string{"instrument_id"},
		Columns: []ColumnSchema{
			{Name: "instrument_id", Type: "string", IsPK: true},
			{Name: "symbol", Type: "string"},
			{Name: "exchange", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "ipo_date", Type: "string"},
			{Name: "listing_date", Type: "string"},
			{Name: "issue_price", Type: "float64"},
			{Name: "issue_quantity", Type: "float64"},
			{Name: "pe_ratio", Type: "float64"},
			{Name: "underwriter", Type: "string"},
		},
	},
	"stock_cg_equity_mortgage_cninfo": &TableSchema{
		Name:        "stock_cg_equity_mortgage_cninfo",
		Description: "Equity mortgage info (CNINFO)",
		Layer:       "core",
		WriteMode:   "append",
		Columns: []ColumnSchema{
			{Name: "instrument_id", Type: "string"},
			{Name: "symbol", Type: "string"},
			{Name: "exchange", Type: "string"},
			{Name: "name", Type: "string"},
		},
	},
	"stock_ipo_summary_cninfo": &TableSchema{
		Name:        "stock_ipo_summary_cninfo",
		Description: "IPO summary (CNINFO)",
		Layer:       "core",
		WriteMode:   "upsert_by_key",
		PrimaryKeys: []string{"symbol"},
		Columns: []ColumnSchema{
			{Name: "symbol", Type: "string", IsPK: true},
			{Name: "instrument_id", Type: "string"},
			{Name: "exchange", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "prospectus_announcement_date", Type: "string"},
			{Name: "lottery_rate_announcement_date", Type: "string"},
			{Name: "par_value", Type: "float64"},
			{Name: "total_issue_shares", Type: "float64"},
			{Name: "nav_per_share_before_issue", Type: "float64"},
			{Name: "diluted_pe", Type: "float64"},
			{Name: "raised_funds_net", Type: "float64"},
			{Name: "online_issue_date", Type: "string"},
		},
	},
	"fund_report_asset_allocation_cninfo": &TableSchema{
		Name:        "fund_report_asset_allocation_cninfo",
		Description: "Fund asset allocation report (CNINFO)",
		Layer:       "core",
		WriteMode:   "append",
		Columns: []ColumnSchema{
			{Name: "report_date", Type: "string"},
			{Name: "fund_count", Type: "float64"},
			{Name: "equity_asset_pct", Type: "float64"},
			{Name: "bond_asset_pct", Type: "float64"},
			{Name: "cash_asset_pct", Type: "float64"},
			{Name: "fund_market_net_assets", Type: "float64"},
		},
	},
	"fund_report_industry_allocation_cninfo": &TableSchema{
		Name:        "fund_report_industry_allocation_cninfo",
		Description: "Fund industry allocation report (CNINFO)",
		Layer:       "core",
		WriteMode:   "append",
		Columns: []ColumnSchema{
			{Name: "industry_code", Type: "string"},
			{Name: "industry_name", Type: "string"},
			{Name: "report_date", Type: "string"},
			{Name: "fund_count", Type: "float64"},
			{Name: "industry_scale", Type: "float64"},
			{Name: "net_asset_pct", Type: "float64"},
		},
	},
	"fund_report_stock_cninfo": &TableSchema{
		Name:        "fund_report_stock_cninfo",
		Description: "Fund stock holdings (CNINFO)",
		Layer:       "core",
		WriteMode:   "append",
		Columns: []ColumnSchema{
			{Name: "record_id", Type: "string"},
			{Name: "symbol", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "report_date", Type: "string"},
			{Name: "fund_count", Type: "float64"},
			{Name: "holding_shares", Type: "float64"},
			{Name: "holding_market_value", Type: "float64"},
		},
	},
	"stock_cg_guarantee_cninfo": &TableSchema{
		Name:        "stock_cg_guarantee_cninfo",
		Description: "Company guarantee info (CNINFO)",
		Layer:       "core",
		WriteMode:   "append",
		Columns: []ColumnSchema{
			{Name: "instrument_id", Type: "string"},
			{Name: "symbol", Type: "string"},
			{Name: "exchange", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "announcement_period", Type: "string"},
			{Name: "guarantee_amount", Type: "float64"},
			{Name: "guarantee_amount_net_asset_pct", Type: "float64"},
			{Name: "guarantee_count", Type: "float64"},
			{Name: "parent_equity", Type: "float64"},
		},
	},
	"stock_cg_lawsuit_cninfo": &TableSchema{
		Name:        "stock_cg_lawsuit_cninfo",
		Description: "Company lawsuit info (CNINFO)",
		Layer:       "core",
		WriteMode:   "append",
		Columns: []ColumnSchema{
			{Name: "instrument_id", Type: "string"},
			{Name: "symbol", Type: "string"},
			{Name: "exchange", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "announcement_period", Type: "string"},
			{Name: "lawsuit_amount", Type: "float64"},
			{Name: "lawsuit_count", Type: "float64"},
		},
	},
	"stock_hold_management_detail_cninfo": &TableSchema{
		Name:        "stock_hold_management_detail_cninfo",
		Description: "Shareholder management detail (CNINFO)",
		Layer:       "core",
		WriteMode:   "append",
		Columns: []ColumnSchema{
			{Name: "instrument_id", Type: "string"},
			{Name: "symbol", Type: "string"},
			{Name: "exchange", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "announcement_date", Type: "string"},
		},
	},
	"stock_industry_change_cninfo": &TableSchema{
		Name:        "stock_industry_change_cninfo",
		Description: "Industry change history (CNINFO)",
		Layer:       "core",
		WriteMode:   "append",
		Columns: []ColumnSchema{
			{Name: "instrument_id", Type: "string"},
			{Name: "symbol", Type: "string"},
			{Name: "exchange", Type: "string"},
			{Name: "name", Type: "string"},
		},
	},
	"stock_industry_pe_ratio_cninfo": &TableSchema{
		Name:        "stock_industry_pe_ratio_cninfo",
		Description: "Industry PE ratio (CNINFO)",
		Layer:       "core",
		WriteMode:   "append",
		Columns: []ColumnSchema{
			{Name: "instrument_id", Type: "string"},
			{Name: "symbol", Type: "string"},
			{Name: "exchange", Type: "string"},
			{Name: "name", Type: "string"},
		},
	},
	"stock_rank_forecast_cninfo": &TableSchema{
		Name:        "stock_rank_forecast_cninfo",
		Description: "Performance forecast ranking (CNINFO)",
		Layer:       "core",
		WriteMode:   "append",
		Columns: []ColumnSchema{
			{Name: "instrument_id", Type: "string"},
			{Name: "symbol", Type: "string"},
			{Name: "exchange", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "publish_date", Type: "string"},
			{Name: "previous_rating", Type: "string"},
			{Name: "rating_change", Type: "string"},
			{Name: "target_price_high", Type: "float64"},
			{Name: "target_price_low", Type: "float64"},
			{Name: "is_first_rating", Type: "string"},
			{Name: "rating", Type: "string"},
			{Name: "analyst_name", Type: "string"},
			{Name: "institution_short_name", Type: "string"},
		},
	},
	"bond_corporate_issue_cninfo": &TableSchema{
		Name:        "bond_corporate_issue_cninfo",
		Description: "Corporate bond issue (CNINFO)",
		Layer:       "core",
		WriteMode:   "append",
		Columns: []ColumnSchema{
			{Name: "bond_code", Type: "string"},
			{Name: "bond_name", Type: "string"},
			{Name: "bond_short_name", Type: "string"},
			{Name: "announcement_date", Type: "string"},
			{Name: "online_issue_start_date", Type: "string"},
			{Name: "online_issue_end_date", Type: "string"},
			{Name: "planned_issue_amount", Type: "float64"},
			{Name: "actual_issue_amount", Type: "float64"},
			{Name: "par_value", Type: "float64"},
			{Name: "issue_price", Type: "float64"},
			{Name: "issue_method", Type: "string"},
			{Name: "issue_target", Type: "string"},
			{Name: "issue_scope", Type: "string"},
			{Name: "underwriting_method", Type: "string"},
			{Name: "min_subscription_unit", Type: "float64"},
			{Name: "fundraising_use", Type: "string"},
			{Name: "min_subscription_amount", Type: "float64"},
		},
	},
	"bond_cov_issue_cninfo": &TableSchema{
		Name:        "bond_cov_issue_cninfo",
		Description: "Convertible bond issue (CNINFO)",
		Layer:       "core",
		WriteMode:   "append",
		Columns: []ColumnSchema{
			{Name: "bond_code", Type: "string"},
			{Name: "bond_name", Type: "string"},
			{Name: "bond_short_name", Type: "string"},
			{Name: "announcement_date", Type: "string"},
			{Name: "issue_start_date", Type: "string"},
			{Name: "issue_end_date", Type: "string"},
			{Name: "planned_issue_amount", Type: "float64"},
			{Name: "actual_issue_amount", Type: "float64"},
			{Name: "par_value", Type: "float64"},
			{Name: "issue_price", Type: "float64"},
			{Name: "issue_method", Type: "string"},
			{Name: "issue_target", Type: "string"},
			{Name: "issue_scope", Type: "string"},
			{Name: "underwriting_method", Type: "string"},
			{Name: "fundraising_use", Type: "string"},
			{Name: "initial_conversion_price", Type: "float64"},
			{Name: "conversion_start_date", Type: "string"},
			{Name: "conversion_end_date", Type: "string"},
			{Name: "trading_market", Type: "string"},
			{Name: "conversion_code", Type: "string"},
		},
	},
	"bond_cov_stock_issue_cninfo": &TableSchema{
		Name:        "bond_cov_stock_issue_cninfo",
		Description: "Convertible bond constituent stock (CNINFO)",
		Layer:       "core",
		WriteMode:   "append",
		Columns: []ColumnSchema{
			{Name: "bond_code", Type: "string"},
			{Name: "bond_name", Type: "string"},
			{Name: "bond_short_name", Type: "string"},
			{Name: "announcement_date", Type: "string"},
			{Name: "conversion_code", Type: "string"},
			{Name: "conversion_short_name", Type: "string"},
			{Name: "conversion_price", Type: "float64"},
			{Name: "voluntary_conversion_start_date", Type: "string"},
			{Name: "voluntary_conversion_end_date", Type: "string"},
			{Name: "underlying_stock", Type: "string"},
		},
	},
	"bond_local_government_issue_cninfo": &TableSchema{
		Name:        "bond_local_government_issue_cninfo",
		Description: "Local government bond issue (CNINFO)",
		Layer:       "core",
		WriteMode:   "append",
		Columns: []ColumnSchema{
			{Name: "bond_code", Type: "string"},
			{Name: "bond_name", Type: "string"},
			{Name: "bond_short_name", Type: "string"},
			{Name: "issue_start_date", Type: "string"},
			{Name: "issue_end_date", Type: "string"},
			{Name: "planned_issue_amount", Type: "float64"},
			{Name: "actual_issue_amount", Type: "float64"},
			{Name: "par_value", Type: "float64"},
			{Name: "issue_price", Type: "float64"},
		},
	},
	"bond_treasure_issue_cninfo": &TableSchema{
		Name:        "bond_treasure_issue_cninfo",
		Description: "Treasury bond issue (CNINFO)",
		Layer:       "core",
		WriteMode:   "append",
		Columns: []ColumnSchema{
			{Name: "bond_code", Type: "string"},
			{Name: "bond_name", Type: "string"},
			{Name: "issue_date", Type: "string"},
			{Name: "issue_amount", Type: "float64"},
			{Name: "maturity", Type: "string"},
		},
	},

	// ─── Eastmoney (东方财富) Tables ───
	"stock_limit_pool_eastmoney": &TableSchema{
		Name:        "stock_limit_pool_eastmoney",
		Description: "Limit up/down pool (Eastmoney)",
		Layer:       "core",
		WriteMode:   "snapshot",
		Columns: []ColumnSchema{
			{Name: "trade_date", Type: "string"},
			{Name: "instrument_id", Type: "string"},
			{Name: "symbol", Type: "string"},
			{Name: "exchange", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "market_code", Type: "string"},
			{Name: "last_price", Type: "float64"},
			{Name: "limit_price", Type: "float64"},
			{Name: "change_pct", Type: "float64"},
			{Name: "amount", Type: "float64"},
			{Name: "float_market_value", Type: "float64"},
			{Name: "turnover_rate", Type: "float64"},
			{Name: "first_limit_time", Type: "string"},
			{Name: "last_limit_time", Type: "string"},
			{Name: "continuous_count", Type: "int64"},
			{Name: "open_times", Type: "int64"},
			{Name: "main_inflow", Type: "float64"},
			{Name: "sector", Type: "string"},
			{Name: "zt_days", Type: "int64"},
			{Name: "zt_count", Type: "int64"},
		},
	},
	"stock_realtime_change_eastmoney": &TableSchema{
		Name:        "stock_realtime_change_eastmoney",
		Description: "Real-time stock change detail (Eastmoney)",
		Layer:       "core",
		WriteMode:   "append",
		Columns: []ColumnSchema{
			{Name: "trade_date", Type: "string"},
			{Name: "instrument_id", Type: "string"},
			{Name: "symbol", Type: "string"},
			{Name: "exchange", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "market_code", Type: "string"},
			{Name: "change_time", Type: "string"},
			{Name: "change_type", Type: "string"},
			{Name: "change_type_name", Type: "string"},
			{Name: "price", Type: "float64"},
			{Name: "change_pct", Type: "float64"},
			{Name: "volume", Type: "float64"},
		},
	},
	"stock_dragon_tiger_eastmoney": &TableSchema{
		Name:        "stock_dragon_tiger_eastmoney",
		Description: "Dragon Tiger daily data (Eastmoney)",
		Layer:       "core",
		WriteMode:   "append",
		Columns: []ColumnSchema{
			{Name: "trade_date", Type: "string"},
			{Name: "instrument_id", Type: "string"},
			{Name: "symbol", Type: "string"},
			{Name: "exchange", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "reason", Type: "string"},
			{Name: "close_price", Type: "float64"},
			{Name: "change_pct", Type: "float64"},
			{Name: "turnover_rate", Type: "float64"},
			{Name: "buy_amount", Type: "float64"},
			{Name: "sell_amount", Type: "float64"},
			{Name: "net_buy_amount", Type: "float64"},
			{Name: "total_amount", Type: "float64"},
			{Name: "market", Type: "string"},
		},
	},
	"stock_margin_trading_eastmoney": &TableSchema{
		Name:        "stock_margin_trading_eastmoney",
		Description: "Margin trading data (Eastmoney)",
		Layer:       "core",
		WriteMode:   "append",
		Columns: []ColumnSchema{
			{Name: "trade_date", Type: "string"},
			{Name: "instrument_id", Type: "string"},
			{Name: "symbol", Type: "string"},
			{Name: "exchange", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "market", Type: "string"},
			{Name: "close_price", Type: "float64"},
			{Name: "change_pct", Type: "float64"},
			{Name: "margin_balance", Type: "float64"},
			{Name: "margin_buy_amount", Type: "float64"},
			{Name: "margin_repay_amount", Type: "float64"},
			{Name: "margin_net_buy_amount", Type: "float64"},
			{Name: "short_balance", Type: "float64"},
			{Name: "short_sell_volume", Type: "float64"},
			{Name: "short_repay_volume", Type: "float64"},
			{Name: "short_net_sell_volume", Type: "float64"},
			{Name: "total_balance", Type: "float64"},
			{Name: "market_value", Type: "float64"},
		},
	},
	"stock_research_reports_eastmoney": &TableSchema{
		Name:        "stock_research_reports_eastmoney",
		Description: "Research reports (Eastmoney)",
		Layer:       "core",
		WriteMode:   "append",
		Columns: []ColumnSchema{
			{Name: "report_id", Type: "string"},
			{Name: "instrument_id", Type: "string"},
			{Name: "symbol", Type: "string"},
			{Name: "exchange", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "title", Type: "string"},
			{Name: "publish_date", Type: "string"},
			{Name: "org_name", Type: "string"},
			{Name: "rating", Type: "string"},
			{Name: "rating_change", Type: "string"},
			{Name: "researcher", Type: "string"},
			{Name: "eps_forecast_this_year", Type: "float64"},
			{Name: "pe_forecast_this_year", Type: "float64"},
			{Name: "file_size_kb", Type: "float64"},
			{Name: "page_count", Type: "int64"},
		},
	},
	"fin_income": &TableSchema{
		Name:        "fin_income",
		Description: "Income statements (Eastmoney datacenter)",
		Layer:       "core",
		WriteMode:   "snapshot",
		Columns: []ColumnSchema{
			{Name: "ts_code", Type: "string"},
			{Name: "symbol", Type: "string"},
			{Name: "exchange", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "industry", Type: "string"},
			{Name: "report_date", Type: "string"},
			{Name: "notice_date", Type: "string"},
			{Name: "report_type", Type: "string"},
			{Name: "revenue", Type: "float64"},
			{Name: "operating_cost", Type: "float64"},
			{Name: "total_operate_cost", Type: "float64"},
			{Name: "net_profit", Type: "float64"},
			{Name: "net_profit_deduct", Type: "float64"},
			{Name: "operating_profit", Type: "float64"},
			{Name: "total_profit", Type: "float64"},
			{Name: "tax", Type: "float64"},
			{Name: "gross_margin", Type: "float64"},
		},
	},
	"fin_balance": &TableSchema{
		Name:        "fin_balance",
		Description: "Balance sheets (Eastmoney datacenter)",
		Layer:       "core",
		WriteMode:   "snapshot",
		Columns: []ColumnSchema{
			{Name: "ts_code", Type: "string"},
			{Name: "symbol", Type: "string"},
			{Name: "exchange", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "industry", Type: "string"},
			{Name: "report_date", Type: "string"},
			{Name: "notice_date", Type: "string"},
			{Name: "total_assets", Type: "float64"},
			{Name: "total_liabilities", Type: "float64"},
			{Name: "total_equity", Type: "float64"},
			{Name: "equity_ratio", Type: "float64"},
			{Name: "debt_asset_ratio", Type: "float64"},
			{Name: "leverage_ratio", Type: "float64"},
		},
	},
	"fin_cashflow": &TableSchema{
		Name:        "fin_cashflow",
		Description: "Cash flow statements (Eastmoney datacenter)",
		Layer:       "core",
		WriteMode:   "snapshot",
		Columns: []ColumnSchema{
			{Name: "ts_code", Type: "string"},
			{Name: "symbol", Type: "string"},
			{Name: "exchange", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "industry", Type: "string"},
			{Name: "report_date", Type: "string"},
			{Name: "notice_date", Type: "string"},
			{Name: "ocf", Type: "float64"},
			{Name: "ocf_ratio", Type: "float64"},
			{Name: "invest_cashflow", Type: "float64"},
			{Name: "finance_cashflow", Type: "float64"},
			{Name: "capex", Type: "float64"},
			{Name: "free_cash_flow", Type: "float64"},
			{Name: "cash_end", Type: "float64"},
			{Name: "cash_begin", Type: "float64"},
		},
	},
	"business_scope": &TableSchema{
		Name:        "business_scope",
		Description: "Main business composition (Eastmoney HSF10)",
		Layer:       "core",
		WriteMode:   "snapshot",
		Columns: []ColumnSchema{
			{Name: "ts_code", Type: "string"},
			{Name: "symbol", Type: "string"},
			{Name: "exchange", Type: "string"},
			{Name: "report_date", Type: "string"},
			{Name: "mainop_type", Type: "string"},
			{Name: "item_name", Type: "string"},
			{Name: "income", Type: "float64"},
			{Name: "income_ratio", Type: "float64"},
			{Name: "cost", Type: "float64"},
			{Name: "profit", Type: "float64"},
			{Name: "gross_profit_ratio", Type: "float64"},
			{Name: "rank", Type: "int64"},
		},
	},
	"earnings_forecast": &TableSchema{
		Name:        "earnings_forecast",
		Description: "Profit pre-announcements (Eastmoney datacenter)",
		Layer:       "core",
		WriteMode:   "snapshot",
		Columns: []ColumnSchema{
			{Name: "ts_code", Type: "string"},
			{Name: "symbol", Type: "string"},
			{Name: "exchange", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "notice_date", Type: "string"},
			{Name: "report_date", Type: "string"},
			{Name: "forecast_item", Type: "string"},
			{Name: "forecast_type", Type: "string"},
			{Name: "amount_lower", Type: "float64"},
			{Name: "amount_upper", Type: "float64"},
			{Name: "change_lower", Type: "float64"},
			{Name: "change_upper", Type: "float64"},
			{Name: "reason", Type: "string"},
			{Name: "report_period", Type: "string"},
		},
	},
	"valuation_snapshot": &TableSchema{
		Name:        "valuation_snapshot",
		Description: "Daily valuation metrics (Eastmoney datacenter)",
		Layer:       "core",
		WriteMode:   "snapshot",
		Columns: []ColumnSchema{
			{Name: "ts_code", Type: "string"},
			{Name: "symbol", Type: "string"},
			{Name: "exchange", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "industry", Type: "string"},
			{Name: "trade_date", Type: "string"},
			{Name: "close_price", Type: "float64"},
			{Name: "change_pct", Type: "float64"},
			{Name: "total_market_cap", Type: "float64"},
			{Name: "free_market_cap", Type: "float64"},
			{Name: "total_shares", Type: "float64"},
			{Name: "free_shares", Type: "float64"},
			{Name: "pe_ttm", Type: "float64"},
			{Name: "pe_lar", Type: "float64"},
			{Name: "pb", Type: "float64"},
			{Name: "ps_ttm", Type: "float64"},
			{Name: "pcf_ttm", Type: "float64"},
			{Name: "peg", Type: "float64"},
		},
	},
	"market_mainline_cls": &TableSchema{
		Name:        "market_mainline_cls",
		Description: "Market mainline overview (CLS)",
		Layer:       "core",
		WriteMode:   "snapshot",
		Columns: []ColumnSchema{
			{Name: "block_key", Type: "string"},
			{Name: "title", Type: "string"},
			{Name: "summary", Type: "string"},
		},
	},
	"stock_sector_belong_eastmoney": &TableSchema{
		Name:        "stock_sector_belong_eastmoney",
		Description: "Stock sector belonging (Eastmoney)",
		Layer:       "core",
		WriteMode:   "upsert_by_key",
		PrimaryKeys: []string{"instrument_id"},
		Columns: []ColumnSchema{
			{Name: "instrument_id", Type: "string", IsPK: true},
			{Name: "symbol", Type: "string"},
			{Name: "exchange", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "sector_code", Type: "string"},
			{Name: "sector_name", Type: "string"},
		},
	},
}

// TableRegistryNames returns all registered table names.
func TableRegistryNames() []string {
	var names []string
	for name := range TableRegistry {
		names = append(names, name)
	}
	return names
}

// GetSchema returns the schema for a table name.
func GetSchema(name string) *TableSchema {
	return TableRegistry[name]
}
