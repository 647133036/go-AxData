package cls

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// CLSAdapter implements the CLS (财联社) API adapter.
type CLSAdapter struct {
	client      *http.Client
	description string
}

// NewCLSAdapter creates a new CLS adapter.
func NewCLSAdapter() *CLSAdapter {
	return &CLSAdapter{
		client:      &http.Client{Timeout: 15 * time.Second},
		description: "CLS (财联社) API adapter",
	}
}

// Name returns the adapter name.
func (a *CLSAdapter) Name() string {
	return "cls"
}

// Description returns adapter info.
func (a *CLSAdapter) Description() string {
	return a.description
}

// Request fetches data from CLS.
func (a *CLSAdapter) Request(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	interfaceName, _ := params["interface"].(string)

	switch interfaceName {
	case "cls_market_emotion":
		return a.requestMarketEmotion(ctx)
	case "cls_market_wind":
		return a.requestMarketWind(ctx)
	case "cls_market_wind_stocks":
		return a.requestMarketWindStocks(ctx, params)
	case "cls_market_mainline":
		return a.requestMarketMainline(ctx)
	case "cls_sector_industry":
		return a.requestSectorList(ctx, "industry", "change")
	case "cls_sector_top":
		return a.requestSectorList(ctx, "industry", "change")
	case "cls_sector_heat":
		return a.requestSectorHeat(ctx)
	case "cls_sector_hot_concept":
		return a.requestSectorHeat(ctx)
	case "cls_sector_rise_concept":
		return a.requestSectorList(ctx, "concept", "change")
	case "cls_sector_volume_concept":
		return nil, fmt.Errorf("cls_sector_volume_concept: CLS API does not support volume sort for concept list, not yet implemented")
	case "cls_sector_amount_concept":
		return nil, fmt.Errorf("cls_sector_amount_concept: CLS API does not support amount sort for concept list, not yet implemented")
	case "cls_sector_popular_stocks":
		return a.requestSectorPopularStocks(ctx, params)
	case "cls_sector_rotation":
		return a.requestSectorRotation(ctx, params)
	case "cls_limit_up_pool":
		return a.requestLimitUpPool(ctx)
	case "cls_stock_timeline":
		return a.requestStockTimeline(ctx, params)
	case "cls_stock_kline":
		return a.requestStockKline(ctx, params)
	case "cls_news_telegraph":
		return a.requestNewsTelegraph(ctx, params)
	default:
		return nil, fmt.Errorf("unknown interface: %s", interfaceName)
	}
}

// ─── Interface implementations ───────────────────────────────────────

func (a *CLSAdapter) requestMarketEmotion(ctx context.Context) ([]map[string]interface{}, error) {
	resp, err := a.httpGet(ctx, "https://x-quote.cls.cn/v2/quote/a/stock/emotion", map[string]string{
		"app":  "CailianpressWeb",
		"os":   "web",
		"sv":   "8.4.6",
		"sign": "9f8797a1f4de66c2370f7a03990d2737",
	}, "x-quote.cls.cn", false)
	if err != nil {
		return nil, err
	}
	raw, err := webStyleRaw(resp, "CLS market emotion")
	if err != nil {
		return nil, err
	}
	body := raw.(map[string]interface{})

	upDown, _ := body["up_down_dis"].(map[string]interface{})
	board, _ := body["limit_up_board"].(map[string]interface{})

	return []map[string]interface{}{
		{
			"market_degree":       parseFloat(body["market_degree"]),
			"shsz_balance":        cleanText(body["shsz_balance"]),
			"preview_balance":     cleanText(body["preview_balance"]),
			"up_ratio":            cleanText(body["up_ratio"]),
			"up_ratio_num":        parseInt(body["up_ratio_num"]),
			"up_open_num":         parseInt(body["up_open_num"]),
			"performance":         cleanText(body["performance"]),
			"rise_num":            parseInt(upDown["rise_num"]),
			"fall_num":            parseInt(upDown["fall_num"]),
			"flat_num":            parseInt(upDown["flat_num"]),
			"up_num":              parseInt(upDown["up_num"]),
			"down_num":            parseInt(upDown["down_num"]),
			"raw_up_down_dis":     upDown,
			"raw_limit_up_board":  board,
		},
	}, nil
}

func (a *CLSAdapter) requestMarketWind(ctx context.Context) ([]map[string]interface{}, error) {
	resp, err := a.httpGet(ctx, "https://api3.cls.cn/v2/todayTuyere", signedParams(), "api3.cls.cn", true)
	if err != nil {
		return nil, err
	}
	raw, err := mobileStyleRaw(resp, "CLS market wind")
	if err != nil {
		return nil, err
	}
	body := raw.(map[string]interface{})

	items, _ := body["today_tuyere"].([]interface{})
	results := make([]map[string]interface{}, 0)
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		results = append(results, map[string]interface{}{
			"plate_code": cleanText(m["plate_code"]),
			"plate_name": cleanText(m["title"]),
			"catalyst":   cleanText(m["interpret"]),
		})
	}
	return results, nil
}

func (a *CLSAdapter) requestMarketWindStocks(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	plateCode, err := requiredText(params, "plate_code")
	if err != nil {
		return nil, err
	}
	sp := signedParams()
	sp["plate_code"] = plateCode
	// Recompute sign with the extra param
	delete(sp, "sign")
	sp["sign"] = makeSign(sp)

	resp, err := a.httpGet(ctx, "https://x-quote.cls.cn/v2/quote/a/plate/tuyere/stocks", sp, "x-quote.cls.cn", true)
	if err != nil {
		return nil, err
	}
	items, err := webStyleDataList(resp, "CLS market wind stocks")
	if err != nil {
		return nil, err
	}

	results := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		m := item.(map[string]interface{})
		results = append(results, normalizeCLSStock(m, map[string]interface{}{
			"continuous_count": parseInt(m["continuous"]),
		}))
	}
	return results, nil
}

func (a *CLSAdapter) requestMarketMainline(ctx context.Context) ([]map[string]interface{}, error) {
	resp, err := a.httpGet(ctx, "https://api3.cls.cn/v2/dingPan/mainline", signedParams(), "api3.cls.cn", true)
	if err != nil {
		return nil, err
	}
	raw, err := mobileStyleRaw(resp, "CLS market mainline")
	if err != nil {
		return nil, err
	}
	body := raw.(map[string]interface{})

	rows := make([]map[string]interface{}, 0)
	for key, value := range body {
		if vmap, ok := value.(map[string]interface{}); ok {
				title := cleanText(vmap["title"])
				if title == "" {
					title = cleanText(vmap["name"])
				}
				summary := cleanText(vmap["desc"])
				if summary == "" {
					summary = cleanText(vmap["summary"])
				}
				if summary == "" {
					summary = cleanText(vmap["interpret"])
				}
				rows = append(rows, map[string]interface{}{
					"block_key": key,
					"title":     title,
					"summary":   summary,
					"raw_item":  vmap,
				})
			} else if vlist, ok := value.([]interface{}); ok {
				rows = append(rows, map[string]interface{}{
					"block_key": key,
					"title":     key,
					"summary":   nil,
					"raw_item":  vlist,
				})
			}
		}
	return rows, nil
}

func (a *CLSAdapter) requestSectorList(ctx context.Context, secType, way string) ([]map[string]interface{}, error) {
	resp, err := a.httpGet(ctx, "https://x-quote.cls.cn/web_quote/plate/plate_list", map[string]string{
		"app":   "CailianpressWeb",
		"os":    "web",
		"page":  "5",
		"rever": "1",
		"sv":    "8.4.6",
		"type":  secType,
		"way":   way,
		"sign":  "ef1ec7886be706a0b722d7e7bf3c0054",
	}, "x-quote.cls.cn", false)
	if err != nil {
		return nil, err
	}
	raw, err := webStyleRaw(resp, "CLS sector list")
	if err != nil {
		return nil, err
	}
	body := raw.(map[string]interface{})

	items, _ := body["plate_data"].([]interface{})
	results := make([]map[string]interface{}, 0)
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		results = append(results, map[string]interface{}{
			"plate_code":        cleanText(m["secu_code"]),
			"plate_name":        cleanText(m["secu_name"]),
			"change_pct":        parseFloat(m["change"]),
			"main_fund_diff":    parseFloat(m["main_fund_diff"]),
			"rise_count":        parseInt(m["limit_up"]),
			"fall_count":        parseInt(m["limit_down"]),
			"limit_up_count":    parseInt(m["limit_up_num"]),
			"limit_down_count":  parseInt(m["limit_down_num"]),
			"trade_status":      cleanText(m["trade_status"]),
			"raw_first_stock":   m["first_stock"],
		})
	}
	return results, nil
}

func (a *CLSAdapter) requestSectorHeat(ctx context.Context) ([]map[string]interface{}, error) {
	resp, err := a.httpGet(ctx, "https://x-quote.cls.cn/v2/quote/a/plate/plate_heat_list", signedParams(), "x-quote.cls.cn", true)
	if err != nil {
		return nil, err
	}
	items, err := webStyleDataList(resp, "CLS sector heat")
	if err != nil {
		return nil, err
	}

	results := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		m := item.(map[string]interface{})
		results = append(results, map[string]interface{}{
			"plate_code":  cleanText(m["plate_code"]),
			"plate_name":  cleanText(m["plate_name"]),
			"rank":        parseInt(m["rank"]),
			"cur_heat":    parseFloat(m["cur_heat"]),
			"rank_change": parseInt(m["rank_change"]),
			"is_new":      parseBoolValue(m["is_new"]),
		})
	}
	return results, nil
}

func (a *CLSAdapter) requestSectorPopularStocks(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	plateCode, err := requiredText(params, "plate_code")
	if err != nil {
		return nil, err
	}
	sp := signedParams()
	sp["plate_code"] = plateCode
	delete(sp, "sign")
	sp["sign"] = makeSign(sp)

	resp, err := a.httpGet(ctx, "https://x-quote.cls.cn/v2/quote/a/plate/popular_stocks", sp, "x-quote.cls.cn", true)
	if err != nil {
		return nil, err
	}
	items, err := webStyleDataList(resp, "CLS sector popular stocks")
	if err != nil {
		return nil, err
	}

	results := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		m := item.(map[string]interface{})
		r := normalizeCLSStock(m, map[string]interface{}{
			"change_text": cleanText(m["change"]),
			"change_px":   parseFloat(m["change_px"]),
			"board_tag":   cleanText(m["tbm"]),
			"head_rank":   parseInt(m["head_num"]),
		})
		results = append(results, r)
	}
	return results, nil
}

func (a *CLSAdapter) requestSectorRotation(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	days := positiveInt(params, "days", 4)
	sp := signedParams()
	sp["days"] = strconv.Itoa(days)
	delete(sp, "sign")
	sp["sign"] = makeSign(sp)

	resp, err := a.httpGet(ctx, "https://x-quote.cls.cn/v2/quote/a/plate/rotation", sp, "x-quote.cls.cn", true)
	if err != nil {
		return nil, err
	}
	items, err := webStyleDataList(resp, "CLS sector rotation")
	if err != nil {
		return nil, err
	}

	rows := make([]map[string]interface{}, 0)
	for _, item := range items {
		m := item.(map[string]interface{})
		tradeDate := normalizeDateText(m["trade_date"])
		if tradeDate == "" {
			tradeDate = normalizeDateText(m["date"])
		}
		plates, _ := m["plates"].([]interface{})
		for i, plate := range plates {
			if pm, ok := plate.(map[string]interface{}); ok {
				rows = append(rows, map[string]interface{}{
					"trade_date": tradeDate,
					"plate_code": cleanText(pm["plate_code"]),
					"plate_name": cleanText(pm["plate_name"]),
					"change_pct": parseFloat(pm["change"]),
					"rank":       i + 1,
				})
			}
		}
	}
	return rows, nil
}

func (a *CLSAdapter) requestLimitUpPool(ctx context.Context) ([]map[string]interface{}, error) {
	resp, err := a.httpGet(ctx, "https://x-quote.cls.cn/quote/index/up_down_analysis", map[string]string{
		"app":   "CailianpressWeb",
		"os":    "web",
		"rever": "1",
		"sv":    "8.4.6",
		"type":  "up_pool",
		"way":   "last_px",
		"sign":  "a6ab28604a6dbe891cdbd7764799eda1",
	}, "x-quote.cls.cn", false)
	if err != nil {
		return nil, err
	}
	items, err := webStyleDataList(resp, "CLS limit-up pool")
	if err != nil {
		return nil, err
	}

	results := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		m := item.(map[string]interface{})
		results = append(results, normalizeCLSStock(m, map[string]interface{}{
			"up_reason": cleanText(m["up_reason"]),
		}))
	}
	return results, nil
}

func (a *CLSAdapter) requestStockTimeline(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	code, err := requiredText(params, "code")
	if err != nil {
		return nil, err
	}
	secuCode, err := toCLSSecuCode(code)
	if err != nil {
		return nil, err
	}

	resp, err := a.httpGet(ctx, "https://x-quote.cls.cn/quote/stock/tline", map[string]string{
		"app":       "CailianpressWeb",
		"os":        "web",
		"sv":        "8.4.6",
		"secu_code": secuCode,
		"fields":    "date,minute,last_px,business_balance,business_amount,open_px,preclose_px,av_px",
		"sign":      "afad7ec0475a1b9854313502389f3346",
	}, "x-quote.cls.cn", false)
	if err != nil {
		return nil, err
	}
	raw, err := webStyleRaw(resp, "CLS stock timeline")
	if err != nil {
		return nil, err
	}
	body := raw.(map[string]interface{})
	items, _ := body["line"].([]interface{})

	identity := identityFromCLSCode(secuCode)
	results := make([]map[string]interface{}, 0)
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		r := make(map[string]interface{})
		for k, v := range identity {
			r[k] = v
		}
		r["trade_date"] = normalizeDateText(m["date"])
		r["minute"] = cleanText(m["minute"])
		r["last_price"] = parseFloat(m["last_px"])
		r["amount"] = parseFloat(m["business_balance"])
		r["volume"] = parseFloat(m["business_amount"])
		r["open"] = parseFloat(m["open_px"])
		r["pre_close"] = parseFloat(m["preclose_px"])
		r["avg_price"] = parseFloat(m["av_px"])
		results = append(results, r)
	}
	return results, nil
}

func (a *CLSAdapter) requestStockKline(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	code, err := requiredText(params, "code")
	if err != nil {
		return nil, err
	}
	secuCode, err := toCLSSecuCode(code)
	if err != nil {
		return nil, err
	}

	klineType := "daily"
	if kt, ok := params["kline_type"]; ok {
		klineType = strings.ToLower(strings.TrimSpace(fmt.Sprint(kt)))
	}
	klineMap := map[string]string{
		"daily":   "fd1",
		"weekly":  "fw",
		"monthly": "fm",
		"yearly":  "fy",
	}
	if _, ok := klineMap[klineType]; !ok {
		return nil, fmt.Errorf("kline_type must be daily, weekly, monthly, or yearly")
	}

	limit := positiveInt(params, "limit", 50)
	if limit > 500 {
		limit = 500
	}
	offset := nonNegativeInt(params, "offset", 0)

	resp, err := a.httpGet(ctx, "https://x-quote.cls.cn/quote/stock/kline", map[string]string{
		"app":       "CailianpressWeb",
		"os":        "web",
		"sv":        "8.4.6",
		"secu_code": secuCode,
		"type":      klineMap[klineType],
		"limit":     strconv.Itoa(limit),
		"offset":    strconv.Itoa(offset),
		"sign":      "d2656d0d3fdc1d489f6f316ea820cc17",
	}, "x-quote.cls.cn", false)
	if err != nil {
		return nil, err
	}
	items, err := webStyleDataList(resp, "CLS stock kline")
	if err != nil {
		return nil, err
	}

	identity := identityFromCLSCode(secuCode)
	results := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		m := item.(map[string]interface{})
		changePct := parseFloat(m["change_rate"])
		if changePct == nil {
			changePct = parseFloat(m["change_pct"])
		}
		r := make(map[string]interface{})
		for k, v := range identity {
			r[k] = v
		}
		r["trade_date"] = normalizeDateText(m["date"])
		r["open"] = parseFloat(m["open"])
		r["high"] = parseFloat(m["high"])
		r["low"] = parseFloat(m["low"])
		r["close"] = parseFloat(m["close"])
		r["volume"] = parseFloat(m["volume"])
		r["amount"] = parseFloat(m["amount"])
		r["change"] = parseFloat(m["change"])
		r["change_pct"] = changePct
		results = append(results, r)
	}
	return results, nil
}

func (a *CLSAdapter) requestNewsTelegraph(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	category := "important"
	if c, ok := params["category"]; ok {
		category = strings.ToLower(strings.TrimSpace(fmt.Sprint(c)))
	}
	newsCategoryMap := map[string]string{
		"all":       "",
		"important": "red",
		"company":   "announcement",
	}
	if _, ok := newsCategoryMap[category]; !ok {
		return nil, fmt.Errorf("category must be all, important, or company")
	}

	dateText := normalizeDateText(params["date"])
	if dateText == "" {
		dateText = time.Now().Format("20060102")
	}

	limit := positiveInt(params, "limit", 20)
	if limit > 100 {
		limit = 100
	}

	// Parse dateText as YYYYMMDD and compute day_start/day_end unix timestamps
	date, err := time.ParseInLocation("20060102", dateText, time.Local)
	if err != nil {
		return nil, fmt.Errorf("invalid date format %s, use YYYYMMDD", dateText)
	}
	dayStart := date.Unix()
	dayEnd := time.Date(date.Year(), date.Month(), date.Day(), 23, 59, 59, 0, time.Local).Unix()

	sp := signedParams()
	sp["refresh_type"] = "1"
	sp["last_time"] = strconv.FormatInt(dayEnd, 10)
	sp["rn"] = strconv.Itoa(limit)

	apiCategory := newsCategoryMap[category]
	if apiCategory != "" {
		delete(sp, "sign")
		sp["category"] = apiCategory
		sp["sign"] = makeSign(sp)
	}

	resp, err := a.httpGet(ctx, "https://api3.cls.cn/v1/roll/get_roll_list", sp, "api3.cls.cn", true)
	if err != nil {
		return nil, err
	}
	raw, err := mobileStyleRaw(resp, "CLS news telegraph")
	if err != nil {
		return nil, err
	}
	body := raw.(map[string]interface{})
	items, _ := body["roll_data"].([]interface{})

	results := make([]map[string]interface{}, 0)
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		ctimeVal := m["ctime"]
		var ctime int64
		switch v := ctimeVal.(type) {
		case float64:
			ctime = int64(v)
		case int:
			ctime = int64(v)
		case nil:
			ctime = 0
		default:
			if i, ok2 := ctimeVal.(int64); ok2 {
				ctime = i
			}
		}
		if ctime < dayStart || ctime > dayEnd {
			continue
		}
		publishTime := ""
		if ctime != 0 {
			publishTime = time.Unix(ctime, 0).Format("2006-01-02 15:04:05")
		}
		results = append(results, map[string]interface{}{
			"news_id":      cleanText(m["id"]),
			"title":        cleanText(m["title"]),
			"content":      cleanText(m["content"]),
			"publish_time": publishTime,
			"ctime":        ctime,
			"category":     category,
		})
		if len(results) >= limit {
			break
		}
	}
	return results, nil
}

// ─── HTTP helpers ─────────────────────────────────────────────────────

func (a *CLSAdapter) httpGet(ctx context.Context, urlStr string, rawParams map[string]string, host string, mobile bool) ([]byte, error) {
	params := make(url.Values)
	for k, v := range rawParams {
		params.Set(k, v)
	}
	fullURL := urlStr + "?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("cls request: %w", err)
	}
	if mobile {
		req.Header.Set("User-Agent", "okhttp/4.9.0")
	} else {
		req.Header.Set("User-Agent", "Mozilla/5.0 AxData/0.1")
	}
	req.Header.Set("Accept", "application/json,text/plain,*/*")
	req.Header.Set("Referer", "https://www.cls.cn/")
	req.Header.Set("Host", host)

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cls request: %w", err)
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

// ─── Response parsing ─────────────────────────────────────────────────

func webStyleRaw(data []byte, context string) (interface{}, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%s: invalid JSON: %w", context, err)
	}
	if code, ok := raw["code"]; ok {
		switch v := code.(type) {
		case float64:
			if v != 200 {
				return nil, fmt.Errorf("%s returned unexpected payload (code=%.0f)", context, v)
			}
		case int:
			if v != 200 {
				return nil, fmt.Errorf("%s returned unexpected payload (code=%d)", context, v)
			}
		default:
			return nil, fmt.Errorf("%s returned unexpected payload (code=%v)", context, code)
		}
	}
	d := raw["data"]
	if d == nil {
		return map[string]interface{}{}, nil
	}
	return d, nil
}

func mobileStyleRaw(data []byte, context string) (interface{}, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%s: invalid JSON: %w", context, err)
	}
	errno, ok := raw["errno"]
	if ok {
		switch v := errno.(type) {
		case int:
			if v != 0 {
				return nil, fmt.Errorf("%s returned error: errno=%d", context, v)
			}
		case float64:
			if v != 0 {
				return nil, fmt.Errorf("%s returned error: errno=%.0f", context, v)
			}
		case string:
			if v != "0" {
				return nil, fmt.Errorf("%s returned error: errno=%s", context, v)
			}
		}
	}
	d := raw["data"]
	if d == nil {
		return map[string]interface{}{}, nil
	}
	return d, nil
}

func webStyleDataList(respBytes []byte, context string) ([]interface{}, error) {
	raw, err := webStyleRaw(respBytes, context)
	if err != nil {
		return nil, err
	}
	if arr, ok := raw.([]interface{}); ok {
		return arr, nil
	}
	return []interface{}{}, nil
}

func mobileStyleDataList(respBytes []byte, context string) ([]interface{}, error) {
	raw, err := mobileStyleRaw(respBytes, context)
	if err != nil {
		return nil, err
	}
	if arr, ok := raw.([]interface{}); ok {
		return arr, nil
	}
	return []interface{}{}, nil
}

// ─── Sign helpers ─────────────────────────────────────────────────────

var baseAppParams = map[string]string{
	"app":           "cailianpress",
	"sv":            "8.7.4",
	"os":            "android",
	"mb":            "Xiaomi-2206123SC",
	"ov":            "32",
	"channel":       "8",
	"motif":         "0",
	"net":           "",
	"province_code": "3205",
	"token":         "",
	"uid":           "",
}

func makeSign(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+params[k])
	}
	text := strings.Join(parts, "&")
	sha1Hash := sha1.Sum([]byte(text))
	md5Hash := md5.Sum(sha1Hash[:])
	return fmt.Sprintf("%x", md5Hash)
}

func signedParams() map[string]string {
	params := make(map[string]string)
	for k, v := range baseAppParams {
		params[k] = v
	}
	params["sign"] = makeSign(params)
	return params
}

// ─── Code conversion helpers ──────────────────────────────────────────

var (
	clsSecuRe = regexp.MustCompile(`^(sh|sz|bj)(\d{6})$`)
	sixDigitRe = regexp.MustCompile(`^\d{6}$`)
)

func toCLSSecuCode(value string) (string, error) {
	text := strings.ToLower(strings.TrimSpace(value))
	if clsSecuRe.MatchString(text) {
		return text, nil
	}
	upper := strings.ToUpper(text)
	if strings.HasSuffix(upper, ".SH") {
		return "sh" + upper[:len(upper)-3], nil
	}
	if strings.HasSuffix(upper, ".SZ") {
		return "sz" + upper[:len(upper)-3], nil
	}
	if strings.HasSuffix(upper, ".BJ") {
		return "bj" + upper[:len(upper)-3], nil
	}
	if sixDigitRe.MatchString(upper) {
		if strings.HasPrefix(upper, "6") || strings.HasPrefix(upper, "5") || strings.HasPrefix(upper, "9") {
			return "sh" + upper, nil
		}
		if strings.HasPrefix(upper, "4") || strings.HasPrefix(upper, "8") || strings.HasPrefix(upper, "92") {
			return "bj" + upper, nil
		}
		return "sz" + upper, nil
	}
	return "", fmt.Errorf("code must be a six-digit A-share code")
}

func identityFromCLSCode(secuCode string) map[string]interface{} {
	text := strings.ToLower(strings.TrimSpace(secuCode))
	m := clsSecuRe.FindStringSubmatch(text)
	if m == nil {
		return map[string]interface{}{
			"instrument_id": nil,
			"symbol":        nil,
			"exchange":      nil,
			"secu_code":     secuCode,
			"secu_name":     nil,
		}
	}
	prefix := m[1]
	symbol := m[2]
	exchangeMap := map[string]string{"sh": "SSE", "sz": "SZSE", "bj": "BSE"}
	suffixMap := map[string]string{"SSE": "SH", "SZSE": "SZ", "BSE": "BJ"}
	exchange := exchangeMap[prefix]
	return map[string]interface{}{
		"instrument_id": symbol + "." + suffixMap[exchange],
		"symbol":        symbol,
		"exchange":      exchange,
		"secu_code":     text,
		"secu_name":     nil,
	}
}

func normalizeCLSStock(item map[string]interface{}, extra map[string]interface{}) map[string]interface{} {
	secuCode := cleanText(item["secu_code"])
	identity := identityFromCLSCode(secuCode)
	r := make(map[string]interface{})
	for k, v := range identity {
		r[k] = v
	}
	r["last_price"] = parseFloat(item["last_px"])
	r["change_pct"] = parseFloat(item["change"])
	for k, v := range extra {
		r[k] = v
	}
	return r
}

// ─── Type conversion helpers ──────────────────────────────────────────

var whitespaceRe = regexp.MustCompile(`\s+`)

func cleanText(v interface{}) string {
	if v == nil {
		return ""
	}
	text := whitespaceRe.ReplaceAllString(fmt.Sprint(v), " ")
	text = strings.TrimSpace(text)
	if text == "" || text == "-" || text == "--" || text == "null" || text == "None" {
		return ""
	}
	return text
}

func parseFloat(v interface{}) *float64 {
	t := cleanText(v)
	if t == "" {
		return nil
	}
	t = strings.ReplaceAll(t, "%", "")
	t = strings.ReplaceAll(t, ",", "")
	f, err := strconv.ParseFloat(t, 64)
	if err != nil {
		return nil
	}
	return &f
}

func parseInt(v interface{}) *int {
	f := parseFloat(v)
	if f == nil {
		return nil
	}
	i := int(*f)
	return &i
}

func parseBoolValue(v interface{}) *bool {
	if v == nil {
		return nil
	}
	if b, ok := v.(bool); ok {
		return &b
	}
	s := strings.TrimSpace(fmt.Sprint(v))
	if s == "1" || s == "true" || s == "True" {
		b := true
		return &b
	}
	return nil
}

func normalizeDateText(v interface{}) string {
	if v == nil {
		return ""
	}
	digits := regexp.MustCompile(`\D`).ReplaceAllString(fmt.Sprint(v), "")
	if len(digits) >= 8 {
		return digits[:8]
	}
	return ""
}

func positiveInt(params map[string]interface{}, name string, defaultVal int) int {
	v := params[name]
	if v == nil || v == "" {
		return defaultVal
	}
	switch val := v.(type) {
	case int:
		if val <= 0 {
			return defaultVal
		}
		return val
	case int64:
		if val <= 0 {
			return defaultVal
		}
		return int(val)
	case float64:
		i := int(val)
		if i <= 0 {
			return defaultVal
		}
		return i
	default:
		i, _ := strconv.Atoi(fmt.Sprint(v))
		if i <= 0 {
			return defaultVal
		}
		return i
	}
}

func nonNegativeInt(params map[string]interface{}, name string, defaultVal int) int {
	v := params[name]
	if v == nil || v == "" {
		return defaultVal
	}
	switch val := v.(type) {
	case int:
		if val < 0 {
			return defaultVal
		}
		return val
	case int64:
		if val < 0 {
			return defaultVal
		}
		return int(val)
	case float64:
		i := int(val)
		if i < 0 {
			return defaultVal
		}
		return i
	default:
		i, _ := strconv.Atoi(fmt.Sprint(v))
		if i < 0 {
			return defaultVal
		}
		return i
	}
}

func requiredText(params map[string]interface{}, name string) (string, error) {
	v := params[name]
	if v == nil {
		return "", fmt.Errorf("%s is required", name)
	}
	t := strings.TrimSpace(fmt.Sprint(v))
	if t == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return t, nil
}
