package wencai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type WencaiAdapter struct {
	client      *http.Client
	description string
}

var (
	_COOKIE_URL = "https://api.levizhang.com/getCookie"
	_WENCAI_URL = "https://www.iwencai.com/stockpick/load-data"

	_HEADERS = map[string]string{
		"User-Agent":       "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/101.0.4951.41 Safari/537.36",
		"Referer":          "https://www.iwencai.com/stockpick/search?typed=1&preParams=&ts=1&f=3&qs=result_rewrite&selfsectsn=&querytype=stock&searchfilter=&tid=stockpick&w=macd&queryarea=",
		"Host":             "www.iwencai.com",
		"X-Requested-With": "XMLHttpRequest",
	}
)

func NewWencaiAdapter() *WencaiAdapter {
	return &WencaiAdapter{
		client:      &http.Client{Timeout: 30 * time.Second},
		description: "iWenCai (i问财) natural language stock strategy query adapter",
	}
}

func (a *WencaiAdapter) Name() string {
	return "wencai"
}

func (a *WencaiAdapter) Description() string {
	return a.description
}

func (a *WencaiAdapter) Request(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	iface, _ := params["interface"].(string)
	if iface != "stock_strategy_wencai" {
		return nil, fmt.Errorf("unknown interface: %s", iface)
	}

	query := ""
	if v, ok := params["query"].(string); ok {
		query = strings.TrimSpace(v)
	}
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}

	page := 1
	if v, ok := params["page"]; ok {
		if i, ok := v.(int); ok && i > 0 {
			page = i
		} else if f, ok := v.(float64); ok && f > 0 {
			page = int(f)
		} else if s, ok := v.(string); ok {
			if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n > 0 {
				page = n
			}
		}
	}

	limit := 50
	if v, ok := params["limit"]; ok {
		if i, ok := v.(int); ok && i > 0 {
			limit = i
		} else if f, ok := v.(float64); ok && f > 0 {
			limit = int(f)
		} else if s, ok := v.(string); ok {
			if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n > 0 {
				limit = n
			}
		}
	}

	return a.queryWencai(ctx, query, page, limit)
}

func (a *WencaiAdapter) getCookie(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", _COOKIE_URL, nil)
	if err != nil {
		return "", fmt.Errorf("cookie request build: %w", err)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("cookie request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("cookie HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("cookie read: %w", err)
	}

	var cookieResp struct {
		Code    string `json:"code"`
		Data    string `json:"data"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(data, &cookieResp); err != nil {
		return "", fmt.Errorf("cookie unmarshal: %w", err)
	}
	if cookieResp.Code != "000000" {
		return "", fmt.Errorf("cookie error: %s", cookieResp.Message)
	}
	return cookieResp.Data, nil
}

func (a *WencaiAdapter) queryWencai(ctx context.Context, query string, page, limit int) ([]map[string]interface{}, error) {
	cookie, err := a.getCookie(ctx)
	if err != nil {
		return nil, fmt.Errorf("wencai cookie: %w", err)
	}

	params := url.Values{
		"typed":     {"1"},
		"ts":        {"1"},
		"f":         {"3"},
		"qs":        {"result_rewrite"},
		"querytype": {"stock"},
		"tid":       {"stockpick"},
		"page":      {fmt.Sprintf("%d", page)},
		"perpage":   {fmt.Sprintf("%d", limit)},
		"w":         {query},
	}

	reqURL := _WENCAI_URL + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("wencai request build: %w", err)
	}

	for k, v := range _HEADERS {
		req.Header.Set(k, v)
	}
	req.Header.Set("Cookie", "v="+cookie)
	req.Header.Set("hexin-v", cookie)

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wencai request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wencai HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("wencai read: %w", err)
	}

	var wencaiResp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			Result struct {
				Title  []interface{}   `json:"title"`
				Result [][]interface{} `json:"result"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &wencaiResp); err != nil {
		return nil, fmt.Errorf("wencai unmarshal: %w", err)
	}

	if !wencaiResp.Success {
		return nil, fmt.Errorf("wencai error: %s", wencaiResp.Message)
	}

	resultMap := wencaiResp.Data.Result

	var rows []map[string]interface{}
	titles := resultMap.Title
	results := resultMap.Result

	for _, row := range results {
		m := map[string]interface{}{
			"_raw": row,
		}
		for i, val := range row {
			var key string
			if i < len(titles) {
				key = fmt.Sprintf("%v", titles[i])
			} else {
				key = fmt.Sprintf("col_%d", i)
			}
			m[key] = val
		}
		rows = append(rows, m)
	}

	if rows == nil {
		rows = []map[string]interface{}{
			{"_raw": []interface{}{}, "title": titles},
		}
	} else {
		// Attach title as metadata on first row
		rows[0]["_title"] = titles
	}

	return rows, nil
}
