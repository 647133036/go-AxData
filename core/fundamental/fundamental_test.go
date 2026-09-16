package fundamental

import (
	"math"
	"testing"
)

func TestPeriodOf(t *testing.T) {
	cases := map[string]string{
		"2025-03-31": "Q1",
		"2025-06-30": "H1",
		"2025-09-30": "Q3",
		"2025-12-31": "FY",
		"20250331":   "Q1",
		"20250630":   "H1",
		"20251231":   "FY",
		"20250115":   "0115",
		"":           "",
		"n/a":        "n/a",
	}
	for in, want := range cases {
		if got := PeriodOf(in); got != want {
			t.Fatalf("PeriodOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRatio(t *testing.T) {
	if got := ratio(10, 50); got != 0.2 {
		t.Fatalf("ratio(10,50) = %f", got)
	}
	if got := ratio(10, 0); got != 0 {
		t.Fatalf("ratio with zero denominator must be 0, got %f", got)
	}
	if got := ratio(-5, 10); got != -0.5 {
		t.Fatalf("ratio(-5,10) = %f", got)
	}
}

func TestYoY(t *testing.T) {
	if got := yoy(110, 100); got != 0.1 {
		t.Fatalf("yoy(110,100) = %f", got)
	}
	if got := yoy(80, 100); got != -0.2 {
		t.Fatalf("yoy(80,100) = %f", got)
	}
	if got := yoy(110, 0); got != 0 {
		t.Fatalf("yoy with zero base must be 0, got %f", got)
	}
	if got := yoy(-50, 100); got != -1.5 {
		t.Fatalf("yoy(-50,100) = %f", got)
	}
}

func TestLatestPeriod(t *testing.T) {
	drivers := []RevenueDriver{
		{ReportDate: "2025-12-31", Item: "A", Rank: 1},
		{ReportDate: "2025-12-31", Item: "B", Rank: 2},
		{ReportDate: "2025-06-30", Item: "C", Rank: 1},
	}
	got := latestPeriod(drivers)
	if len(got) != 2 {
		t.Fatalf("expected 2 drivers in the latest period, got %d", len(got))
	}
	for _, d := range got {
		if d.ReportDate != "2025-12-31" {
			t.Fatalf("unexpected date in latest period: %s", d.ReportDate)
		}
	}
	if got := latestPeriod(nil); got != nil {
		t.Fatal("latestPeriod(nil) must be nil")
	}
}

// TestLatestPeriodSingleDimension guards the double-counting fix. Eastmoney
// returns several breakdown dimensions per period, each holding its own
// rank-1 row; picking the first slice element would be order-dependent.
func TestLatestPeriodSingleDimension(t *testing.T) {
	drivers := []RevenueDriver{
		// Region sits first with a rank-1 row, so position-based selection
		// would have chosen it. Product must win instead.
		{ReportDate: "20250630", Type: "3", Item: "Domestic", IncomeRatio: 0.99, Rank: 1},
		{ReportDate: "20250630", Type: "2", Item: "ProductA", IncomeRatio: 0.80, Rank: 1},
		{ReportDate: "20250630", Type: "2", Item: "ProductB", IncomeRatio: 0.20, Rank: 2},
		{ReportDate: "20250630", Type: "1", Item: "Industry", IncomeRatio: 1.00, Rank: 1},
		{ReportDate: "20250331", Type: "2", Item: "Stale", IncomeRatio: 1.00, Rank: 1},
	}
	got := latestPeriod(drivers)
	if len(got) != 2 {
		t.Fatalf("expected only the two product rows, got %d: %v", len(got), got)
	}
	for _, d := range got {
		if d.Type != "2" {
			t.Fatalf("expected dimension 2 only, got %q (%s)", d.Type, d.Item)
		}
	}
	// The two ratios must sum to 1, never exceed it.
	var sum float64
	for _, d := range got {
		sum += d.IncomeRatio
	}
	approx(t, sum, 1.0, 1e-9)
}

// TestLatestPeriodFallsBackToAvailableDimension checks that a security with no
// product breakdown (banks, for instance) still gets its best dimension.
func TestLatestPeriodFallsBackToAvailableDimension(t *testing.T) {
	drivers := []RevenueDriver{
		{ReportDate: "20250630", Type: "3", Item: "Domestic", IncomeRatio: 0.9, Rank: 1},
		{ReportDate: "20250630", Type: "1", Item: "Wholesale", IncomeRatio: 0.46, Rank: 1},
		{ReportDate: "20250630", Type: "1", Item: "Retail", IncomeRatio: 0.34, Rank: 2},
	}
	got := latestPeriod(drivers)
	if len(got) != 2 {
		t.Fatalf("expected the two industry rows, got %d", len(got))
	}
	for _, d := range got {
		if d.Type != "1" {
			t.Fatalf("expected dimension 1 only, got %q", d.Type)
		}
	}
}

func TestDimensionName(t *testing.T) {
	cases := map[string]string{
		"1":   "行业",
		"2":   "产品",
		"3":   "地区",
		"4":   "销售模式",
		"5":   "客户",
		" 2 ": "产品",
		"7":   "7",
		"":    "",
	}
	for in, want := range cases {
		if got := DimensionName(in); got != want {
			t.Fatalf("DimensionName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestComputeMetricsEmpty(t *testing.T) {
	m := computeMetrics(nil, nil)
	if m.ReportDate != "" {
		t.Fatalf("empty metrics must have no report date, got %q", m.ReportDate)
	}
}

func TestComputeMetricsBasic(t *testing.T) {
	stmts := []Statement{
		{ReportDate: "2025-12-31", Revenue: 1000, NetProfit: 100, TotalEquity: 500,
			TotalAssets: 800, OCF: 150, GrossMargin: 0.3, DebtAssetRatio: 0.4},
		{ReportDate: "2025-06-30", Revenue: 800, NetProfit: 70},
		{ReportDate: "2024-12-31", Revenue: 800, NetProfit: 70},
	}
	m := computeMetrics(stmts, nil)
	if m.Revenue != 1000 {
		t.Fatalf("revenue: %f", m.Revenue)
	}
	approx(t, m.NetMargin, 0.1, 1e-12)
	approx(t, m.ROE, 0.2, 1e-12)
	approx(t, m.ROA, 0.125, 1e-12)
	approx(t, m.OCFToNetProfit, 1.5, 1e-12)
	approx(t, m.RevenueYoY, 0.25, 1e-12)
	approx(t, m.NetProfitYoY, 30/70.0, 1e-12)
}

// TestComputeMetricsSkipsAdjacentPeriods ensures growth compares the same
// fiscal period a year earlier. H1 must not be used as the base for an FY
// figure, which is the error that produced a nonsense +100% H1-over-Q1.
func TestComputeMetricsSkipsAdjacentPeriods(t *testing.T) {
	stmts := []Statement{
		{ReportDate: "2025-12-31", Revenue: 1000, NetProfit: 100},
		{ReportDate: "2025-06-30", Revenue: 500, NetProfit: 50},
		{ReportDate: "2025-03-31", Revenue: 200, NetProfit: 20},
	}
	m := computeMetrics(stmts, nil)
	if m.RevenueYoY != 0 {
		t.Fatalf("FY revenue YoY must be 0 with no prior-year FY, got %f", m.RevenueYoY)
	}
	if m.NetProfitYoY != 0 {
		t.Fatalf("FY net profit YoY must be 0 with no prior-year FY, got %f", m.NetProfitYoY)
	}
}

func TestSamePeriodLastYear(t *testing.T) {
	stmts := []Statement{
		{ReportDate: "2025-12-31", Revenue: 100},
		{ReportDate: "20250630", Revenue: 60},
		{ReportDate: "2024-12-31", Revenue: 80},
		{ReportDate: "2024-06-30", Revenue: 50},
	}
	if p := SamePeriodLastYear(stmts, "2025-12-31"); p == nil || p.Revenue != 80 {
		t.Fatalf("FY base = %v", p)
	}
	if p := SamePeriodLastYear(stmts, "20250630"); p == nil || p.Revenue != 50 {
		t.Fatalf("H1 base = %v", p)
	}
	if p := SamePeriodLastYear(stmts, "2026-03-31"); p != nil {
		t.Fatalf("missing period must be nil, got %v", p)
	}
	if p := SamePeriodLastYear(stmts, "2025"); p != nil {
		t.Fatalf("malformed date must be nil, got %v", p)
	}
}

func TestComputeMetricsDriverConcentration(t *testing.T) {
	stmts := []Statement{{ReportDate: "2025-12-31", Revenue: 1000}}
	drivers := []RevenueDriver{
		{ReportDate: "2025-12-31", Item: "Core", IncomeRatio: 0.60, Rank: 1},
		{ReportDate: "2025-12-31", Item: "Side", IncomeRatio: 0.25, Rank: 2},
		{ReportDate: "2025-12-31", Item: "Minor", IncomeRatio: 0.10, Rank: 3},
		{ReportDate: "2025-12-31", Item: "Tail", IncomeRatio: 0.05, Rank: 4},
	}
	m := computeMetrics(stmts, drivers)
	if m.TopDriver != "Core" {
		t.Fatalf("top driver: %q", m.TopDriver)
	}
	approx(t, m.TopDriverShare, 0.60, 1e-12)
	approx(t, m.RevenueConcentration, 0.95, 1e-12)
}

func TestComputeMetricsPicksLargestDriver(t *testing.T) {
	// Drivers arrive ordered by rank, which need not be ordered by share.
	stmts := []Statement{{ReportDate: "2025-12-31", Revenue: 1000}}
	drivers := []RevenueDriver{
		{ReportDate: "2025-12-31", Item: "Small", IncomeRatio: 0.20, Rank: 1},
		{ReportDate: "2025-12-31", Item: "Big", IncomeRatio: 0.80, Rank: 2},
	}
	m := computeMetrics(stmts, drivers)
	if m.TopDriver != "Big" {
		t.Fatalf("top driver must be the largest share, got %q", m.TopDriver)
	}
	approx(t, m.TopDriverShare, 0.80, 1e-12)
	approx(t, m.RevenueConcentration, 1.0, 1e-12)
}

func TestTableInterface(t *testing.T) {
	if got := tableInterface("fin_income"); got != "eastmoney_financial_income" {
		t.Fatalf("tableInterface(fin_income) = %q", got)
	}
	if got := tableInterface("valuation_snapshot"); got != "eastmoney_valuation_snapshot" {
		t.Fatalf("tableInterface(valuation_snapshot) = %q", got)
	}
	if got := tableInterface("no_such_table"); got != "" {
		t.Fatalf("unknown table must map to empty string, got %q", got)
	}
}

func approx(t *testing.T, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Fatalf("got %.12f, want %.12f (tol %.12f)", got, want, tol)
	}
}
