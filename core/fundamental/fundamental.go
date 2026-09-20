// Package fundamental assembles a company profile from financial statements,
// business composition and valuation multiples.
package fundamental

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/electkismet/axdata-go/core/cache"
	"github.com/electkismet/axdata-go/core/valuation"
)

// Service reads fundamentals through the local-first cache facade.
type Service struct {
	Getter *cache.Getter
}

// Statement is one reporting period of a financial statement.
type Statement struct {
	Name             string
	ReportDate       string
	NoticeDate       string
	Revenue          float64
	OperatingCost    float64
	NetProfit        float64
	NetProfitDeduct  float64
	OperatingProfit  float64
	TotalProfit      float64
	GrossMargin      float64
	TotalAssets      float64
	TotalLiabilities float64
	TotalEquity      float64
	DebtAssetRatio   float64
	OCF              float64
	Capex            float64
	FreeCashFlow     float64
}

// RevenueDriver is one line of the main-business composition breakdown.
// Ratio fields are fractions in [0,1], matching the upstream data.
type RevenueDriver struct {
	ReportDate       string
	Type             string
	Item             string
	Income           float64
	IncomeRatio      float64
	Cost             float64
	Profit           float64
	GrossProfitRatio float64
	Rank             int
}

// Valuation is a point-in-time multiple snapshot.
type Valuation struct {
	Code           string
	Name           string
	Industry       string
	TradeDate      string
	ClosePrice     float64
	TotalMarketCap float64
	FreeMarketCap  float64
	TotalShares    float64
	FreeShares     float64
	PERatio        float64
	PELAR          float64
	PBRatio        float64
	PSRatio        float64
	PCRatio        float64
	PEG            float64
}

// Profile is the assembled company view.
type Profile struct {
	Code       string
	Name       string
	Industry   string
	Statements []Statement
	Drivers    []RevenueDriver
	Valuation  *Valuation
	Metrics    Metrics
}

// Metrics holds derived ratios for the most recent reporting period.
// Every ratio field is a fraction in [0,1] except the YoY fields, which are
// growth rates such as 0.25 for +25%.
type Metrics struct {
	ReportDate string
	Revenue    float64
	// RevenueYoY is the year-over-year revenue growth rate.
	RevenueYoY float64
	NetProfit  float64
	// NetProfitYoY is the year-over-year net profit growth rate.
	NetProfitYoY float64
	NetMargin    float64
	GrossMargin  float64
	ROE          float64
	ROA          float64
	DebtRatio    float64
	// OCFToNetProfit is operating cash flow divided by net profit.
	OCFToNetProfit float64
	// TopDriver is the revenue driver with the largest income share.
	TopDriver string
	// TopDriverShare is that share as a fraction.
	TopDriverShare float64
	// RevenueConcentration is the combined share of the three largest drivers.
	RevenueConcentration float64
}

// Statements returns the income, balance and cashflow statements merged by
// reporting period, newest first. Fetches double the requested depth so that a
// year-over-year base period is available even after truncation.
func (s *Service) Statements(ctx context.Context, code string, limit int) ([]Statement, error) {
	if limit <= 0 {
		limit = 12
	}
	fetchLimit := limit * 2
	income, err := s.fetch(ctx, "fin_income", code, fetchLimit)
	if err != nil {
		return nil, fmt.Errorf("fundamental: income: %w", err)
	}
	balance, err := s.fetch(ctx, "fin_balance", code, fetchLimit)
	if err != nil {
		return nil, fmt.Errorf("fundamental: balance: %w", err)
	}
	cashflow, err := s.fetch(ctx, "fin_cashflow", code, fetchLimit)
	if err != nil {
		return nil, fmt.Errorf("fundamental: cashflow: %w", err)
	}

	byDate := map[string]*Statement{}
	get := func(d string) *Statement {
		if r, ok := byDate[d]; ok {
			return r
		}
		r := &Statement{ReportDate: d}
		byDate[d] = r
		return r
	}
	for _, r := range income {
		st := get(r.Str("report_date"))
		st.Name = r.Str("name")
		st.NoticeDate = r.Str("notice_date")
		st.Revenue = r.F64("revenue")
		st.OperatingCost = r.F64("operating_cost")
		st.NetProfit = r.F64("net_profit")
		st.NetProfitDeduct = r.F64("net_profit_deduct")
		st.OperatingProfit = r.F64("operating_profit")
		st.TotalProfit = r.F64("total_profit")
		st.GrossMargin = r.F64("gross_margin")
	}
	for _, r := range balance {
		st := get(r.Str("report_date"))
		st.TotalAssets = r.F64("total_assets")
		st.TotalLiabilities = r.F64("total_liabilities")
		st.TotalEquity = r.F64("total_equity")
		st.DebtAssetRatio = r.F64("debt_asset_ratio")
	}
	for _, r := range cashflow {
		st := get(r.Str("report_date"))
		st.OCF = r.F64("ocf")
		st.Capex = r.F64("capex")
		st.FreeCashFlow = r.F64("free_cash_flow")
	}

	out := make([]Statement, 0, len(byDate))
	for _, st := range byDate {
		out = append(out, *st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ReportDate > out[j].ReportDate })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// Drivers returns the main-business composition lines, newest period first.
func (s *Service) Drivers(ctx context.Context, code string, limit int) ([]RevenueDriver, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.fetch(ctx, "business_scope", code, limit*4)
	if err != nil {
		return nil, fmt.Errorf("fundamental: business scope: %w", err)
	}
	out := make([]RevenueDriver, 0, len(rows))
	for _, r := range rows {
		out = append(out, RevenueDriver{
			ReportDate:       r.Str("report_date"),
			Type:             r.Str("mainop_type"),
			Item:             r.Str("item_name"),
			Income:           r.F64("income"),
			IncomeRatio:      r.F64("income_ratio"),
			Cost:             r.F64("cost"),
			Profit:           r.F64("profit"),
			GrossProfitRatio: r.F64("gross_profit_ratio"),
			Rank:             int(r.I64("rank")),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ReportDate != out[j].ReportDate {
			return out[i].ReportDate > out[j].ReportDate
		}
		return out[i].Rank < out[j].Rank
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// Valuation returns the latest available valuation snapshot.
func (s *Service) Valuation(ctx context.Context, code string) (*Valuation, error) {
	rows, err := s.fetch(ctx, "valuation_snapshot", code, 1)
	if err != nil {
		return nil, fmt.Errorf("fundamental: valuation: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("fundamental: no valuation data for %s", code)
	}
	r := rows[0]
	return &Valuation{
		Code:           code,
		Name:           r.Str("name"),
		Industry:       r.Str("industry"),
		TradeDate:      r.Str("trade_date"),
		ClosePrice:     r.F64("close_price"),
		TotalMarketCap: r.F64("total_market_cap"),
		FreeMarketCap:  r.F64("free_market_cap"),
		TotalShares:    r.F64("total_shares"),
		FreeShares:     r.F64("free_shares"),
		PERatio:        r.F64("pe_ttm"),
		PELAR:          r.F64("pe_lar"),
		PBRatio:        r.F64("pb"),
		PSRatio:        r.F64("ps_ttm"),
		PCRatio:        r.F64("pcf_ttm"),
		PEG:            r.F64("peg"),
	}, nil
}

// Profile assembles the full company view in one call.
func (s *Service) Profile(ctx context.Context, code string) (*Profile, error) {
	statements, err := s.Statements(ctx, code, 12)
	if err != nil {
		return nil, err
	}
	drivers, err := s.Drivers(ctx, code, 50)
	if err != nil {
		return nil, err
	}
	v, err := s.Valuation(ctx, code)
	if err != nil {
		return nil, err
	}

	p := &Profile{
		Code:       code,
		Name:       v.Name,
		Industry:   v.Industry,
		Statements: statements,
		Drivers:    drivers,
		Valuation:  v,
	}
	p.Metrics = computeMetrics(statements, drivers)
	return p, nil
}

// fetch applies the local-first strategy for a table filtered by ts_code.
func (s *Service) fetch(ctx context.Context, table, code string, minRows int) ([]cache.Row, error) {
	return s.Getter.FetchTable(ctx, table, map[string]string{"ts_code": code}, minRows, func(ctx context.Context) ([]map[string]interface{}, error) {
		name := tableInterface(table)
		if name == "" {
			return nil, fmt.Errorf("no source interface for table %s", table)
		}
		return s.Getter.SourceRequest(ctx, "eastmoney", name,
			map[string]interface{}{"code": code, "limit": minRows})
	})
}

func tableInterface(table string) string {
	switch table {
	case "fin_income":
		return "eastmoney_financial_income"
	case "fin_balance":
		return "eastmoney_financial_balance"
	case "fin_cashflow":
		return "eastmoney_financial_cashflow"
	case "business_scope":
		return "eastmoney_business_scope"
	case "earnings_forecast":
		return "eastmoney_earnings_forecast"
	case "valuation_snapshot":
		return "eastmoney_valuation_snapshot"
	}
	return ""
}

func computeMetrics(statements []Statement, drivers []RevenueDriver) Metrics {
	m := Metrics{}
	if len(statements) == 0 {
		return m
	}
	cur := statements[0]
	m.ReportDate = cur.ReportDate
	m.Revenue = cur.Revenue
	m.NetProfit = cur.NetProfit
	m.NetMargin = ratio(cur.NetProfit, cur.Revenue)
	m.GrossMargin = cur.GrossMargin
	m.ROE = ratio(cur.NetProfit, cur.TotalEquity)
	m.ROA = ratio(cur.NetProfit, cur.TotalAssets)
	m.DebtRatio = cur.DebtAssetRatio
	m.OCFToNetProfit = ratio(cur.OCF, cur.NetProfit)

	if prev := SamePeriodLastYear(statements, cur.ReportDate); prev != nil {
		m.RevenueYoY = yoy(cur.Revenue, prev.Revenue)
		m.NetProfitYoY = yoy(cur.NetProfit, prev.NetProfit)
	}

	latest := latestPeriod(drivers)
	if len(latest) > 0 {
		sort.Slice(latest, func(i, j int) bool { return latest[i].IncomeRatio > latest[j].IncomeRatio })
		m.TopDriver = latest[0].Item
		m.TopDriverShare = latest[0].IncomeRatio
		sumTop3 := 0.0
		for i, d := range latest {
			if i >= 3 {
				break
			}
			sumTop3 += d.IncomeRatio
		}
		// IncomeRatio is a fraction in [0,1], so the top-3 sum is already a fraction.
		m.RevenueConcentration = sumTop3
	}
	return m
}

// latestPeriod returns the main-business lines for the most recent reporting
// period, restricted to a single breakdown dimension. Eastmoney returns several
// dimensions per period (industry, product, region); summing across them counts
// the same revenue twice, which is why concentration would otherwise exceed 100%.
//
// The dimension is chosen by preference rather than by row position: the caller
// sorts by rank, and several dimensions can hold a rank-1 row, so relying on
// first-in-slice would be order-dependent.
func latestPeriod(drivers []RevenueDriver) []RevenueDriver {
	if len(drivers) == 0 {
		return nil
	}
	date := drivers[0].ReportDate
	want := ""
	for _, d := range drivers {
		if d.ReportDate != date {
			continue
		}
		if want == "" || dimensionRank(d.Type) < dimensionRank(want) {
			want = d.Type
		}
	}
	var out []RevenueDriver
	for _, d := range drivers {
		if d.ReportDate == date && d.Type == want {
			out = append(out, d)
		}
	}
	return out
}

// dimensionRank orders breakdown dimensions by informativeness. Product
// composition usually says more about where revenue comes from than
// "the company's whole industry is X", which is why it is preferred first.
// Unknown codes sort last but remain deterministic.
func dimensionRank(mainopType string) int {
	switch strings.TrimSpace(mainopType) {
	case "2":
		return 0 // 产品
	case "1":
		return 1 // 行业
	case "3":
		return 2 // 地区
	case "4":
		return 3 // 销售模式
	case "5":
		return 4 // 客户
	default:
		return 100 + len(mainopType)
	}
}

// DimensionName maps Eastmoney's numeric mainop_type code to a readable
// breakdown label. Unknown codes are passed through unchanged.
func DimensionName(mainopType string) string {
	switch strings.TrimSpace(mainopType) {
	case "1":
		return "行业"
	case "2":
		return "产品"
	case "3":
		return "地区"
	case "4":
		return "销售模式"
	case "5":
		return "客户"
	}
	return mainopType
}

func ratio(num, den float64) float64 {
	if den == 0 {
		return 0
	}
	return num / den
}

// yoy returns the year-over-year change as a fraction, 0 when not comparable.
func yoy(cur, prev float64) float64 {
	if prev == 0 {
		return 0
	}
	return (cur - prev) / prev
}

// compactDate reduces a report date to its 8-digit YYYYMMDD form.
func compactDate(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '-' || r == '/' {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
}

// SamePeriodLastYear returns the statement filed for the same fiscal period as
// reportDate but one year earlier, so growth rates compare like for like
// (H1 against H1) instead of against whatever period happens to be adjacent.
func SamePeriodLastYear(statements []Statement, reportDate string) *Statement {
	d := compactDate(reportDate)
	if len(d) < 8 {
		return nil
	}
	year, err := strconv.Atoi(d[:4])
	if err != nil {
		return nil
	}
	want := fmt.Sprintf("%04d%s", year-1, d[4:8])
	for i := range statements {
		if compactDate(statements[i].ReportDate) == want {
			return &statements[i]
		}
	}
	return nil
}

// PeriodOf maps a report date to its fiscal period label. It accepts both
// YYYYMMDD (the AxData storage format) and YYYY-MM-DD.
func PeriodOf(reportDate string) string {
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

// CashFlowSeries returns FCF history for a DCF model, oldest first.
func CashFlowSeries(statements []Statement) valuation.CashFlows {
	sorted := make([]Statement, len(statements))
	copy(sorted, statements)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ReportDate < sorted[j].ReportDate
	})
	fcf := make([]float64, 0, len(sorted))
	ocf := make([]float64, 0, len(sorted))
	for _, st := range sorted {
		if st.ReportDate == "" {
			continue
		}
		fcf = append(fcf, st.FreeCashFlow)
		ocf = append(ocf, st.OCF)
	}
	return valuation.CashFlows{FreeCashFlow: fcf, OperatingCashFlow: ocf}
}
