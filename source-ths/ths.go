package ths

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type THSAdapter struct {
	client      *http.Client
	description string
}

var _HTTP_HEADERS = map[string]string{
	"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
	"Referer":    "https://www.10jqka.com.cn/",
}

var _HOT_URL = "https://dq.10jqka.com.cn/fuyao/hot_list_data/out/hot_list/v1/stock"
var _DETAIL_URL = "http://push2delay.eastmoney.com/api/qt/ulist.np/get"

var _DIGIT_RE = regexp.MustCompile(`[^\d]`)

func NewTHSAdapter() *THSAdapter {
	return &THSAdapter{
		client:      &http.Client{Timeout: 30 * time.Second},
		description: "TongHuaShun (同花顺) hot stock rank adapter",
	}
}

func (a *THSAdapter) Name() string {
	return "ths"
}

func (a *THSAdapter) Description() string {
	return a.description
}

func (a *THSAdapter) Request(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	iface, _ := params["interface"].(string)
	if iface != "stock_hot_rank_ths" {
		return nil, fmt.Errorf("unknown interface: %s", iface)
	}
	limit := 100
	if v, ok := params["limit"]; ok {
		if i, ok := v.(int); ok && i > 0 {
			limit = i
		} else if f, ok := v.(float64); ok && f > 0 {
			limit = int(f)
		} else if s, ok := v.(string); ok {
			if i, err := strconv.Atoi(s); err == nil && i > 0 {
				limit = i
			}
		}
	}
	if limit > 100 {
		limit = 100
	}
	return a.getHotRank(ctx, limit)
}

func (a *THSAdapter) getHotRank(ctx context.Context, limit int) ([]map[string]interface{}, error) {
	params := url.Values{
		"stock_type": {"a"},
		"type":       {"hour"},
		"list_type":  {"normal"},
	}
	req, err := http.NewRequestWithContext(ctx, "GET", _HOT_URL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("hot rank request build: %w", err)
	}
	for k, v := range _HTTP_HEADERS {
		req.Header.Set(k, v)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("hot rank request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hot rank HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("hot rank read: %w", err)
	}

	var raw struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg"`
		Data       struct {
			StockList []map[string]interface{} `json:"stock_list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("hot rank unmarshal: %w", err)
	}
	if raw.StatusCode != 0 {
		return nil, fmt.Errorf("hot rank error: status_code=%d msg=%s", raw.StatusCode, raw.StatusMsg)
	}

	stockList := raw.Data.StockList
	if len(stockList) > limit {
		stockList = stockList[:limit]
	}

	var result []map[string]interface{}
	var secids []string
	for i, item := range stockList {
		code := cleanCode(item["code"])
		if code == "" {
			continue
		}
		secid := buildSecid(code)
		if secid != "" {
			secids = append(secids, secid)
		}
		result = append(result, map[string]interface{}{
			"rank":       i + 1,
			"code":       code,
			"name":       cleanText(item["name"]),
			"price":      0.0,
			"change_pct": 0.0,
			"change_amt": 0.0,
			"tag":        item["tag"],
		})
	}

	if len(secids) > 0 {
		if err := a.enrichPrices(ctx, result, secids); err != nil {
			log.Printf("enrich prices warning: %v", err)
		}
	}

	return result, nil
}

func (a *THSAdapter) enrichPrices(ctx context.Context, result []map[string]interface{}, secids []string) error {
	fields := "f2,f3,f4,f12"
	params := map[string]string{
		"fields": fields,
		"invt":   "2",
		"ut":     "fa5fd1943c7b386f172d6893dbfba10b",
		"secids": strings.Join(secids, ","),
	}
	reqURL := _DETAIL_URL + "?" + buildQuery(params)

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return fmt.Errorf("price enrich request build: %w", err)
	}
	for k, v := range _HTTP_HEADERS {
		req.Header.Set(k, v)
	}
	req.Header.Set("Referer", "https://quote.eastmoney.com/")

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("price enrich request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("price enrich read: %w", err)
	}

	var raw struct {
		Rc   int `json:"rc"`
		Data struct {
			Diff []map[string]interface{} `json:"diff"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return fmt.Errorf("price enrich unmarshal: %w", err)
	}
	if raw.Rc != 0 {
		return fmt.Errorf("price enrich error: rc=%d", raw.Rc)
	}

	detailMap := make(map[string]map[string]interface{})
	for _, d := range raw.Data.Diff {
		code := cleanText(d["f12"])
		detailMap[code] = d
	}

	for i, r := range result {
		code, _ := r["code"].(string)
		detail := detailMap[code]
		if detail == nil {
			continue
		}
		result[i]["price"] = floatVal(detail["f2"]) / 100
		result[i]["change_pct"] = floatVal(detail["f3"]) / 100
		result[i]["change_amt"] = floatVal(detail["f4"]) / 100
	}

	return nil
}

func buildSecid(code string) string {
	if strings.HasPrefix(code, "6") {
		return "1." + code
	}
	return "0." + code
}

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
		if val == "-" {
			return 0
		}
		f, _ := strconv.ParseFloat(val, 64)
		return f
	}
	return 0
}

func cleanCode(v interface{}) string {
	if v == nil {
		return ""
	}
	s := fmt.Sprintf("%v", v)
	s = strings.TrimSpace(s)
	if s == "" || s == "undefined" || s == "null" {
		return ""
	}
	digits := _DIGIT_RE.ReplaceAllString(s, "")
	if len(digits) != 6 {
		return ""
	}
	return digits
}

func cleanText(v interface{}) string {
	if v == nil {
		return ""
	}
	s := fmt.Sprintf("%v", v)
	s = strings.TrimSpace(s)
	if s == "" || s == "null" {
		return ""
	}
	return s
}

func buildQuery(params map[string]string) string {
	vals := url.Values{}
	for k, v := range params {
		vals.Set(k, v)
	}
	return vals.Encode()
}
