package mock

import (
	"context"
	"fmt"
	"math/rand"
	"time"
)

// MockAdapter provides mock data for testing and development.
type MockAdapter struct {
	name        string
	description string
}

// NewMockAdapter creates a new mock adapter.
func NewMockAdapter() *MockAdapter {
	return &MockAdapter{
		name:        "mock",
		description: "Mock adapter for testing - generates synthetic financial data",
	}
}

// Name returns the adapter name.
func (a *MockAdapter) Name() string {
	return a.name
}

// Description returns adapter info.
func (a *MockAdapter) Description() string {
	return a.description
}

// Request generates mock data.
func (a *MockAdapter) Request(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	interfaceName, _ := params["interface"].(string)
	limit, _ := params["limit"].(int)
	if limit == 0 {
		limit = 20
	}
	symbol, _ := params["symbol"].(string)

	var results []map[string]interface{}

	switch interfaceName {
	case "daily":
		results = a.generateDaily(symbol, limit)
	case "quote":
		results = a.generateQuote(symbol, limit)
	case "stock_list":
		results = a.generateStockList(limit)
	case "calendar":
		results = a.generateCalendar()
	default:
		results = a.generateDaily("", limit)
	}

	return results, nil
}

// generateDaily creates mock daily OHLCV data.
func (a *MockAdapter) generateDaily(symbol string, limit int) []map[string]interface{} {
	var results []map[string]interface{}

	for i := 0; i < limit; i++ {
		date := time.Now().AddDate(0, 0, -i*5) // every 5 days
		open := 10.0 + rand.Float64()*15.0
		high := open + rand.Float64()*2.0
		low := open - rand.Float64()*1.0
		close := low + rand.Float64()*(high-low)
		volume := int64(100000 + rand.Int63n(5000000))

		sym := symbol
		if sym == "" {
			sym = fmt.Sprintf("%06d.SZ", 100000+i)
		}

		record := map[string]interface{}{
			"ts_code":    sym,
			"trade_date": date.Format("20060102"),
			"open":       open,
			"high":       high,
			"low":        low,
			"close":      close,
			"pre_close":  open - 0.1,
			"change":     close - (open - 0.1),
			"pct_chg":    ((close - (open - 0.1)) / (open - 0.1)) * 100,
			"vol":        float64(volume / 100),
			"amount":     float64(volume) * close / 1000,
		}
		results = append(results, record)
	}

	return results
}

// generateQuote creates mock real-time quote data.
func (a *MockAdapter) generateQuote(symbol string, limit int) []map[string]interface{} {
	var results []map[string]interface{}

	for i := 0; i < limit; i++ {
		sym := symbol
		if sym == "" {
			sym = fmt.Sprintf("%06d.SZ", 100000+i)
		}

		bid := 15.0 + rand.Float64()*20.0
		ask := bid + 0.01

		record := map[string]interface{}{
			"instrument_id": sym,
			"open":          14.5 + rand.Float64()*5.0,
			"close":         bid - 0.1,
			"high":          bid + 0.5,
			"low":           bid - 0.3,
			"bid":           bid,
			"ask":           ask,
			"bid_vol":       int64(100 + rand.Int63n(5000)),
			"ask_vol":       int64(100 + rand.Int63n(5000)),
			"volume":        int64(500000 + rand.Int63n(10000000)),
			"amount":        80000000 + rand.Int63n(200000000),
			"timestamp":     time.Now().Format(time.RFC3339),
		}
		results = append(results, record)
	}

	return results
}

// generateStockList creates mock stock list.
func (a *MockAdapter) generateStockList(limit int) []map[string]interface{} {
	names := []string{
		"平安银行", "万科A", "格力电器", "招商银行", "五粮液",
		"美的集团", "恒瑞医药", "中信证券", "海康威视", "大华股份",
		"贵州茅台", "伊利股份", "中兴通讯", "用友网络", "科大讯飞",
	}

	var results []map[string]interface{}
	for i := 0; i < limit; i++ {
		name := "股票"
		if i < len(names) {
			name = names[i]
		}
		code := fmt.Sprintf("%06d", 100000+i)
		sym := code
		if i%2 == 0 {
			sym = code + ".SZ"
		} else {
			sym = code + ".SH"
		}

		record := map[string]interface{}{
			"instrument_id":  sym,
			"symbol":         code,
			"exchange":       "SZSE",
			"name":           name,
			"market":         "主板",
			"region":         "广东",
			"industry":       "科技",
			"total_share":    1.0 + rand.Float64()*20.0,
			"float_share":    0.5 + rand.Float64()*10.0,
			"list_date":      fmt.Sprintf("20%02d0101", 10+rand.Intn(10)),
			"delist_date":    "",
			"listing_status": "listed",
		}
		if i%2 == 1 {
			record["exchange"] = "SSE"
		}
		results = append(results, record)
	}

	return results
}

// generateCalendar creates mock trading calendar.
func (a *MockAdapter) generateCalendar() []map[string]interface{} {
	var results []map[string]interface{}

	for i := 0; i < 30; i++ {
		date := time.Now().AddDate(0, 0, i)
		dow := date.Weekday()
		isOpen := int64(1)
		if dow == time.Saturday || dow == time.Sunday {
			isOpen = 0
		}

		record := map[string]interface{}{
			"exchange":     "SSE",
			"cal_date":     date.Format("20060102"),
			"is_open":      isOpen,
			"pretrade_date": date.AddDate(0, 0, -1).Format("20060102"),
		}
		results = append(results, record)
	}

	return results
}
