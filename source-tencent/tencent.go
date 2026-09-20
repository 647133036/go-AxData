package tencent

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

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

const (
	TencentQuoteURL  = "https://qt.gtimg.cn/q=%s"
	TencentBoardRank = "https://proxy.finance.qq.com/cgi/cgi-bin/rank/hs/getBoardRankList"
	TencentKlineURL  = "https://proxy.finance.qq.com/ifzqgtimg/appstock/app/newfqkline/get"
	TencentTickURL   = "http://stock.gtimg.cn/data/index.php"
	TencentStartYear = "https://web.ifzq.gtimg.cn/other/klineweb/klineWeb/weekTrends"
)

type adapter struct {
	baseURL string
	client  *http.Client
}

func NewTencentAdapter() *adapter {
	return &adapter{
		baseURL: "https://qt.gtimg.cn",
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (a *adapter) Name() string {
	return "tencent"
}

func (a *adapter) Description() string {
	return "Tencent Finance API adapter"
}

var (
	quoteRe = regexp.MustCompile(`v_([a-z]{2}\d{6})="([^"]*)"`)
	tickRe  = regexp.MustCompile(`\[\s*\d+\s*,\s*"([^"]*)"\s*\]`)
)

func (a *adapter) Request(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	iface, _ := params["interface"].(string)
	if iface == "" {
		return nil, fmt.Errorf("interface parameter required")
	}

	switch iface {
	case "stock_zh_a_spot_tx":
		return a.requestSpot(ctx, params)
	case "stock_zh_a_hist_tx":
		return a.requestKline(ctx, params, "stock")
	case "stock_zh_a_tick_tx_js":
		return a.requestTick(ctx, params)
	case "stock_zh_index_daily_tx":
		return a.requestKline(ctx, params, "index")
	case "get_tx_start_year":
		return a.requestStartYear(ctx, params)
	case "tencent_realtime_snapshot":
		return a.requestSnapshot(ctx, params)
	default:
		return nil, fmt.Errorf("unknown interface: %s", iface)
	}
}

// requestSpot fetches sorted A-stock snapshot from board rank API.
func (a *adapter) requestSpot(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	sortType := paramStr(params, "sort_type", "price")
	direction := paramStr(params, "direction", "down")
	offset, _ := paramInt(params, "offset", 0)
	limit, _ := paramInt(params, "limit", 20)

	if limit > 200 {
		limit = 200
	}

	sortKey := map[string]string{
		"price":      "price",
		"change_pct": "zdf",
		"volume":     "volume",
		"amount":     "turnover",
	}[sortType]

	paramsMap := map[string]string{
		"_appver":    "11.17.0",
		"board_code": "aStock",
		"sort_type":  sortKey,
		"direct":     direction,
		"offset":     strconv.Itoa(offset),
		"count":      strconv.Itoa(limit),
	}
	u := TencentBoardRank + "?" + buildQuery(paramsMap)

	data, err := a.fetchJSON(ctx, u)
	if err != nil {
		return nil, err
	}

	payload, ok := data.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("board rank: unexpected JSON type")
	}
	// JSON numbers unmarshal to float64, so compare against 0.0; comparing
	// against the untyped int literal 0 always fails for a numeric code and
	// would report every successful response as an error.
	if code, ok := payload["code"]; ok && code != nil {
		if f, ok := code.(float64); !ok || f != 0 {
			return nil, fmt.Errorf("board rank error: %v", code)
		}
	}

	source := payload["data"]
	if source == nil {
		return []map[string]interface{}{}, nil
	}
	sourceMap, ok := source.(map[string]interface{})
	if !ok {
		return []map[string]interface{}{}, nil
	}
	rankList, ok := sourceMap["rank_list"].([]interface{})
	if !ok {
		return []map[string]interface{}{}, nil
	}

	var rows []map[string]interface{}
	for _, item := range rankList {
		row, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		result := normalizeBoardRankRow(row)
		if result != nil {
			rows = append(rows, result)
		}
	}
	return rows, nil
}

// requestKline fetches historical K-line data.
func (a *adapter) requestKline(ctx context.Context, params map[string]interface{}, assetType string) ([]map[string]interface{}, error) {
	symbol := paramStr(params, "code", "")
	if symbol == "" {
		return nil, fmt.Errorf("code parameter required")
	}
	quoteCode := cleanSymbol(symbol)

	startDate := paramStr(params, "start_date", "20240101")
	endDate := paramStr(params, "end_date", startDate)
	startDate = formatDate(startDate)
	endDate = formatDate(endDate)
	if startDate > endDate {
		return nil, fmt.Errorf("start_date must be before or equal to end_date")
	}

	adjust := paramStr(params, "adjust", "none")
	if assetType == "index" && adjust == "none" {
		adjust = "qfq"
	}
	limit, _ := paramInt(params, "limit", 120)
	if limit < 1 {
		limit = 120
	}
	if limit > 640 {
		limit = 640
	}

	startYear, _ := strconv.Atoi(startDate[:4])
	endYear, _ := strconv.Atoi(endDate[:4])
	var allRows []map[string]interface{}

	for year := startYear; year <= endYear; year++ {
		paramsMap := map[string]string{
			"_var":  fmt.Sprintf("kline_day%s%d", adjust, year),
			"param": fmt.Sprintf("%s,day,%d-01-01,%d-12-31,640,%s", quoteCode, year, year+1, adjust),
			"r":     "0.8205512681390605",
		}
		u := TencentKlineURL + "?" + buildQuery(paramsMap)

		data, err := a.fetchKline(ctx, u)
		if err != nil {
			continue
		}
		payload, ok := data.(map[string]interface{})
		if !ok {
			continue
		}
		if code := payload["code"]; code != nil {
			if f, ok := code.(float64); !ok || f != 0 {
				continue
			}
		}

		adjustKey := map[string]string{
			"none": "day",
			"qfq":  "qfqday",
			"hfq":  "hfqday",
		}[adjust]

		source := payload["data"]
		if source == nil {
			continue
		}
		sourceMap, ok := source.(map[string]interface{})
		if !ok {
			continue
		}
		qp, ok := sourceMap[quoteCode].(map[string]interface{})
		if !ok {
			continue
		}

		rawRows := qp[adjustKey]
		if rawRows == nil {
			rawRows = qp["day"]
		}
		if rawRows == nil {
			rawRows = qp["qfqday"]
		}
		if rawRows == nil {
			rawRows = qp["hfqday"]
		}

		arr, ok := rawRows.([]interface{})
		if !ok {
			continue
		}

		inner := normalizeKlineRows(arr, quoteCode, adjust, assetType)
		for _, r := range inner {
			td := r["trade_date"].(string)
			if td >= startDate && td <= endDate {
				allRows = append(allRows, r)
			}
		}
	}

	if len(allRows) > limit {
		allRows = allRows[:limit]
	}
	return allRows, nil
}

// requestTick fetches intraday tick data.
func (a *adapter) requestTick(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	symbol := paramStr(params, "code", "")
	if symbol == "" {
		return nil, fmt.Errorf("code parameter required")
	}
	quoteCode := cleanSymbol(symbol)
	page, _ := paramInt(params, "page", 0)

	paramsMap := map[string]string{
		"appn":   "detail",
		"action": "data",
		"c":      quoteCode,
		"p":      strconv.Itoa(page),
	}
	u := TencentTickURL + "?" + buildQuery(paramsMap)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 AxData/0.1")
	req.Header.Set("Referer", "https://gu.qq.com/")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tick HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	decoded, err := decodeGBK(body)
	if err != nil {
		decoded = body
	}

	match := tickRe.FindStringSubmatch(string(decoded))
	if match == nil {
		return []map[string]interface{}{}, nil
	}
	payload := match[1]

	exchange := exchangeFromQuoteCode(quoteCode)
	symbolPart := quoteCode[2:]
	instrumentID := fmt.Sprintf("%s.%s", symbolPart, exchangeSuffix(exchange))

	var rows []map[string]interface{}
	for _, item := range strings.Split(payload, "|") {
		parts := strings.Split(item, "/")
		if len(parts) < 7 {
			continue
		}
		seq := ""
		if n, e := strconv.Atoi(parts[0]); e == nil {
			seq = fmt.Sprintf("%d", n)
		}
		side := parts[6]
		upper := strings.ToUpper(strings.TrimSpace(side))
		switch upper {
		case "B":
			side = "buy"
		case "S":
			side = "sell"
		case "M":
			side = "neutral"
		}
		rows = append(rows, map[string]interface{}{
			"instrument_id": instrumentID,
			"symbol":        symbolPart,
			"exchange":      exchange,
			"sequence":      seq,
			"trade_time":    parts[1],
			"price":         parts[2],
			"change":        parts[3],
			"volume":        parts[4],
			"amount":        parts[5],
			"trade_side":    side,
		})
	}
	return rows, nil
}

// requestStartYear fetches the first trading year for a symbol.
func (a *adapter) requestStartYear(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	symbol := paramStr(params, "code", "")
	if symbol == "" {
		return nil, fmt.Errorf("code parameter required")
	}
	quoteCode := cleanSymbol(symbol)

	paramsMap := map[string]string{
		"_var":  "kline_dayqfq",
		"param": fmt.Sprintf("%s,day,,,320,qfq", quoteCode),
		"r":     "0.751892490072597",
	}
	u := TencentKlineURL + "?" + buildQuery(paramsMap)

	data, err := a.fetchKline(ctx, u)
	if err != nil {
		return nil, err
	}
	payload, ok := data.(map[string]interface{})
	if !ok {
		return []map[string]interface{}{}, nil
	}
	if code := payload["code"]; code != nil {
		if f, ok := code.(float64); !ok || f != 0 {
			return []map[string]interface{}{}, nil
		}
	}

	source := payload["data"]
	if source == nil {
		return []map[string]interface{}{}, nil
	}
	sourceMap, ok := source.(map[string]interface{})
	if !ok {
		return []map[string]interface{}{}, nil
	}
	qp, ok := sourceMap[quoteCode].(map[string]interface{})
	if !ok {
		return []map[string]interface{}{}, nil
	}

	rawRows := qp["qfqday"]
	if rawRows == nil {
		rawRows = qp["day"]
	}
	arr, ok := rawRows.([]interface{})
	if !ok || len(arr) == 0 {
		return []map[string]interface{}{}, nil
	}
	first, ok := arr[0].([]interface{})
	if !ok || len(first) < 2 {
		return []map[string]interface{}{}, nil
	}

	dateStr := dateFromDash(first[0])
	if dateStr == "" {
		return []map[string]interface{}{}, nil
	}
	exchange := exchangeFromQuoteCode(quoteCode)
	symbolPart := quoteCode[2:]

	return []map[string]interface{}{
		{
			"instrument_id": fmt.Sprintf("%s.%s", symbolPart, exchangeSuffix(exchange)),
			"symbol":        symbolPart,
			"exchange":      exchange,
			"asset_type":    assetTypeFromCode(quoteCode),
			"start_date":    dateStr,
			"source_value":  first[1],
		},
	}, nil
}

// requestSnapshot fetches real-time snapshot for given codes.
func (a *adapter) requestSnapshot(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	symbols := paramStr(params, "symbols", "")
	if symbols == "" {
		return nil, fmt.Errorf("symbols parameter required")
	}

	codes := formatSymbols(symbols)
	u := fmt.Sprintf(TencentQuoteURL, codes)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 AxData/0.1")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Referer", "https://gu.qq.com/")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("quote HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	decoded, err := decodeGBK(body)
	if err != nil {
		decoded = body
	}
	return parseQuotePayload(string(decoded)), nil
}

// fetchJSON fetches a JSON response.
func (a *adapter) fetchJSON(ctx context.Context, u string) (interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 AxData/0.1")
	req.Header.Set("Accept", "application/json,*/*")
	req.Header.Set("Referer", "https://gu.qq.com/")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, u)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var payload interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

// fetchKline fetches a K-line JSONP response and unwraps it.
func (a *adapter) fetchKline(ctx context.Context, u string) (interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 AxData/0.1")
	req.Header.Set("Accept", "application/json,text/javascript,*/*")
	req.Header.Set("Referer", "https://gu.qq.com/")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kline HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	decoded, err := decodeGBK(body)
	if err != nil {
		decoded = body
	}

	// Strip JSONP wrapper: "kline_dayqfq2024={...}" -> "{...}"
	idx := strings.Index(string(decoded), "={")
	if idx > 0 {
		decoded = decoded[idx+1:]
	}

	var payload interface{}
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

// formatSymbols formats AxData symbols into Tencent quote codes.
func formatSymbols(symbols string) string {
	var formatted []string
	for _, part := range strings.Split(symbols, ",") {
		code := cleanSymbol(part)
		if code != "" {
			formatted = append(formatted, code)
		}
	}
	return strings.Join(formatted, ",")
}

// cleanSymbol converts AxData symbol to Tencent quote code.
func cleanSymbol(symbol string) string {
	s := strings.ToLower(strings.TrimSpace(symbol))
	s = strings.ReplaceAll(s, ".sz", "")
	s = strings.ReplaceAll(s, ".sh", "")
	s = strings.ReplaceAll(s, ".bj", "")

	if strings.HasPrefix(s, "sh") || strings.HasPrefix(s, "sz") || strings.HasPrefix(s, "bj") {
		return s
	}

	if strings.HasPrefix(s, "92") {
		return "bj" + s
	}
	if strings.HasPrefix(s, "6") || strings.HasPrefix(s, "5") || strings.HasPrefix(s, "9") {
		return "sh" + s
	}
	if strings.HasPrefix(s, "4") || strings.HasPrefix(s, "8") {
		return "bj" + s
	}
	return "sz" + s
}

// formatDate normalizes a date string to YYYYMMDD.
func formatDate(v string) string {
	v = strings.ReplaceAll(v, "-", "")
	if len(v) != 8 {
		return "20240101"
	}
	_, err := time.Parse("20060102", v)
	if err != nil {
		return "20240101"
	}
	return v
}

// parseQuotePayload parses Tencent quote response using regex.
func parseQuotePayload(text string) []map[string]interface{} {
	var rows []map[string]interface{}
	for _, match := range quoteRe.FindAllStringSubmatch(text, -1) {
		if len(match) != 3 {
			continue
		}
		quoteCode := match[1]
		raw := match[2]
		if raw == "" {
			continue
		}
		parts := strings.Split(raw, "~")
		if len(parts) < 3 {
			continue
		}
		if quoteField(parts, 1) == "" || quoteField(parts, 2) == "" {
			continue
		}
		result := normalizeQuoteRow(quoteCode, parts)
		if result != nil {
			rows = append(rows, result)
		}
	}
	return rows
}

// normalizeQuoteRow maps ~-delimited fields to AxData schema.
//
// Tencent's quote payload is variable-width: a row can legitimately have as few
// as 3 fields or as many as 83. Every index above 2 is therefore read through
// quoteField, which returns "" past the end instead of panicking.
func normalizeQuoteRow(quoteCode string, parts []string) map[string]interface{} {
	symbol := quoteField(parts, 2)
	if symbol == "" {
		symbol = quoteCode[2:]
	}
	exchange := exchangeFromQuoteCode(quoteCode)
	instrumentID := fmt.Sprintf("%s.%s", symbol, exchangeSuffix(exchange))
	amount := amountFromCombined(quoteField(parts, 35))

	return map[string]interface{}{
		"instrument_id":      instrumentID,
		"symbol":             symbol,
		"exchange":           exchange,
		"asset_type":         assetType(quoteField(parts, 61), symbol),
		"name":               quoteField(parts, 1),
		"quote_time":         quoteField(parts, 30),
		"last_price":         quoteField(parts, 3),
		"pre_close":          quoteField(parts, 4),
		"open":               quoteField(parts, 5),
		"high":               quoteField(parts, 33),
		"low":                quoteField(parts, 34),
		"change":             quoteField(parts, 31),
		"change_pct":         quoteField(parts, 32),
		"volume":             quoteField(parts, 36),
		"amount":             amount,
		"turnover_rate":      quoteField(parts, 38),
		"pe_dynamic":         quoteField(parts, 39),
		"pb":                 quoteField(parts, 46),
		"total_market_value": quoteField(parts, 45),
		"float_market_value": quoteField(parts, 44),
		"limit_up_price":     quoteField(parts, 47),
		"limit_down_price":   quoteField(parts, 48),
		"currency":           quoteField(parts, 82),
	}
}

// quoteField reads one field of a quote row, returning "" when the row is
// shorter than the requested index.
func quoteField(parts []string, i int) string {
	if i < 0 || i >= len(parts) {
		return ""
	}
	return parts[i]
}

// normalizeBoardRankRow maps board rank fields to AxData schema.
func normalizeBoardRankRow(row map[string]interface{}) map[string]interface{} {
	code := fmt.Sprintf("%v", row["code"])
	code = strings.TrimSpace(strings.ToLower(code))
	if code == "" {
		return nil
	}
	symbol := code
	if len(code) > 2 && (strings.HasPrefix(code, "sh") || strings.HasPrefix(code, "sz") || strings.HasPrefix(code, "bj")) {
		symbol = code[2:]
	}
	if len(symbol) != 6 {
		return nil
	}

	exchange := exchangeFromQuoteCode(strings.ToLower(code))
	instrumentID := fmt.Sprintf("%s.%s", symbol, exchangeSuffix(exchange))

	return map[string]interface{}{
		"instrument_id":      instrumentID,
		"symbol":             symbol,
		"exchange":           exchange,
		"asset_type":         "stock",
		"name":               row["name"],
		"last_price":         row["pn"],
		"change":             row["zd"],
		"change_pct":         row["zdf"],
		"amplitude":          row["zf"],
		"volume":             row["volume"],
		"amount":             row["turnover"],
		"turnover_rate":      row["hsl"],
		"pe_ttm":             row["pe_ttm"],
		"total_market_value": row["zsz"],
		"float_market_value": row["ltsz"],
		"quote_state":        row["state"],
	}
}

// normalizeKlineRows maps K-line array rows to AxData schema.
func normalizeKlineRows(arr []interface{}, quoteCode, adjust, assetType string) []map[string]interface{} {
	exchange := exchangeFromQuoteCode(quoteCode)
	symbol := quoteCode[2:]
	instrumentID := fmt.Sprintf("%s.%s", symbol, exchangeSuffix(exchange))

	var rows []map[string]interface{}
	for _, raw := range arr {
		parts, ok := raw.([]interface{})
		if !ok {
			continue
		}
		// The row below reads up to parts[8]; skipping only below 6 let a
		// short row reach parts[8] and panic with index out of range.
		if len(parts) < 9 {
			continue
		}
		dateStr := dateFromDash(parts[0])
		if dateStr == "" {
			continue
		}
		rows = append(rows, map[string]interface{}{
			"instrument_id": instrumentID,
			"symbol":        symbol,
			"exchange":      exchange,
			"asset_type":    assetType,
			"trade_date":    dateStr,
			"adjust":        adjust,
			"open":          parts[1],
			"close":         parts[2],
			"high":          parts[3],
			"low":           parts[4],
			"volume":        parts[5],
			"amount":        parts[8],
		})
	}
	return rows
}

// amountFromCombined parses the combined amount field.
func amountFromCombined(v string) string {
	parts := strings.Split(v, "/")
	if len(parts) >= 3 {
		return parts[2]
	}
	return ""
}

// dateFromDash converts dash-formatted date to YYYYMMDD.
func dateFromDash(v interface{}) string {
	s := fmt.Sprintf("%v", v)
	s = strings.TrimSpace(s)
	if s == "" || s == "<nil>" {
		return ""
	}
	digits := strings.ReplaceAll(s, "-", "")
	if len(digits) == 8 {
		_, err := time.Parse("20060102", digits)
		if err == nil {
			return digits
		}
	}
	_, err := time.Parse("2006-01-02", s)
	if err != nil {
		return ""
	}
	return strings.ReplaceAll(s, "-", "")
}

// exchangeFromQuoteCode returns exchange name from quote code prefix.
func exchangeFromQuoteCode(code string) string {
	switch code[:2] {
	case "sh":
		return "SSE"
	case "bj":
		return "BSE"
	default:
		return "SZSE"
	}
}

// exchangeSuffix returns the exchange suffix for an exchange name.
func exchangeSuffix(exchange string) string {
	return map[string]string{
		"SSE":  "SH",
		"SZSE": "SZ",
		"BSE":  "BJ",
	}[exchange]
}

// assetType determines the asset type from type flag and symbol.
func assetType(flag, symbol string) string {
	upper := strings.ToUpper(flag)
	if strings.Contains(upper, "ETF") || strings.HasPrefix(symbol, "15") ||
		strings.HasPrefix(symbol, "16") || strings.HasPrefix(symbol, "50") ||
		strings.HasPrefix(symbol, "51") || strings.HasPrefix(symbol, "56") ||
		strings.HasPrefix(symbol, "58") {
		return "etf"
	}
	if strings.Contains(upper, "ZS") || (strings.HasPrefix(symbol, "000") && upper == "ZS") ||
		(strings.HasPrefix(symbol, "399") && upper == "ZS") {
		return "index"
	}
	return "stock"
}

// assetTypeFromCode determines asset type from quote code.
func assetTypeFromCode(quoteCode string) string {
	symbol := quoteCode[2:]
	if strings.HasPrefix(symbol, "15") || strings.HasPrefix(symbol, "16") ||
		strings.HasPrefix(symbol, "50") || strings.HasPrefix(symbol, "51") ||
		strings.HasPrefix(symbol, "56") || strings.HasPrefix(symbol, "58") {
		return "etf"
	}
	if strings.HasPrefix(quoteCode, "sh000") || strings.HasPrefix(quoteCode, "sz399") {
		return "index"
	}
	return "stock"
}

// decodeGBK decodes GBK-encoded bytes to UTF-8.
func decodeGBK(data []byte) ([]byte, error) {
	decoder := simplifiedchinese.GBK.NewDecoder()
	decoded, _, err := transform.Bytes(decoder, data)
	if err != nil {
		return nil, err
	}
	return decoded, nil
}

// buildQuery builds a URL-encoded query string.
func buildQuery(params map[string]string) string {
	values := url.Values{}
	for k, v := range params {
		values.Set(k, v)
	}
	return values.Encode()
}

// paramStr extracts a string parameter.
func paramStr(params map[string]interface{}, key, defaultVal string) string {
	if v, ok := params[key].(string); ok && v != "" {
		return v
	}
	return defaultVal
}

// paramInt extracts an integer parameter.
func paramInt(params map[string]interface{}, key string, defaultVal int) (int, bool) {
	switch v := params[key].(type) {
	case int:
		return v, true
	case float64:
		return int(v), true
	case string:
		n, err := strconv.Atoi(v)
		if err == nil && n != 0 {
			return n, true
		}
	}
	return defaultVal, false
}
