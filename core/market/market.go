// Package market provides quote, kline and chart assembly on top of the
// local-first cache facade.
package market

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/electkismet/axdata-go/core/cache"
	"github.com/electkismet/axdata-go/core/chart"
	"github.com/electkismet/axdata-go/core/indicator"
	"go.uber.org/zap"
)

// Quote is one real-time observation for a security.
// ChangePct and TurnoverRate are already expressed in percentage points
// (1.69 means 1.69%), matching the upstream eastmoney fields.
type Quote struct {
	Code      string
	Name      string
	LastPrice float64
	// ChangePct is the price change in percentage points.
	ChangePct float64
	Change    float64
	// Volume is in lots (手).
	Volume   float64
	Amount   float64
	Open     float64
	High     float64
	Low      float64
	PreClose float64
	// TurnoverRate is in percentage points.
	TurnoverRate float64
	PE           float64
	PB           float64
	Source       string
}

// KlineOption tunes a kline request.
type KlineOption struct {
	Code      string
	StartDate string
	EndDate   string
	Limit     int
}

// Kline is one OHLCV bar with its date in YYYYMMDD form.
type Kline struct {
	Date   string
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
	Amount float64
}

// Service assembles market data for a single security.
type Service struct {
	Getter *cache.Getter
}

// Quote fetches a real-time snapshot for one or more codes.
func (s *Service) Quote(ctx context.Context, codes ...string) ([]Quote, error) {
	if s.Getter == nil {
		return nil, fmt.Errorf("market: cache getter not configured")
	}
	if len(codes) == 0 {
		return nil, fmt.Errorf("market: at least one code required")
	}
	joined := strings.Join(codes, ",")

	rows, sourceName, err := s.Getter.FanoutRequest(ctx,
		cache.Target{Source: "eastmoney", Interface: "eastmoney_stock_realtime_snapshot",
			Params: map[string]interface{}{"code": joined, "limit": 100}},
		cache.Target{Source: "tencent", Interface: "tencent_realtime_snapshot",
			Params: map[string]interface{}{"symbols": joined}},
		cache.Target{Source: "sina", Interface: "real_time",
			Params: map[string]interface{}{"symbols": joined}},
	)
	if err != nil {
		return nil, fmt.Errorf("market: quote: %w", err)
	}

	byCode := map[string]Quote{}
	for _, r := range rows {
		q := Quote{
			Code:         firstNonEmpty(r, "ts_code", "instrument_id", "symbol", "code"),
			Name:         firstNonEmpty(r, "name"),
			LastPrice:    f64(r, "last_price", "close", "price"),
			ChangePct:    f64(r, "change_pct", "pct_chg"),
			Change:       f64(r, "change"),
			Volume:       f64(r, "volume", "vol"),
			Amount:       f64(r, "amount"),
			Open:         f64(r, "open"),
			High:         f64(r, "high"),
			Low:          f64(r, "low"),
			PreClose:     f64(r, "pre_close"),
			TurnoverRate: f64(r, "turnover_rate"),
			PE:           f64(r, "pe_ttm"),
			PB:           f64(r, "pb"),
			Source:       sourceName,
		}
		if q.Code == "" {
			continue
		}
		byCode[normalizeCode(q.Code)] = q
	}

	out := make([]Quote, 0, len(codes))
	for _, c := range codes {
		if q, ok := byCode[normalizeCode(c)]; ok {
			q.Code = c
			out = append(out, q)
		}
	}
	return out, nil
}

// Kline returns daily bars for one security, local cache first.
func (s *Service) Kline(ctx context.Context, opt KlineOption) ([]Kline, error) {
	if s.Getter == nil {
		return nil, fmt.Errorf("market: cache getter not configured")
	}
	if opt.Code == "" {
		return nil, fmt.Errorf("market: code required")
	}
	limit := opt.Limit
	if limit <= 0 {
		limit = 120
	}

	local, err := s.localKline(ctx, opt.Code, limit)
	if err == nil && len(local) >= minBars(limit) {
		return local, nil
	}

	params := map[string]interface{}{
		"code":       opt.Code,
		"limit":      limit,
		"start_date": opt.StartDate,
		"end_date":   opt.EndDate,
	}
	rows, _, err := s.Getter.FanoutRequest(ctx,
		cache.Target{Source: "tencent", Interface: "stock_zh_a_hist_tx", Params: params},
		cache.Target{Source: "sina", Interface: "kline", Params: map[string]interface{}{
			"symbols": opt.Code, "limit": limit,
			"start_date": opt.StartDate, "end_date": opt.EndDate,
		}},
	)
	if err != nil {
		if len(local) > 0 {
			s.Getter.Logger.Warn("kline source fallback failed, using stale cache",
				zap.String("code", opt.Code), zap.Int("local", len(local)), zap.Error(err))
			return local, nil
		}
		return nil, fmt.Errorf("market: kline for %s: %w", opt.Code, err)
	}

	bars := make([]Kline, 0, len(rows))
	for _, r := range rows {
		bars = append(bars, Kline{
			Date:   firstNonEmpty(r, "trade_date", "date"),
			Open:   f64(r, "open"),
			High:   f64(r, "high"),
			Low:    f64(r, "low"),
			Close:  f64(r, "close"),
			Volume: f64(r, "volume", "vol"),
			Amount: f64(r, "amount"),
		})
	}
	sort.Slice(bars, func(i, j int) bool { return bars[i].Date < bars[j].Date })
	if len(bars) > limit {
		bars = bars[len(bars)-limit:]
	}
	return bars, nil
}

// localKline reads the cached daily table and returns the most recent bars.
func (s *Service) localKline(ctx context.Context, code string, limit int) ([]Kline, error) {
	rows, err := s.Getter.Rows(ctx, "daily", map[string]string{"ts_code": code})
	if err != nil {
		return nil, err
	}
	bars := make([]Kline, 0, len(rows))
	for _, r := range rows {
		bars = append(bars, Kline{
			Date:   r.Str("trade_date"),
			Open:   r.F64("open"),
			High:   r.F64("high"),
			Low:    r.F64("low"),
			Close:  r.F64("close"),
			Volume: r.F64("vol"),
			Amount: r.F64("amount"),
		})
	}
	sort.Slice(bars, func(i, j int) bool { return bars[i].Date < bars[j].Date })
	if len(bars) > limit {
		bars = bars[len(bars)-limit:]
	}
	return bars, nil
}

func minBars(limit int) int {
	if limit/5 < 30 {
		return limit / 5
	}
	return 30
}

// BuildChart turns a security into chart data with moving averages, RSI and
// MACD already computed.
func BuildChart(code, name string, bars []Kline) (*chart.ChartData, error) {
	if len(bars) < 5 {
		return nil, fmt.Errorf("market: need at least 5 bars for a chart, have %d", len(bars))
	}
	dates := make([]string, len(bars))
	open := make([]float64, len(bars))
	high := make([]float64, len(bars))
	low := make([]float64, len(bars))
	close_ := make([]float64, len(bars))
	volume := make([]float64, len(bars))
	for i, b := range bars {
		dates[i] = b.Date
		open[i], high[i], low[i] = b.Open, b.High, b.Low
		close_[i], volume[i] = b.Close, b.Volume
	}

	c := &chart.ChartData{Code: code, Name: name, Dates: dates,
		Open: open, High: high, Low: low, Close: close_, Volume: volume}

	// Indicators leave their pre-warmup slots as 0. Those zeros would otherwise
	// be drawn as real readings and would collapse the chart's price scale to
	// zero, so they are masked to NaN which the renderers treat as missing.
	mask := func(s []float64, warmup int) []float64 {
		for i := 0; i < warmup && i < len(s); i++ {
			s[i] = math.NaN()
		}
		return s
	}
	// A nil series means the window was too short to compute, so renderers skip
	// the pane entirely instead of drawing a flat line of zeros.
	missing := func() []float64 { return nil }
	ma, err := indicator.MA(close_, 5)
	if err != nil {
		c.MA5 = missing()
	} else {
		c.MA5 = mask(ma, 4)
	}
	if ma, err := indicator.MA(close_, 10); err == nil {
		c.MA10 = mask(ma, 9)
	} else {
		c.MA10 = missing()
	}
	if ma, err := indicator.MA(close_, 20); err == nil {
		c.MA20 = mask(ma, 19)
	} else {
		c.MA20 = missing()
	}
	if rsi, err := indicator.RSI(close_, 14); err == nil {
		c.RSI = mask(rsi, 14)
	} else {
		c.RSI = missing()
	}
	if macd, err := indicator.MACD(close_, 12, 26, 9); err == nil {
		c.DIF, c.DEA, c.HIST = mask(macd.DIF, 25), mask(macd.DEA, 33), mask(macd.HIST, 33)
	} else {
		c.DIF, c.DEA, c.HIST = missing(), missing(), missing()
	}
	return c, nil
}

// LastBarDate extracts the most recent bar date, useful for staleness checks.
func LastBarDate(bars []Kline) string {
	if len(bars) == 0 {
		return ""
	}
	d := bars[0].Date
	for _, b := range bars[1:] {
		if b.Date > d {
			d = b.Date
		}
	}
	return d
}

// BarsAsOf drops bars older than the given YYYYMMDD date.
func BarsAsOf(bars []Kline, asOf string) []Kline {
	if asOf == "" {
		return bars
	}
	out := bars[:0:0]
	for _, b := range bars {
		if b.Date <= asOf {
			out = append(out, b)
		}
	}
	return out
}

// RecentTradeDay returns now in YYYYMMDD, walking back over weekends.
func RecentTradeDay(now time.Time) string {
	for now.Weekday() == time.Saturday || now.Weekday() == time.Sunday {
		now = now.AddDate(0, 0, -1)
	}
	return now.Format("20060102")
}

func normalizeCode(code string) string {
	if i := indexDot(code); i >= 0 {
		return code[:i]
	}
	return code
}

func indexDot(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			return i
		}
	}
	return -1
}

func firstNonEmpty(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		v, ok := m[k]
		if !ok || v == nil {
			continue
		}
		switch t := v.(type) {
		case string:
			if t != "" {
				return t
			}
		case []byte:
			if string(t) != "" {
				return string(t)
			}
		default:
			s := fmt.Sprintf("%v", t)
			if s != "" && s != "<nil>" {
				return s
			}
		}
	}
	return ""
}

func f64(m map[string]interface{}, keys ...string) float64 {
	for _, k := range keys {
		v, ok := m[k]
		if !ok || v == nil {
			continue
		}
		switch t := v.(type) {
		case float64:
			return t
		case float32:
			return float64(t)
		case int:
			return float64(t)
		case int64:
			return float64(t)
		case string:
			var f float64
			if _, err := fmt.Sscanf(t, "%g", &f); err == nil {
				return f
			}
		}
	}
	return 0
}
