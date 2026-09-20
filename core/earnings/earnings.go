// Package earnings compares profit pre-announcements against reported results.
package earnings

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/electkismet/axdata-go/core/cache"
)

// Service reads forecasts and actuals through the local-first cache facade.
type Service struct {
	Getter *cache.Getter
}

// ForecastType groups pre-announcements by their announced direction.
type ForecastType int

const (
	TypeUnknown        ForecastType = iota
	TypePreProfit                   // 预盈
	TypePreLoss                     // 预亏
	TypePreTurn                     // 扭亏
	TypePreReduce                   // 略减
	TypePreIncrease                 // 略增
	TypePreFlat                     // 持平
	TypePreBigIncrease              // 预增
	TypePreBigDecrease              // 预减
)

func (t ForecastType) String() string {
	switch t {
	case TypePreProfit:
		return "预盈"
	case TypePreLoss:
		return "预亏"
	case TypePreTurn:
		return "扭亏"
	case TypePreReduce:
		return "略减"
	case TypePreIncrease:
		return "略增"
	case TypePreFlat:
		return "持平"
	case TypePreBigIncrease:
		return "预增"
	case TypePreBigDecrease:
		return "预减"
	}
	return "未知"
}

// Verdict compares one forecast against the reported result.
type Verdict int

const (
	VerdictUnavailable Verdict = iota
	VerdictBeat                // 超预期
	VerdictMet                 // 符合预期
	VerdictMiss                // 低于预期
	VerdictNoForecast          // 无预告可比
)

func (v Verdict) String() string {
	switch v {
	case VerdictBeat:
		return "超预期"
	case VerdictMet:
		return "符合预期"
	case VerdictMiss:
		return "低于预期"
	case VerdictNoForecast:
		return "无预告可比"
	}
	return "无法比较"
}

// Forecast is one profit pre-announcement.
// ChangeLower and ChangeUpper are percentage points of growth, taken
// directly from eastmoney's ADD_AMP_* fields.
type Forecast struct {
	Code         string
	Name         string
	NoticeDate   string
	ReportDate   string
	Item         string
	ForecastType ForecastType
	TypeLabel    string
	AmountLower  float64
	AmountUpper  float64
	ChangeLower  float64
	ChangeUpper  float64
	Reason       string
}

// Comparison pairs a forecast with its reported outcome.
type Comparison struct {
	Code         string
	Name         string
	ReportDate   string
	NoticeDate   string
	ForecastItem string
	AmountLower  float64
	AmountUpper  float64
	ChangeLower  float64
	ChangeUpper  float64
	Actual       float64
	Midpoint     float64
	BeatPct      float64
	Verdict      Verdict
	Reason       string
}

// Report is the full earnings view for one security.
type Report struct {
	Code        string
	Name        string
	Forecasts   []Forecast
	Comparisons []Comparison
	Latest      *Comparison
}

// Forecasts returns pre-announcements newest first.
func (s *Service) Forecasts(ctx context.Context, code string, limit int) ([]Forecast, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.fetch(ctx, code, limit)
	if err != nil {
		return nil, fmt.Errorf("earnings: forecast: %w", err)
	}
	out := make([]Forecast, 0, len(rows))
	for _, r := range rows {
		ft := classifyForecast(r.Str("forecast_type"), r.Str("forecast_item"))
		out = append(out, Forecast{
			Code:         firstCode(r.Str("ts_code"), r.Str("symbol"), code),
			Name:         r.Str("name"),
			NoticeDate:   r.Str("notice_date"),
			ReportDate:   r.Str("report_date"),
			Item:         r.Str("forecast_item"),
			ForecastType: ft,
			TypeLabel:    ft.String(),
			AmountLower:  r.F64("amount_lower"),
			AmountUpper:  r.F64("amount_upper"),
			ChangeLower:  r.F64("change_lower"),
			ChangeUpper:  r.F64("change_upper"),
			Reason:       r.Str("reason"),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NoticeDate > out[j].NoticeDate })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// Compare matches each forecast against the reported net profit from the
// income statement, marking beats, misses and unmatched forecasts.
func (s *Service) Compare(ctx context.Context, code string, limit int) ([]Comparison, error) {
	if limit <= 0 {
		limit = 12
	}
	forecasts, err := s.Forecasts(ctx, code, limit*2)
	if err != nil {
		return nil, err
	}
	if len(forecasts) == 0 {
		return nil, nil
	}

	actuals, err := s.actualsByPeriod(ctx, code, limit*2)
	if err != nil {
		return nil, fmt.Errorf("earnings: actuals: %w", err)
	}

	out := make([]Comparison, 0, len(forecasts))
	for _, f := range forecasts {
		c := Comparison{
			Code:         code,
			Name:         f.Name,
			ReportDate:   f.ReportDate,
			NoticeDate:   f.NoticeDate,
			ForecastItem: f.Item,
			AmountLower:  f.AmountLower,
			AmountUpper:  f.AmountUpper,
			ChangeLower:  f.ChangeLower,
			ChangeUpper:  f.ChangeUpper,
			Verdict:      VerdictNoForecast,
			Reason:       f.Reason,
		}
		c.Midpoint = (f.AmountLower + f.AmountUpper) / 2
		if !profitItem(f.Item) {
			c.Reason = "每股口径预告，无法与净利润对比"
			out = append(out, c)
			continue
		}
		a, ok := actualFor(f.Item, actuals[f.ReportDate])
		if ok {
			c.Actual = a
			c.BeatPct = beatPct(a, c.Midpoint)
			c.Verdict = judge(c.BeatPct)
		} else {
			c.Verdict = VerdictUnavailable
			c.Reason = "未找到对应报告期财报数据，无法比较"
		}
		out = append(out, c)
	}
	return out, nil
}

// Report assembles forecasts plus comparisons in one call.
func (s *Service) Report(ctx context.Context, code string, limit int) (*Report, error) {
	forecasts, err := s.Forecasts(ctx, code, limit)
	if err != nil {
		return nil, err
	}
	comparisons, err := s.Compare(ctx, code, limit)
	if err != nil {
		return nil, err
	}
	r := &Report{Code: code, Forecasts: forecasts, Comparisons: comparisons}
	if len(forecasts) > 0 {
		r.Name = forecasts[0].Name
	}
	if len(comparisons) > 0 {
		c := comparisons[0]
		r.Latest = &c
	}
	return r, nil
}

// fetch applies the local-first strategy for the forecast table.
func (s *Service) fetch(ctx context.Context, code string, minRows int) ([]cache.Row, error) {
	return s.Getter.FetchTable(ctx, "earnings_forecast", map[string]string{"ts_code": code}, minRows, func(ctx context.Context) ([]map[string]interface{}, error) {
		return s.Getter.SourceRequest(ctx, "eastmoney", "eastmoney_earnings_forecast",
			map[string]interface{}{"code": code, "limit": minRows})
	})
}

// actualsByPeriod maps a report date to its reported income statement figures.
func (s *Service) actualsByPeriod(ctx context.Context, code string, minRows int) (map[string]periodActuals, error) {
	rows, err := s.Getter.FetchTable(ctx, "fin_income", map[string]string{"ts_code": code}, minRows, func(ctx context.Context) ([]map[string]interface{}, error) {
		return s.Getter.SourceRequest(ctx, "eastmoney", "eastmoney_financial_income",
			map[string]interface{}{"code": code, "limit": minRows})
	})
	if err != nil {
		return nil, err
	}
	out := make(map[string]periodActuals, len(rows))
	for _, r := range rows {
		d := r.Str("report_date")
		if d == "" {
			continue
		}
		out[d] = periodActuals{
			Revenue:   r.F64("revenue"),
			NetProfit: r.F64("net_profit"),
			Has:       true,
		}
	}
	return out, nil
}

// classifyForecast maps the announcement type fields onto a ForecastType.
func classifyForecast(typeCode, item string) ForecastType {
	text := strings.TrimSpace(typeCode + " " + item)
	switch {
	case strings.Contains(text, "预亏"):
		return TypePreLoss
	case strings.Contains(text, "扭亏"):
		return TypePreTurn
	case strings.Contains(text, "略增") && !strings.Contains(text, "略减"):
		return TypePreIncrease
	case strings.Contains(text, "略减"):
		return TypePreReduce
	case strings.Contains(text, "预增"):
		return TypePreBigIncrease
	case strings.Contains(text, "预减"):
		return TypePreBigDecrease
	case strings.Contains(text, "预盈"):
		return TypePreProfit
	}
	return TypeUnknown
}

// periodActuals is one reported period's income statement figures.
type periodActuals struct {
	Revenue   float64
	NetProfit float64
	Has       bool
}

// actualFor picks the reported figure a forecast is speaking about. Eastmoney
// pre-announces revenue as well as net profit, so comparing a revenue forecast
// against net profit would report a spurious ~50% miss.
func actualFor(item string, a periodActuals) (float64, bool) {
	if !a.Has || !profitItem(item) {
		return 0, false
	}
	if strings.Contains(item, "营业收入") || strings.Contains(item, "营业总收入") {
		if a.Revenue == 0 {
			return 0, false
		}
		return a.Revenue, true
	}
	if a.NetProfit == 0 {
		return 0, false
	}
	return a.NetProfit, true
}

// profitItem reports whether a forecast is denominated in total profit, as
// opposed to per-share. Only total-profit forecasts can be compared against the
// reported net profit column.
func profitItem(item string) bool {
	return !strings.Contains(item, "每股收益")
}

// beatPct is how far the actual landed above the forecast midpoint.
// beatPct returns how far the actual sits from the forecast midpoint.
func beatPct(actual, midpoint float64) float64 {
	if midpoint == 0 {
		return 0
	}
	return (actual - midpoint) / midpoint
}

// judge buckets a beat percentage into a Verdict.
func judge(beatPct float64) Verdict {
	if beatPct > 0.05 {
		return VerdictBeat
	}
	if beatPct < -0.05 {
		return VerdictMiss
	}
	return VerdictMet
}

func firstCode(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// PeriodLabel converts a forecast notice into the fiscal period it refers to.
func PeriodLabel(reportDate string) string {
	s := strings.Map(func(r rune) rune {
		if r == '-' || r == '/' {
			return -1
		}
		return r
	}, strings.TrimSpace(reportDate))
	if len(s) < 8 {
		return reportDate
	}
	switch s[4:8] {
	case "0331":
		return "Q1"
	case "0630":
		return "H1"
	case "0930":
		return "Q3"
	case "1231":
		return "FY"
	}
	return s[4:8]
}
