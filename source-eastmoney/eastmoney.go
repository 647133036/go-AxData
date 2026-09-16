package eastmoney

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// EastMoneyAdapter implements the EastMoney (东方财富) API adapter.
// Interface names and field mappings match upstream Python axdata_core adapter.
type EastMoneyAdapter struct {
	baseURL     string
	client      *http.Client
	description string
}

// API URLs matching upstream Python constants.
const (
	EASTMONEY_CLIST_URL         = "https://push2delay.eastmoney.com/api/qt/clist/get"
	EASTMONEY_ULIST_URL         = "https://push2delay.eastmoney.com/api/qt/ulist.np/get"
	EASTMONEY_DATA_CENTER_URL   = "https://datacenter-web.eastmoney.com/api/data/v1/get"
	EASTMONEY_REPORT_LIST_URL   = "https://reportapi.eastmoney.com/report/list"
	EASTMONEY_ZT_POOL_URL       = "https://push2ex.eastmoney.com/getTopicZTPool"
	EASTMONEY_DT_POOL_URL       = "https://push2ex.eastmoney.com/getTopicDTPool"
	EASTMONEY_YESTERDAY_ZT_URL  = "https://push2ex.eastmoney.com/getYesterdayZTPool"
	EASTMONEY_STOCK_CHANGES_URL = "https://push2ex.eastmoney.com/getAllStockChanges"
	EASTMONEY_STOCK_CHANGE_URL  = "https://push2ex.eastmoney.com/getStockChanges"
)

var (
	defaultIndexNames = []string{
		"上证指数", "深证成指", "创业板指", "科创50", "沪深300", "中证500",
	}
	sectorTypeFS = map[string]string{
		"industry":    "m:90+t:2+f:!50",
		"concept":     "m:90+t:3+f:!50",
		"industry_l1": "m:90+s:2+f:!50",
		"industry_l2": "m:90+s:4+f:!50",
		"industry_l3": "m:90+s:8+f:!50",
	}
	changeTypeNames = map[string]string{
		"8201": "火箭发射", "8202": "快速反弹", "8203": "加速下跌",
		"8204": "高台跳水", "8193": "大笔买入", "8194": "大笔卖出",
		"8205": "封涨停板", "8206": "封跌停板", "8207": "打开跌停板",
		"8208": "打开涨停板", "64": "有大买盘", "128": "有大卖盘",
		"8209": "竞价上涨", "8210": "竞价下跌", "8211": "高开5日线",
		"8212": "低开5日线", "8213": "向上缺口", "8214": "向下缺口",
		"8215": "60日新高", "8216": "60日新低", "8217": "60日大幅上涨",
		"8218": "60日大幅下跌",
	}
	supportedInterfaces = map[string]bool{
		"eastmoney_market_index_realtime":   true,
		"eastmoney_market_index_all_em":     true,
		"eastmoney_stocks_all_em":           true,
		"eastmoney_stock_realtime_snapshot": true,
		"eastmoney_sector_realtime":         true,
		"eastmoney_sector_constituents":     true,
		"eastmoney_stock_sector_belong":     true,
		"eastmoney_limit_up_pool":           true,
		"eastmoney_limit_down_pool":         true,
		"eastmoney_yesterday_limit_up_pool": true,
		"eastmoney_stock_changes":           true,
		"eastmoney_stock_change_detail":     true,
		"eastmoney_dragon_tiger_daily":      true,
		"eastmoney_margin_trading":          true,
		"eastmoney_research_reports":        true,
		"eastmoney_is_trade_day":            true,
		"eastmoney_trade_days":              true,
		"eastmoney_financial_income":        true,
		"eastmoney_financial_balance":       true,
		"eastmoney_financial_cashflow":      true,
		"eastmoney_business_scope":          true,
		"eastmoney_earnings_forecast":       true,
		"eastmoney_valuation_snapshot":      true,
		"is_trade_day":                      true,
		"get_trade_days":                    true,
	}
)

// NewEastMoneyAdapter creates a new EastMoney adapter.
func NewEastMoneyAdapter() *EastMoneyAdapter {
	return &EastMoneyAdapter{
		baseURL:     "https://quote.eastmoney.com",
		client:      &http.Client{Timeout: 30 * time.Second},
		description: "East Money API adapter",
	}
}

// Name returns the adapter name.
func (a *EastMoneyAdapter) Name() string {
	return "eastmoney"
}

// Description returns adapter info.
func (a *EastMoneyAdapter) Description() string {
	return a.description
}

// Request fetches data from EastMoney by interface name.
func (a *EastMoneyAdapter) Request(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	interfaceName, _ := params["interface"].(string)

	if !supportedInterfaces[interfaceName] {
		return nil, fmt.Errorf("unknown interface: %s", interfaceName)
	}

	switch interfaceName {
	case "eastmoney_market_index_realtime":
		return a.requestMarketIndexRealtime(ctx, params)
	case "eastmoney_market_index_all_em":
		return a.requestMarketIndexAll(ctx, params)
	case "eastmoney_stocks_all_em":
		return a.requestStocksAllEm(ctx, params)
	case "eastmoney_stock_realtime_snapshot":
		return a.requestStockRealtimeSnapshot(ctx, params)
	case "eastmoney_sector_realtime":
		return a.requestSectorRealtime(ctx, params)
	case "eastmoney_sector_constituents":
		return a.requestSectorConstituents(ctx, params)
	case "eastmoney_stock_sector_belong":
		return a.requestStockSectorBelong(ctx, params)
	case "eastmoney_limit_up_pool":
		return a.requestLimitPool(ctx, params, "up")
	case "eastmoney_limit_down_pool":
		return a.requestLimitPool(ctx, params, "down")
	case "eastmoney_yesterday_limit_up_pool":
		return a.requestYesterdayLimitUpPool(ctx, params)
	case "eastmoney_stock_changes":
		return a.requestStockChanges(ctx, params)
	case "eastmoney_stock_change_detail":
		return a.requestStockChangeDetail(ctx, params)
	case "eastmoney_dragon_tiger_daily":
		return a.requestDragonTigerDaily(ctx, params)
	case "eastmoney_margin_trading":
		return a.requestMarginTrading(ctx, params)
	case "eastmoney_research_reports":
		return a.requestResearchReports(ctx, params)
	case "eastmoney_financial_income":
		return a.requestFinancialReport(ctx, params, "income")
	case "eastmoney_financial_balance":
		return a.requestFinancialReport(ctx, params, "balance")
	case "eastmoney_financial_cashflow":
		return a.requestFinancialReport(ctx, params, "cashflow")
	case "eastmoney_business_scope":
		return a.requestBusinessScope(ctx, params)
	case "eastmoney_earnings_forecast":
		return a.requestEarningsForecast(ctx, params)
	case "eastmoney_valuation_snapshot":
		return a.requestValuationSnapshot(ctx, params)
	case "eastmoney_is_trade_day", "is_trade_day":
		return a.requestIsTradeDay(ctx, params)
	case "eastmoney_trade_days", "get_trade_days":
		return a.requestTradeDays(ctx, params)
	default:
		return nil, fmt.Errorf("unknown interface: %s", interfaceName)
	}
}

// getParamString returns a string parameter or default value.
func getParamString(params map[string]interface{}, key string, defaultVal string) string {
	if v, ok := params[key].(string); ok && v != "" {
		return v
	}
	return defaultVal
}

// getParamInt returns an int parameter or default value.
func getParamInt(params map[string]interface{}, key string, defaultVal int) int {
	if v, ok := params[key].(int); ok && v > 0 {
		return v
	}
	if v, ok := params[key].(float64); ok && v > 0 {
		return int(v)
	}
	if v, ok := params[key].(string); ok {
		i, _ := strconv.Atoi(v)
		if i > 0 {
			return i
		}
	}
	return defaultVal
}

// parseCodeList parses codes from a comma/Chinese-comma/space-separated string.
func parseCodeList(raw string) []string {
	re := codeListSepRe
	parts := re.Split(raw, -1)
	var codes []string
	for _, p := range parts {
		c := strings.ToUpper(strings.TrimSpace(p))
		if c == "" {
			continue
		}
		if strings.HasSuffix(c, ".SH") {
			c = strings.TrimSuffix(c, ".SH")
		} else if strings.HasSuffix(c, ".SZ") {
			c = strings.TrimSuffix(c, ".SZ")
		} else if strings.HasSuffix(c, ".BJ") {
			c = strings.TrimSuffix(c, ".BJ")
		}
		// Strip leading SH/SZ/BJ prefix
		if len(c) >= 8 && (strings.HasPrefix(c, "SH") || strings.HasPrefix(c, "SZ") || strings.HasPrefix(c, "BJ")) {
			c = c[2:]
		}
		if len(c) == 6 && sixDigitRe.MatchString(c) {
			codes = append(codes, c)
		}
	}
	return codes
}

// secidFromCode converts a 6-digit code to EastMoney secid format.
func secidFromCode(symbol string) string {
	market := "0"
	if strings.HasPrefix(symbol, "6") || strings.HasPrefix(symbol, "5") || strings.HasPrefix(symbol, "9") {
		market = "1"
	}
	return market + "." + symbol
}

// exchangeFromSymbol determines exchange from symbol prefix.
func exchangeFromSymbol(symbol string) string {
	if strings.HasPrefix(symbol, "6") || strings.HasPrefix(symbol, "5") || strings.HasPrefix(symbol, "9") {
		return "SSE"
	}
	if strings.HasPrefix(symbol, "4") || strings.HasPrefix(symbol, "8") || strings.HasPrefix(symbol, "92") {
		return "BSE"
	}
	return "SZSE"
}

// exchangeSuffix returns the suffix for an exchange code.
func exchangeSuffix(exchange string) string {
	switch exchange {
	case "SSE":
		return "SH"
	case "SZSE":
		return "SZ"
	case "BSE":
		return "BJ"
	}
	return "SZ"
}

// instrumentID constructs "CODE.EXCHANGE_SUFFIX" from a 6-digit symbol.
func instrumentID(symbol string) string {
	exchange := exchangeFromSymbol(symbol)
	return symbol + "." + exchangeSuffix(exchange)
}

// instrumentIDFromMarketCode constructs from symbol and market code (0/1).
func instrumentIDFromMarketCode(symbol, marketCode string) string {
	switch marketCode {
	case "1":
		return symbol + ".SH"
	default:
		return symbol + "." + exchangeSuffix(exchangeFromSymbol(symbol))
	}
}

// splitInstrumentID splits "CODE.SH" into symbol and exchange.
func splitInstrumentID(id string) (symbol, exchange string) {
	parts := strings.Split(id, ".")
	if len(parts) != 2 {
		return "", ""
	}
	sym := parts[0]
	suf := parts[1]
	switch suf {
	case "SH":
		return sym, "SSE"
	case "SZ":
		return sym, "SZSE"
	case "BJ":
		return sym, "BSE"
	}
	return sym, ""
}

// httpGet performs an HTTP GET request and returns response bytes.
func (a *EastMoneyAdapter) httpGet(ctx context.Context, targetURL string, referer string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 AxData/0.1")
	req.Header.Set("Accept", "application/json,text/plain,*/*")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// buildQueryURL builds a URL with query parameters.
func buildQueryURL(baseURL string, params map[string]string) string {
	q := url.Values{}
	for k, v := range params {
		if v != "" {
			q.Set(k, v)
		}
	}
	return baseURL + "?" + q.Encode()
}

// payloadDiffRows extracts rows from push2delay response format {data: {diff: [...], ...}}.
func payloadDiffRows(data []byte) ([]map[string]interface{}, error) {
	var raw struct {
		Rc   interface{} `json:"rc"`
		Data struct {
			Diff []map[string]interface{} `json:"diff"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if raw.Data.Diff == nil {
		return []map[string]interface{}{}, nil
	}
	return raw.Data.Diff, nil
}

// parsePoolRows extracts rows from push2ex pool response {data: {pool: [...]}}.
func parsePoolRows(data []byte) ([]map[string]interface{}, error) {
	var raw struct {
		Data struct {
			Pool []map[string]interface{} `json:"pool"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if raw.Data.Pool == nil {
		return []map[string]interface{}{}, nil
	}
	return raw.Data.Pool, nil
}

// parseStockChangesRows extracts from {data: {allstock: [...]}}.
func parseStockChangesRows(data []byte) ([]map[string]interface{}, error) {
	var raw struct {
		Data struct {
			AllStock []map[string]interface{} `json:"allstock"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if raw.Data.AllStock == nil {
		return []map[string]interface{}{}, nil
	}
	return raw.Data.AllStock, nil
}

// parseStockChangeDetailRows extracts from {data: {data: [...]}}.
func parseStockChangeDetailRows(data []byte) ([]map[string]interface{}, error) {
	var raw struct {
		Data struct {
			Data []map[string]interface{} `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if raw.Data.Data == nil {
		return []map[string]interface{}{}, nil
	}
	return raw.Data.Data, nil
}

// parseDataCenterRows extracts rows from datacenter-web response {result: {data: [...]}}.
func parseDataCenterRows(data []byte) ([]map[string]interface{}, error) {
	var raw struct {
		Success bool `json:"success"`
		Result  struct {
			Data []map[string]interface{} `json:"data"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if raw.Result.Data == nil {
		return []map[string]interface{}{}, nil
	}
	return raw.Result.Data, nil
}

// parseResearchReports extracts from {data: [...], hits: N}.
func parseResearchReports(data []byte) ([]map[string]interface{}, error) {
	var raw struct {
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if raw.Data == nil {
		return []map[string]interface{}{}, nil
	}
	return raw.Data, nil
}

// floatVal safely converts a value to float64.
func floatVal(v interface{}) float64 {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case string:
		f, _ := strconv.ParseFloat(val, 64)
		return f
	}
	return 0
}

// dateFromValue extracts YYYYMMDD date from a value.
func dateFromValue(v interface{}) string {
	s := fmt.Sprintf("%v", v)
	digits := digitAllRe.FindAllString(s, -1)
	if len(digits) > 0 {
		s = strings.Join(digits, "")
	}
	re := date8Re
	m := re.FindString(s)
	if m != "" {
		return m[:8]
	}
	return ""
}

// cleanText strips HTML tags and normalizes whitespace.
func cleanText(v interface{}) string {
	if v == nil {
		return ""
	}
	s := fmt.Sprintf("%v", v)
	// Remove HTML tags
	s = htmlTagRe.ReplaceAllString(s, "")
	// Normalize whitespace
	s = wsRunRe.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	return s
}

// formatTime converts time value to HHMMSS string.
func formatTime(v interface{}) string {
	s := cleanText(v)
	digits := digitAllRe.FindAllString(s, -1)
	if len(digits) == 0 {
		return ""
	}
	s = strings.Join(digits, "")
	if len(s) <= 6 {
		s = strings.Repeat("0", 6-len(s)) + s
	} else {
		s = s[len(s)-6:]
	}
	return s
}

// todayYYYYMMDD returns current date as YYYYMMDD.
func todayYYYYMMDD() string {
	return time.Now().Format("20060102")
}

// dateToDash converts YYYYMMDD to YYYY-MM-DD.
func dateToDash(d string) string {
	if len(d) != 8 {
		return d
	}
	return d[:4] + "-" + d[4:6] + "-" + d[6:8]
}

// parseBool parses a boolean parameter.
func parseBool(params map[string]interface{}, key string, defaultVal bool) bool {
	if v, ok := params[key].(bool); ok {
		return v
	}
	if v, ok := params[key].(string); ok {
		lower := strings.ToLower(v)
		if lower == "true" || lower == "1" {
			return true
		}
		if lower == "false" || lower == "0" {
			return false
		}
	}
	return defaultVal
}

// requestMarketIndexAll fetches all market indices.
func (a *EastMoneyAdapter) requestMarketIndexAll(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	return a.requestMarketIndexRealtime(ctx, map[string]interface{}{"scope": "all"})
}

// requestStocksAllEm fetches all A-share realtime data.
func (a *EastMoneyAdapter) requestStocksAllEm(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	filterST := parseBool(params, "filter_st", true)
	page := getParamInt(params, "page", 1)
	limit := getParamInt(params, "limit", 50)
	if limit > 500 {
		limit = 500
	}

	paramsMap := map[string]string{
		"pn":     strconv.Itoa(page),
		"pz":     strconv.Itoa(limit),
		"po":     "1",
		"np":     "1",
		"ut":     "bd1d9ddb04089700cf9c27f6f7426281",
		"fltt":   "2",
		"invt":   "2",
		"fid":    "f3",
		"fs":     "m:0+t:6,m:0+t:80,m:1+t:2,m:1+t:23,m:0+t:81+s:2048",
		"fields": "f2,f3,f4,f5,f6,f7,f8,f9,f10,f12,f14,f15,f16,f17,f18,f20,f21,f23",
	}

	data, err := a.httpGet(ctx, buildQueryURL(EASTMONEY_CLIST_URL, paramsMap), "https://quote.eastmoney.com/")
	if err != nil {
		return nil, fmt.Errorf("stocks all em request: %w", err)
	}

	rows, err := payloadDiffRows(data)
	if err != nil {
		return nil, err
	}

	var normalized []map[string]interface{}
	for _, row := range rows {
		symbol := cleanText(row["f12"])
		if symbol == "" {
			continue
		}
		name := cleanText(row["f14"])
		lp := floatVal(row["f2"])
		if filterST && (strings.HasPrefix(name, "ST") || strings.HasPrefix(name, "*ST") || (lp != 0 && lp <= 1.0)) {
			continue
		}
		normalized = append(normalized, map[string]interface{}{
			"instrument_id": instrumentID(symbol),
			"symbol":        symbol,
			"exchange":      exchangeFromSymbol(symbol),
			"name":          name,
			"last_price":    floatVal(row["f2"]),
			"change_pct":    floatVal(row["f3"]),
			"change":        floatVal(row["f4"]),
			"volume":        floatVal(row["f5"]),
			"amount":        floatVal(row["f6"]),
			"turnover_rate": floatVal(row["f8"]),
			"pe_ttm":        floatVal(row["f9"]),
			"volume_ratio":  floatVal(row["f10"]),
			"high":          floatVal(row["f15"]),
			"low":           floatVal(row["f16"]),
			"open":          floatVal(row["f17"]),
			"pre_close":     floatVal(row["f18"]),
			"total_market":  floatVal(row["f20"]),
			"circ_market":   floatVal(row["f21"]),
			"pb":            floatVal(row["f23"]),
		})
	}

	return normalized, nil
}

// requestIsTradeDay uses levizhang.com trade calendar API.
func (a *EastMoneyAdapter) requestIsTradeDay(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.levizhang.com/isTradeDay", nil)
	if err != nil {
		return nil, fmt.Errorf("is_trade_day request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 AxData/0.1")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("is_trade_day request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("is_trade_day read: %w", err)
	}

	var raw struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("is_trade_day unmarshal: %w", err)
	}
	isTrade := raw.Code == "000000"

	return []map[string]interface{}{
		{"trade_date": todayYYYYMMDD(), "is_trade_day": isTrade},
	}, nil
}

// requestTradeDays uses levizhang.com trade calendar API.
func (a *EastMoneyAdapter) requestTradeDays(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	n := getParamInt(params, "n", 10)
	if n < 1 || n > 30 {
		n = 10
	}

	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("https://api.levizhang.com/getTradeDays?n=%d", n), nil)
	if err != nil {
		return nil, fmt.Errorf("trade_days request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 AxData/0.1")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("trade_days request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("trade_days read: %w", err)
	}

	var raw struct {
		Code string   `json:"code"`
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("trade_days unmarshal: %w", err)
	}
	if raw.Code != "000000" {
		return nil, fmt.Errorf("trade_days error: %v", raw.Data)
	}

	var tradeDays []map[string]interface{}
	for _, td := range raw.Data {
		if len(td) == 8 {
			tradeDays = append(tradeDays, map[string]interface{}{
				"trade_date": td,
			})
		}
	}

	return tradeDays, nil
}

func (a *EastMoneyAdapter) requestMarketIndexRealtime(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	scope := getParamString(params, "scope", "default")
	if scope != "default" && scope != "all" {
		return nil, fmt.Errorf("scope must be default or all")
	}
	limit := getParamInt(params, "limit", 100)
	if limit > 500 {
		limit = 500
	}

	paramsMap := map[string]string{
		"np":     "1",
		"fltt":   "1",
		"invt":   "2",
		"fs":     "b:MK0010",
		"fields": "f12,f14,f2,f3,f4,f5,f6,f15,f16,f17,f18",
		"pn":     "1",
		"pz":     strconv.Itoa(limit),
		"po":     "1",
		"ut":     "fa5fd1943c7b386f172d6893dbfba10b",
		"dect":   "1",
	}

	data, err := a.httpGet(ctx, buildQueryURL(EASTMONEY_CLIST_URL, paramsMap), "https://quote.eastmoney.com/")
	if err != nil {
		return nil, fmt.Errorf("market index realtime request: %w", err)
	}

	rows, err := payloadDiffRows(data)
	if err != nil {
		return nil, err
	}

	var normalized []map[string]interface{}
	for _, row := range rows {
		normalized = append(normalized, map[string]interface{}{
			"index_code": cleanText(row["f12"]),
			"index_name": cleanText(row["f14"]),
			"last_price": floatVal(row["f2"]) / 100,
			"change_pct": floatVal(row["f3"]) / 100,
			"change":     floatVal(row["f4"]) / 100,
			"volume":     floatVal(row["f5"]),
			"amount":     floatVal(row["f6"]),
			"high":       floatVal(row["f15"]) / 100,
			"low":        floatVal(row["f16"]) / 100,
			"open":       floatVal(row["f17"]) / 100,
			"pre_close":  floatVal(row["f18"]) / 100,
		})
	}

	if scope == "default" {
		byName := map[string]map[string]interface{}{}
		for _, r := range normalized {
			if n, ok := r["index_name"].(string); ok {
				byName[n] = r
			}
		}
		var filtered []map[string]interface{}
		for _, name := range defaultIndexNames {
			if r, ok := byName[name]; ok {
				filtered = append(filtered, r)
			}
		}
		normalized = filtered
	}

	return normalized, nil
}

// requestStockRealtimeSnapshot fetches stock realtime snapshot.
func (a *EastMoneyAdapter) requestStockRealtimeSnapshot(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	filterST := parseBool(params, "filter_st", true)

	codesRaw := getParamString(params, "code", "")
	var rows []map[string]interface{}

	if codesRaw != "" {
		codes := parseCodeList(codesRaw)
		if len(codes) > 100 {
			codes = codes[:100]
		}
		secids := []string{}
		for _, c := range codes {
			secids = append(secids, secidFromCode(c))
		}
		paramsMap := map[string]string{
			"fields": "f2,f3,f4,f5,f6,f7,f8,f9,f10,f12,f14,f15,f16,f17,f18,f20,f21,f23",
			"fltt":   "2",
			"invt":   "2",
			"ut":     "fa5fd1943c7b386f172d6893dbfba10b",
			"secids": strings.Join(secids, ","),
		}
		data, err := a.httpGet(ctx, buildQueryURL(EASTMONEY_ULIST_URL, paramsMap), "https://quote.eastmoney.com/")
		if err != nil {
			return nil, fmt.Errorf("stock realtime snapshot (by code) request: %w", err)
		}
		rows, err = payloadDiffRows(data)
		if err != nil {
			return nil, err
		}
	} else {
		page := getParamInt(params, "page", 1)
		limit := getParamInt(params, "limit", 50)
		if limit > 200 {
			limit = 200
		}
		paramsMap := map[string]string{
			"pn":     strconv.Itoa(page),
			"pz":     strconv.Itoa(limit),
			"po":     "1",
			"np":     "1",
			"ut":     "bd1d9ddb04089700cf9c27f6f7426281",
			"fltt":   "2",
			"invt":   "2",
			"fid":    "f3",
			"fs":     "m:0+t:6,m:0+t:80,m:1+t:2,m:1+t:23,m:0+t:81+s:2048",
			"fields": "f2,f3,f4,f5,f6,f7,f8,f9,f10,f12,f14,f15,f16,f17,f18,f20,f21,f23",
		}
		data, err := a.httpGet(ctx, buildQueryURL(EASTMONEY_CLIST_URL, paramsMap), "https://quote.eastmoney.com/")
		if err != nil {
			return nil, fmt.Errorf("stock realtime snapshot (clist) request: %w", err)
		}
		rows, err = payloadDiffRows(data)
		if err != nil {
			return nil, err
		}
	}

	var normalized []map[string]interface{}
	for _, row := range rows {
		symbol := cleanText(row["f12"])
		if symbol == "" {
			continue
		}
		exchange := exchangeFromSymbol(symbol)
		iid := instrumentID(symbol)
		record := map[string]interface{}{
			"instrument_id":      iid,
			"symbol":             symbol,
			"exchange":           exchange,
			"name":               cleanText(row["f14"]),
			"last_price":         floatVal(row["f2"]),
			"change_pct":         floatVal(row["f3"]),
			"change":             floatVal(row["f4"]),
			"volume":             floatVal(row["f5"]),
			"amount":             floatVal(row["f6"]),
			"amplitude":          floatVal(row["f7"]),
			"turnover_rate":      floatVal(row["f8"]),
			"pe_ttm":             floatVal(row["f9"]),
			"volume_ratio":       floatVal(row["f10"]),
			"high":               floatVal(row["f15"]),
			"low":                floatVal(row["f16"]),
			"open":               floatVal(row["f17"]),
			"pre_close":          floatVal(row["f18"]),
			"total_market_value": floatVal(row["f20"]),
			"float_market_value": floatVal(row["f21"]),
			"pb":                 floatVal(row["f23"]),
		}
		if filterST {
			name := cleanText(row["f14"])
			if strings.HasPrefix(name, "ST") || strings.HasPrefix(name, "*ST") {
				continue
			}
			lp := floatVal(row["f2"])
			if lp != 0 && lp <= 1.0 {
				continue
			}
		}
		normalized = append(normalized, record)
	}

	return normalized, nil
}

// requestSectorRealtime fetches sector realtime data.
func (a *EastMoneyAdapter) requestSectorRealtime(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	sectorType := strings.ToLower(getParamString(params, "sector_type", "industry"))
	fs, ok := sectorTypeFS[sectorType]
	if !ok {
		return nil, fmt.Errorf("sector_type must be one of industry, concept, industry_l1, industry_l2, industry_l3")
	}
	page := getParamInt(params, "page", 1)
	limit := getParamInt(params, "limit", 100)
	if limit > 200 {
		limit = 200
	}

	paramsMap := map[string]string{
		"pn":     strconv.Itoa(page),
		"pz":     strconv.Itoa(limit),
		"po":     "1",
		"np":     "1",
		"ut":     "bd1d9ddb04089700cf9c27f6f7426281",
		"fltt":   "2",
		"invt":   "2",
		"fid":    "f3",
		"fs":     fs,
		"fields": "f12,f14,f2,f3,f4,f5,f6,f7,f8,f20,f62,f128,f136,f140,f104,f105",
	}

	data, err := a.httpGet(ctx, buildQueryURL(EASTMONEY_CLIST_URL, paramsMap), "https://quote.eastmoney.com/")
	if err != nil {
		return nil, fmt.Errorf("sector realtime request: %w", err)
	}

	rows, err := payloadDiffRows(data)
	if err != nil {
		return nil, err
	}

	var normalized []map[string]interface{}
	for _, row := range rows {
		leadSymbol := cleanText(row["f140"])
		normalized = append(normalized, map[string]interface{}{
			"sector_code":           cleanText(row["f12"]),
			"sector_name":           cleanText(row["f14"]),
			"sector_type":           sectorType,
			"last_price":            floatVal(row["f2"]),
			"change_pct":            floatVal(row["f3"]),
			"change":                floatVal(row["f4"]),
			"volume":                floatVal(row["f5"]),
			"amount":                floatVal(row["f6"]),
			"amplitude":             floatVal(row["f7"]),
			"turnover_rate":         floatVal(row["f8"]),
			"total_market_value":    floatVal(row["f20"]),
			"main_inflow":           floatVal(row["f62"]),
			"lead_stock_name":       cleanText(row["f128"]),
			"lead_stock_symbol":     leadSymbol,
			"lead_stock_change_pct": floatVal(row["f136"]),
			"up_count":              int(floatVal(row["f104"])),
			"down_count":            int(floatVal(row["f105"])),
		})
	}

	return normalized, nil
}

// requestSectorConstituents fetches sector constituents.
func (a *EastMoneyAdapter) requestSectorConstituents(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	sectorCode := strings.ToUpper(getParamString(params, "sector_code", ""))
	if sectorCode == "" {
		return nil, fmt.Errorf("sector_code is required")
	}
	page := getParamInt(params, "page", 1)
	limit := getParamInt(params, "limit", 100)
	if limit > 200 {
		limit = 200
	}

	paramsMap := map[string]string{
		"pn":     strconv.Itoa(page),
		"pz":     strconv.Itoa(limit),
		"po":     "1",
		"np":     "1",
		"ut":     "bd1d9ddb04089700cf9c27f6f7426281",
		"fltt":   "2",
		"invt":   "2",
		"fid":    "f3",
		"fs":     "b:" + sectorCode + "+f:!50",
		"fields": "f2,f3,f4,f5,f6,f7,f8,f9,f10,f12,f14,f15,f16,f17,f18,f20,f21,f23",
	}

	data, err := a.httpGet(ctx, buildQueryURL(EASTMONEY_CLIST_URL, paramsMap), "https://quote.eastmoney.com/")
	if err != nil {
		return nil, fmt.Errorf("sector constituents request: %w", err)
	}

	rows, err := payloadDiffRows(data)
	if err != nil {
		return nil, err
	}

	var normalized []map[string]interface{}
	for _, row := range rows {
		symbol := cleanText(row["f12"])
		if symbol == "" {
			continue
		}
		exchange := exchangeFromSymbol(symbol)
		normalized = append(normalized, map[string]interface{}{
			"instrument_id":      instrumentID(symbol),
			"symbol":             symbol,
			"exchange":           exchange,
			"name":               cleanText(row["f14"]),
			"last_price":         floatVal(row["f2"]),
			"change_pct":         floatVal(row["f3"]),
			"change":             floatVal(row["f4"]),
			"volume":             floatVal(row["f5"]),
			"amount":             floatVal(row["f6"]),
			"amplitude":          floatVal(row["f7"]),
			"turnover_rate":      floatVal(row["f8"]),
			"pe_ttm":             floatVal(row["f9"]),
			"volume_ratio":       floatVal(row["f10"]),
			"high":               floatVal(row["f15"]),
			"low":                floatVal(row["f16"]),
			"open":               floatVal(row["f17"]),
			"pre_close":          floatVal(row["f18"]),
			"total_market_value": floatVal(row["f20"]),
			"float_market_value": floatVal(row["f21"]),
			"pb":                 floatVal(row["f23"]),
		})
	}

	return normalized, nil
}

// requestStockSectorBelong fetches sector belonging for stocks.
func (a *EastMoneyAdapter) requestStockSectorBelong(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	codesRaw := getParamString(params, "code", "")
	if codesRaw == "" {
		return nil, fmt.Errorf("code is required")
	}
	codes := parseCodeList(codesRaw)
	if len(codes) == 0 {
		return nil, fmt.Errorf("code is required")
	}
	if len(codes) > 100 {
		codes = codes[:100]
	}
	secids := []string{}
	for _, c := range codes {
		secids = append(secids, secidFromCode(c))
	}

	paramsMap := map[string]string{
		"fields": "f12,f14,f100",
		"invt":   "2",
		"ut":     "fa5fd1943c7b386f172d6893dbfba10b",
		"secids": strings.Join(secids, ","),
	}

	data, err := a.httpGet(ctx, buildQueryURL(EASTMONEY_ULIST_URL, paramsMap), "https://quote.eastmoney.com/")
	if err != nil {
		return nil, fmt.Errorf("stock sector belong request: %w", err)
	}

	rows, err := payloadDiffRows(data)
	if err != nil {
		return nil, err
	}

	var normalized []map[string]interface{}
	for _, row := range rows {
		symbol := cleanText(row["f12"])
		if symbol == "" {
			continue
		}
		exchange := exchangeFromSymbol(symbol)
		normalized = append(normalized, map[string]interface{}{
			"instrument_id": instrumentID(symbol),
			"symbol":        symbol,
			"exchange":      exchange,
			"name":          cleanText(row["f14"]),
			"sector_name":   cleanText(row["f100"]),
		})
	}

	return normalized, nil
}

// requestLimitPool fetches limit up/down pool.
func (a *EastMoneyAdapter) requestLimitPool(ctx context.Context, params map[string]interface{}, pool string) ([]map[string]interface{}, error) {
	tradeDate := getParamString(params, "trade_date", todayYYYYMMDD())
	if len(tradeDate) != 8 {
		return nil, fmt.Errorf("trade_date must be YYYYMMDD")
	}
	url := EASTMONEY_ZT_POOL_URL
	sort := "fbt:asc"
	if pool == "down" {
		url = EASTMONEY_DT_POOL_URL
		sort = "fund:asc"
	}

	paramsMap := map[string]string{
		"ut":        "7eea3edcaed734bea9cbfc24409ed989",
		"dpt":       "wz.ztzt",
		"Pageindex": "0",
		"pagesize":  "3000",
		"sort":      sort,
		"date":      tradeDate,
	}

	data, err := a.httpGet(ctx, buildQueryURL(url, paramsMap), "https://quote.eastmoney.com/")
	if err != nil {
		return nil, fmt.Errorf("limit %s pool request: %w", pool, err)
	}

	rows, err := parsePoolRows(data)
	if err != nil {
		return nil, err
	}

	var normalized []map[string]interface{}
	for _, row := range rows {
		symbol := cleanText(row["c"])
		marketCode := cleanText(row["m"])
		iid := instrumentIDFromMarketCode(symbol, marketCode)
		sym, exchange := splitInstrumentID(iid)
		// Get zttj sub-object
		zttjRaw, _ := row["zttj"]
		zttj := map[string]interface{}{}
		if z, ok := zttjRaw.(map[string]interface{}); ok {
			zttj = z
		}
		// limit price: ztp for up, dtp for down
		limitPrice := 0.0
		if pool == "up" {
			limitPrice = floatVal(row["ztp"]) / 1000
		} else {
			limitPrice = floatVal(row["dtp"]) / 1000
		}

		normalized = append(normalized, map[string]interface{}{
			"trade_date":         tradeDate,
			"instrument_id":      iid,
			"symbol":             sym,
			"exchange":           exchange,
			"name":               cleanText(row["n"]),
			"market_code":        marketCode,
			"last_price":         floatVal(row["p"]) / 1000,
			"limit_price":        limitPrice,
			"change_pct":         floatVal(row["zdp"]),
			"amount":             floatVal(row["amount"]),
			"float_market_value": floatVal(row["ltsz"]),
			"turnover_rate":      floatVal(row["hs"]),
			"first_limit_time":   formatTime(row["fbt"]),
			"last_limit_time":    formatTime(row["lbt"]),
			"continuous_count":   int(floatVal(row["lbc"])),
			"open_times":         int(floatVal(row["zbc"])),
			"main_inflow":        floatVal(row["fund"]),
			"sector":             cleanText(row["hybk"]),
			"zt_days":            int(floatVal(zttj["days"])),
			"zt_count":           int(floatVal(zttj["ct"])),
		})
	}

	return normalized, nil
}

// requestYesterdayLimitUpPool fetches yesterday limit-up pool.
func (a *EastMoneyAdapter) requestYesterdayLimitUpPool(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	tradeDate := getParamString(params, "trade_date", todayYYYYMMDD())
	if len(tradeDate) != 8 {
		return nil, fmt.Errorf("trade_date must be YYYYMMDD")
	}

	paramsMap := map[string]string{
		"ut":        "7eea3edcaed734bea9cbfc24409ed989",
		"dpt":       "wz.ztzt",
		"Pageindex": "0",
		"pagesize":  "3000",
		"sort":      "zs:desc",
		"date":      tradeDate,
	}

	data, err := a.httpGet(ctx, buildQueryURL(EASTMONEY_YESTERDAY_ZT_URL, paramsMap), "https://quote.eastmoney.com/")
	if err != nil {
		return nil, fmt.Errorf("yesterday limit-up pool request: %w", err)
	}

	rows, err := parsePoolRows(data)
	if err != nil {
		return nil, err
	}

	var normalized []map[string]interface{}
	for _, row := range rows {
		symbol := cleanText(row["c"])
		marketCode := cleanText(row["m"])
		iid := instrumentIDFromMarketCode(symbol, marketCode)
		sym, exchange := splitInstrumentID(iid)

		normalized = append(normalized, map[string]interface{}{
			"trade_date":                 tradeDate,
			"instrument_id":              iid,
			"symbol":                     sym,
			"exchange":                   exchange,
			"name":                       cleanText(row["n"]),
			"market_code":                marketCode,
			"last_price":                 floatVal(row["p"]) / 1000,
			"limit_price":                floatVal(row["ztp"]) / 1000,
			"change_pct":                 floatVal(row["zdp"]),
			"amount":                     floatVal(row["amount"]),
			"float_market_value":         floatVal(row["ltsz"]),
			"turnover_rate":              floatVal(row["hs"]),
			"first_limit_time":           formatTime(row["fbt"]),
			"last_limit_time":            formatTime(row["lbt"]),
			"continuous_count":           int(floatVal(row["lbc"])),
			"open_times":                 int(floatVal(row["zbc"])),
			"main_inflow":                floatVal(row["fund"]),
			"sector":                     cleanText(row["hybk"]),
			"amplitude":                  floatVal(row["zf"]),
			"open_ratio":                 floatVal(row["zs"]),
			"yesterday_limit_time":       formatTime(row["yfbt"]),
			"yesterday_continuous_count": int(floatVal(row["ylbc"])),
		})
	}

	return normalized, nil
}

// requestStockChanges fetches stock changes.
func (a *EastMoneyAdapter) requestStockChanges(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	changeType := getParamString(params, "change_type", "8201")
	_, ok := changeTypeNames[changeType]
	if !ok {
		return nil, fmt.Errorf("change_type is not supported")
	}
	filterST := parseBool(params, "filter_st", true)

	paramsMap := map[string]string{
		"type":      changeType,
		"ut":        "7eea3edcaed734bea9cbfc24409ed989",
		"pageindex": "0",
		"pagesize":  "10000",
		"dpt":       "wzchanges",
	}

	data, err := a.httpGet(ctx, buildQueryURL(EASTMONEY_STOCK_CHANGES_URL, paramsMap), "https://quote.eastmoney.com/")
	if err != nil {
		return nil, fmt.Errorf("stock changes request: %w", err)
	}

	rows, err := parseStockChangesRows(data)
	if err != nil {
		return nil, err
	}

	var normalized []map[string]interface{}
	for _, row := range rows {
		symbol := cleanText(row["c"])
		marketCode := cleanText(row["m"])
		iid := instrumentIDFromMarketCode(symbol, marketCode)
		sym, exchange := splitInstrumentID(iid)
		name := cleanText(row["n"])
		if filterST {
			if strings.HasPrefix(symbol, "4") {
				continue
			}
			if strings.HasPrefix(name, "ST") || strings.HasPrefix(name, "*ST") {
				continue
			}
		}
		normalized = append(normalized, map[string]interface{}{
			"instrument_id":    iid,
			"symbol":           sym,
			"exchange":         exchange,
			"name":             name,
			"market_code":      marketCode,
			"change_time":      formatTime(row["tm"]),
			"change_pct":       floatVal(row["i"]),
			"change_type":      changeType,
			"change_type_name": changeTypeNames[changeType],
		})
	}

	return normalized, nil
}

// requestStockChangeDetail fetches stock change detail.
func (a *EastMoneyAdapter) requestStockChangeDetail(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	codeRaw := getParamString(params, "code", "")
	if codeRaw == "" {
		return nil, fmt.Errorf("code is required")
	}
	codes := parseCodeList(codeRaw)
	if len(codes) == 0 {
		return nil, fmt.Errorf("code is required")
	}
	symbol := codes[0]
	tradeDate := getParamString(params, "trade_date", todayYYYYMMDD())
	if len(tradeDate) != 8 {
		return nil, fmt.Errorf("trade_date must be YYYYMMDD")
	}

	marketCode := getParamString(params, "market", "")
	if marketCode == "" {
		if exchangeFromSymbol(symbol) == "SSE" {
			marketCode = "1"
		} else {
			marketCode = "0"
		}
	}

	paramsMap := map[string]string{
		"ut":     "7eea3edcaed734bea9cbfc24409ed989",
		"date":   tradeDate,
		"dpt":    "wzchanges",
		"code":   symbol,
		"market": marketCode,
	}

	data, err := a.httpGet(ctx, buildQueryURL(EASTMONEY_STOCK_CHANGE_URL, paramsMap), "https://quote.eastmoney.com/")
	if err != nil {
		return nil, fmt.Errorf("stock change detail request: %w", err)
	}

	rows, err := parseStockChangeDetailRows(data)
	if err != nil {
		return nil, err
	}

	iid := instrumentID(symbol)
	_, exchange := splitInstrumentID(iid)

	var normalized []map[string]interface{}
	for _, row := range rows {
		changeType := cleanText(row["t"])
		normalized = append(normalized, map[string]interface{}{
			"trade_date":       tradeDate,
			"instrument_id":    iid,
			"symbol":           symbol,
			"exchange":         exchange,
			"name":             cleanText(row["n"]),
			"market_code":      marketCode,
			"change_time":      formatTime(row["tm"]),
			"change_type":      changeType,
			"change_type_name": changeTypeNames[changeType],
			"price":            floatVal(row["p"]) / 100,
			"change_pct":       floatVal(row["u"]),
			"volume":           floatVal(row["v"]),
		})
	}

	return normalized, nil
}

// requestDragonTigerDaily fetches dragon tiger daily data.
func (a *EastMoneyAdapter) requestDragonTigerDaily(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	tradeDate := getParamString(params, "trade_date", "")
	if tradeDate == "" {
		return nil, fmt.Errorf("trade_date is required")
	}
	if len(tradeDate) != 8 {
		return nil, fmt.Errorf("trade_date must be YYYYMMDD")
	}
	page := getParamInt(params, "page", 1)
	limit := getParamInt(params, "limit", 50)
	if limit > 200 {
		limit = 200
	}

	paramsMap := map[string]string{
		"sortColumns": "TRADE_DATE,SECURITY_CODE",
		"sortTypes":   "-1,1",
		"pageSize":    strconv.Itoa(limit),
		"pageNumber":  strconv.Itoa(page),
		"reportName":  "RPT_DAILYBILLBOARD_DETAILS",
		"columns":     "ALL",
		"source":      "WEB",
		"client":      "WEB",
		"filter":      "(TRADE_DATE='" + dateToDash(tradeDate) + "')",
	}

	data, err := a.httpGet(ctx, buildQueryURL(EASTMONEY_DATA_CENTER_URL, paramsMap), "https://data.eastmoney.com/stock/lhb.html")
	if err != nil {
		return nil, fmt.Errorf("dragon tiger daily request: %w", err)
	}

	rows, err := parseDataCenterRows(data)
	if err != nil {
		return nil, err
	}

	var normalized []map[string]interface{}
	for _, row := range rows {
		secucode := cleanText(row["SECUCODE"])
		securityCode := cleanText(row["SECURITY_CODE"])
		var iid string
		if strings.Contains(secucode, ".") {
			iid = secucode
		} else if securityCode != "" {
			iid = instrumentID(securityCode)
		}
		sym, exchange := splitInstrumentID(iid)
		normalized = append(normalized, map[string]interface{}{
			"trade_date":     dateFromValue(row["TRADE_DATE"]),
			"instrument_id":  iid,
			"symbol":         sym,
			"exchange":       exchange,
			"name":           cleanText(row["SECURITY_NAME_ABBR"]),
			"reason":         cleanText(row["EXPLANATION"]),
			"close_price":    floatVal(row["CLOSE_PRICE"]),
			"change_pct":     floatVal(row["CHANGE_RATE"]),
			"turnover_rate":  floatVal(row["TURNOVERRATE"]),
			"buy_amount":     floatVal(row["BILLBOARD_BUY_AMT"]),
			"sell_amount":    floatVal(row["BILLBOARD_SELL_AMT"]),
			"net_buy_amount": floatVal(row["BILLBOARD_NET_AMT"]),
			"total_amount":   floatVal(row["BILLBOARD_DEAL_AMT"]),
			"market":         cleanText(row["TRADE_MARKET"]),
		})
	}

	return normalized, nil
}

// requestMarginTrading fetches margin trading data.
func (a *EastMoneyAdapter) requestMarginTrading(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	codeRaw := getParamString(params, "code", "")
	if codeRaw == "" {
		return nil, fmt.Errorf("code is required")
	}
	codes := parseCodeList(codeRaw)
	if len(codes) == 0 {
		return nil, fmt.Errorf("code is required")
	}
	symbol := codes[0]
	startDate := getParamString(params, "start_date", "")
	endDate := getParamString(params, "end_date", "")
	if startDate != "" && endDate != "" && startDate > endDate {
		return nil, fmt.Errorf("start_date must be before or equal to end_date")
	}
	page := getParamInt(params, "page", 1)
	limit := getParamInt(params, "limit", 50)
	if limit > 200 {
		limit = 200
	}

	filterParts := []string{fmt.Sprintf(`(SCODE="%s")`, symbol)}
	if startDate != "" {
		filterParts = append(filterParts, "(DATE>='"+dateToDash(startDate)+"')")
	}
	if endDate != "" {
		filterParts = append(filterParts, "(DATE<='"+dateToDash(endDate)+"')")
	}

	paramsMap := map[string]string{
		"reportName":  "RPTA_WEB_RZRQ_GGMX",
		"columns":     "ALL",
		"source":      "WEB",
		"sortColumns": "date",
		"sortTypes":   "-1",
		"pageNumber":  strconv.Itoa(page),
		"pageSize":    strconv.Itoa(limit),
		"filter":      strings.Join(filterParts, ""),
	}

	data, err := a.httpGet(ctx, buildQueryURL(EASTMONEY_DATA_CENTER_URL, paramsMap),
		"https://data.eastmoney.com/rzrq/stock/"+symbol+".html")
	if err != nil {
		return nil, fmt.Errorf("margin trading request: %w", err)
	}

	rows, err := parseDataCenterRows(data)
	if err != nil {
		return nil, err
	}

	var normalized []map[string]interface{}
	for _, row := range rows {
		secucode := cleanText(row["SECUCODE"])
		scode := cleanText(row["SCODE"])
		var iid string
		if strings.Contains(secucode, ".") {
			iid = secucode
		} else if scode != "" {
			iid = instrumentID(scode)
		}
		sym, exchange := splitInstrumentID(iid)
		normalized = append(normalized, map[string]interface{}{
			"trade_date":            dateFromValue(row["DATE"]),
			"instrument_id":         iid,
			"symbol":                sym,
			"exchange":              exchange,
			"name":                  cleanText(row["SECNAME"]),
			"market":                cleanText(row["TRADE_MARKET"]),
			"close_price":           floatVal(row["SPJ"]),
			"change_pct":            floatVal(row["ZDF"]),
			"margin_balance":        floatVal(row["RZYE"]),
			"margin_buy_amount":     floatVal(row["RZMRE"]),
			"margin_repay_amount":   floatVal(row["RZCHE"]),
			"margin_net_buy_amount": floatVal(row["RZJME"]),
			"short_balance":         floatVal(row["RQYE"]),
			"short_sell_volume":     floatVal(row["RQMCL"]),
			"short_repay_volume":    floatVal(row["RQCHL"]),
			"short_net_sell_volume": floatVal(row["RQJMG"]),
			"total_balance":         floatVal(row["RZRQYE"]),
			"market_value":          floatVal(row["SZ"]),
		})
	}

	return normalized, nil
}

// requestResearchReports fetches research reports.
func (a *EastMoneyAdapter) requestResearchReports(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	codeRaw := getParamString(params, "code", "")
	if codeRaw == "" {
		return nil, fmt.Errorf("code is required")
	}
	codes := parseCodeList(codeRaw)
	if len(codes) == 0 {
		return nil, fmt.Errorf("code is required")
	}
	symbol := codes[0]
	startDate := getParamString(params, "start_date", "")
	endDate := getParamString(params, "end_date", "")
	if startDate != "" && endDate != "" && startDate > endDate {
		return nil, fmt.Errorf("start_date must be before or equal to end_date")
	}
	page := getParamInt(params, "page", 1)
	limit := getParamInt(params, "limit", 20)
	if limit > 100 {
		limit = 100
	}

	paramsMap := map[string]string{
		"pageNo":       strconv.Itoa(page),
		"pageSize":     strconv.Itoa(limit),
		"code":         symbol,
		"industryCode": "*",
		"industry":     "*",
		"rating":       "",
		"ratingChange": "",
		"beginTime":    dateToDash(startDate),
		"endTime":      dateToDash(endDate),
		"qType":        "0",
		"orgCode":      "",
		"rcode":        "",
	}

	data, err := a.httpGet(ctx, buildQueryURL(EASTMONEY_REPORT_LIST_URL, paramsMap),
		"https://data.eastmoney.com/report/stock.jshtml")
	if err != nil {
		return nil, fmt.Errorf("research reports request: %w", err)
	}

	rows, err := parseResearchReports(data)
	if err != nil {
		return nil, err
	}

	var normalized []map[string]interface{}
	for _, row := range rows {
		iid := instrumentID(symbol)
		_, exchange := splitInstrumentID(iid)
		ratingChange := cleanText(row["ratingChange"])
		normalized = append(normalized, map[string]interface{}{
			"report_id":              cleanText(row["infoCode"]),
			"instrument_id":          iid,
			"symbol":                 symbol,
			"exchange":               exchange,
			"name":                   cleanText(row["stockName"]),
			"title":                  cleanText(row["title"]),
			"publish_date":           dateFromValue(row["publishDate"]),
			"org_name":               cleanText(row["orgName"]),
			"rating":                 cleanText(row["emRatingName"]),
			"rating_change":          ratingChange,
			"researcher":             cleanText(row["researcher"]),
			"eps_forecast_this_year": floatVal(row["predictThisYearEps"]),
			"pe_forecast_this_year":  floatVal(row["predictThisYearPe"]),
			"file_size_kb":           floatVal(row["attachSize"]),
			"page_count":             int(floatVal(row["attachPages"])),
		})
	}

	return normalized, nil
}

var (
	digitAllRe    = regexp.MustCompile(`\d+`)
	date8Re       = regexp.MustCompile(`\d{4}\d{2}\d{2}`)
	htmlTagRe     = regexp.MustCompile(`<[^>]+>`)
	wsRunRe       = regexp.MustCompile(`\s+`)
	sixDigitRe    = regexp.MustCompile(`^\d{6}$`)
	codeListSepRe = regexp.MustCompile(`[,\s，]+`)
)
