package sina

import (
	"context"
	"encoding/json"
	"fmt"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// hqStrRe matches the var hq_str_<code>="..." line the quote endpoint returns.
var hqStrRe = regexp.MustCompile(`var\s+hq_str_(\w+)="(.+)"`)

// SinaAdapter implements the Sina Finance (新浪财经) API.
type SinaAdapter struct {
	baseURL     string
	client      *http.Client
	description string
}

// NewSinaAdapter creates a new Sina adapter.
func NewSinaAdapter() *SinaAdapter {
	return &SinaAdapter{
		baseURL:     "https://hq.sinajs.cn",
		client:      &http.Client{Timeout: 30 * time.Second},
		description: "Sina Finance (新浪财经) API adapter",
	}
}

// Name returns the adapter name.
func (a *SinaAdapter) Name() string {
	return "sina"
}

// Description returns adapter info.
func (a *SinaAdapter) Description() string {
	return a.description
}

// Request fetches data from Sina.
func (a *SinaAdapter) Request(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	interfaceName, _ := params["interface"].(string)
	symbols, _ := params["symbols"].(string)

	if symbols == "" {
		return nil, fmt.Errorf("symbols parameter required")
	}

	switch interfaceName {
	case "real_time":
		return a.requestRealTime(ctx, symbols)
	case "kline":
		return a.requestKline(ctx, symbols, params)
	case "rank":
		return a.requestRank(ctx)
	case "history":
		return a.requestHistory(ctx, symbols, params)
	default:
		return nil, fmt.Errorf("unknown interface: %s", interfaceName)
	}
}

// requestRealTime fetches real-time quote data.
func (a *SinaAdapter) requestRealTime(ctx context.Context, symbols string) ([]map[string]interface{}, error) {
	symStr := sinaFormatSymbols(symbols)
	data, err := a.httpGet(ctx, a.baseURL+"/list="+symStr)
	if err != nil {
		return nil, fmt.Errorf("real-time request: %w", err)
	}
	decoded, err := decodeGBK(data)
	if err != nil {
		decoded = data
	}
	return a.parseRealTime(string(decoded))
}

// requestKline fetches K-line data.
func (a *SinaAdapter) requestKline(ctx context.Context, symbol string, params map[string]interface{}) ([]map[string]interface{}, error) {
	s := cleanSinaSymbol(symbol)
	scale := "240"
	if period, ok := params["period"].(string); ok {
		scale = periodToScale(period)
	}
	limit := 2048
	if l, ok := params["limit"].(int); ok {
		limit = l
	}

	// K-line uses the same CN_MarketData.getKLineData service as history,
	// with a smaller scale for intra-day periods.
	apiURL := "https://vip.stock.finance.sina.com.cn/quotes_service/api/json_v2.php/CN_MarketData.getKLineData"

	qs := url.Values{
		"symbol":  {s},
		"scale":   {scale},
		"ma":      {"no"},
		"datalen": {strconv.Itoa(limit)},
	}

	data, err := a.httpGet(ctx, apiURL+"?"+qs.Encode())
	if err != nil {
		return nil, fmt.Errorf("kline request: %w", err)
	}

	return a.parseHistoryJSON(data)
}

// requestRank fetches market index spot data.
func (a *SinaAdapter) requestRank(ctx context.Context) ([]map[string]interface{}, error) {
	apiURL := "https://vip.stock.finance.sina.com.cn/quotes_service/api/json_v2.php/Market_Center.getHQNodeDataSimple"
	qs := url.Values{
		"page": {"1"},
		"num":  {"80"},
		"sort": {"symbol"},
		"asc":  {"1"},
		"node": {"hs_s"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL+"?"+qs.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", "https://vip.stock.finance.sina.com.cn/mkt/")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	decoded, err := decodeGBK(data)
	if err != nil {
		decoded = data
	}

	var arr []map[string]interface{}
	if err := json.Unmarshal(decoded, &arr); err != nil {
		return nil, err
	}

	var results []map[string]interface{}
	for _, d := range arr {
		sym, _ := d["symbol"].(string)
		axCode := sinaToAxCode(sym)
		results = append(results, map[string]interface{}{
			"instrument_id": axCode,
			"name":          d["name"],
			"latest_price":  d["trade"],
			"change":        d["pricechange"],
			"change_pct":    d["changepercent"],
			"bid":           d["buy"],
			"ask":           d["sell"],
			"pre_close":     d["settlement"],
			"open":          d["open"],
			"high":          d["high"],
			"low":           d["low"],
			"volume":        d["volume"],
			"amount":        d["amount"],
			"tick_time":     d["ticktime"],
		})
	}
	return results, nil
}

// requestHistory fetches historical K-line data.
func (a *SinaAdapter) requestHistory(ctx context.Context, symbol string, params map[string]interface{}) ([]map[string]interface{}, error) {
	s := cleanSinaSymbol(symbol)
	scale := "240"
	if period, ok := params["period"].(string); ok {
		scale = periodToScale(period)
	}
	limit := 500
	if l, ok := params["limit"].(int); ok {
		limit = l
	}

	apiURL := fmt.Sprintf(
		"https://money.finance.sina.com.cn/quotes_service/api/json_v2.php/CN_MarketData.getKLineData?symbol=%s&scale=%s&ma=no&datalen=%d",
		s, scale, limit,
	)

	data, err := a.httpGet(ctx, apiURL)
	if err != nil {
		return nil, fmt.Errorf("history request: %w", err)
	}

	return a.parseHistoryJSON(data)
}

// parseRealTime parses Sina real-time quote text.
func (a *SinaAdapter) parseRealTime(raw string) ([]map[string]interface{}, error) {
	var results []map[string]interface{}

	matches := hqStrRe.FindAllStringSubmatch(raw, -1)

	for _, match := range matches {
		code := match[1]
		data := match[2]
		fields := strings.Split(data, ",")

		if len(fields) < 30 {
			continue
		}

		axCode := sinaToAxCode(code)

		record := map[string]interface{}{
			"instrument_id": axCode,
			"name":          fields[0],
			"open":          fields[1],
			"pre_close":     fields[2],
			"close":         fields[3],
			"high":          fields[4],
			"low":           fields[5],
			"bid":           fields[6],
			"ask":           fields[7],
			"volume":        fields[8],
			"amount":        fields[9],
			"bid_1_vol":     fields[10],
			"bid_1_price":   fields[11],
			"ask_1_vol":     fields[12],
			"ask_1_price":   fields[13],
			"bid_2_vol":     fields[14],
			"bid_2_price":   fields[15],
			"ask_2_vol":     fields[16],
			"ask_2_price":   fields[17],
			"bid_3_vol":     fields[18],
			"bid_3_price":   fields[19],
			"ask_3_vol":     fields[20],
			"ask_3_price":   fields[21],
			"bid_4_vol":     fields[22],
			"bid_4_price":   fields[23],
			"ask_4_vol":     fields[24],
			"ask_4_price":   fields[25],
			"bid_5_vol":     fields[26],
			"bid_5_price":   fields[27],
			"ask_5_vol":     fields[28],
			"ask_5_price":   fields[29],
			"trade_date":    fields[30],
			"trade_time":    fields[31],
		}
		results = append(results, record)
	}

	return results, nil
}

// parseHistoryJSON parses historical K-line JSON.
func (a *SinaAdapter) parseHistoryJSON(data []byte) ([]map[string]interface{}, error) {
	decoded, err := decodeGBK(data)
	if err != nil {
		decoded = data
	}

	var results []map[string]interface{}

	// Try plain array format first
	var arr []map[string]interface{}
	if err := json.Unmarshal(decoded, &arr); err == nil {
		for _, d := range arr {
			result := map[string]interface{}{
				"trade_date": d["day"],
				"open":       d["open"],
				"close":      d["close"],
				"high":       d["high"],
				"low":        d["low"],
				"volume":     d["volume"],
			}
			results = append(results, result)
		}
		return results, nil
	}

	// Fallback: Datas format
	var raw struct {
		Datas []map[string]interface{} `json:"Datas"`
	}
	if err := json.Unmarshal(decoded, &raw); err != nil {
		return nil, err
	}

	for _, d := range raw.Datas {
		result := map[string]interface{}{
			"trade_date": d["t"],
			"open":       d["o"],
			"close":      d["c"],
			"high":       d["h"],
			"low":        d["l"],
			"volume":     d["v"],
		}
		results = append(results, result)
	}
	return results, nil
}

// sinaFormatSymbols formats symbols for Sina API.
func sinaFormatSymbols(symbols string) string {
	parts := strings.Split(symbols, ",")
	formatted := []string{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		s := cleanSinaSymbol(p)
		if s != "" {
			formatted = append(formatted, s)
		}
	}
	return strings.Join(formatted, ",")
}

// decodeGBK decodes GBK encoded bytes to UTF-8.
func decodeGBK(data []byte) ([]byte, error) {
	decoder := simplifiedchinese.GBK.NewDecoder()
	decoded, _, err := transform.Bytes(decoder, data)
	if err != nil {
		return nil, err
	}
	return decoded, nil
}

// cleanSinaSymbol converts AxData symbol to Sina format.
func cleanSinaSymbol(symbol string) string {
	s := strings.ToLower(strings.TrimSpace(symbol))
	s = strings.ReplaceAll(s, ".sz", "")
	s = strings.ReplaceAll(s, ".sh", "")
	s = strings.ReplaceAll(s, ".bj", "")

	if !strings.HasPrefix(s, "sz") && !strings.HasPrefix(s, "sh") && !strings.HasPrefix(s, "bj") {
		if strings.HasPrefix(s, "0") || strings.HasPrefix(s, "1") || strings.HasPrefix(s, "2") || strings.HasPrefix(s, "3") {
			s = "sz" + s
		} else if strings.HasPrefix(s, "6") {
			s = "sh" + s
		} else if strings.HasPrefix(s, "9") || strings.HasPrefix(s, "4") || strings.HasPrefix(s, "8") {
			s = "bj" + s
		}
	}
	return s
}

// sinaToAxCode converts Sina code to AxData format.
func sinaToAxCode(code string) string {
	s := strings.ToLower(code)
	if strings.HasPrefix(s, "sz") {
		return strings.TrimPrefix(s, "sz") + ".SZ"
	}
	if strings.HasPrefix(s, "sh") {
		return strings.TrimPrefix(s, "sh") + ".SH"
	}
	if strings.HasPrefix(s, "bj") {
		return strings.TrimPrefix(s, "bj") + ".BJ"
	}
	return s
}

// periodToScale maps period to Sina scale.
func periodToScale(period string) string {
	switch period {
	case "m1", "1m", "minute", "m":
		return "1"
	case "m5", "5m", "5min":
		return "5"
	case "m15", "15m", "15min":
		return "15"
	case "m30", "30m", "30min":
		return "30"
	case "m60", "60m", "60min", "h":
		return "60"
	case "d", "day", "":
		return "240"
	case "w", "week":
		return "10080"
	case "M", "month":
		return "1440"
	default:
		return "240"
	}
}

// httpGet performs an HTTP GET request.
func (a *SinaAdapter) httpGet(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", "https://finance.sina.com.cn")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}
