package earnings

import (
	"testing"
)

func TestForecastTypeClassification(t *testing.T) {
	cases := []struct {
		typeCode string
		item     string
		want     ForecastType
	}{
		{"", "预亏", TypePreLoss},
		{"", "扭亏", TypePreTurn},
		{"", "预增", TypePreBigIncrease},
		{"", "预减", TypePreBigDecrease},
		{"", "略增", TypePreIncrease},
		{"", "略减", TypePreReduce},
		{"", "预盈", TypePreProfit},
		{"", "无关文本", TypeUnknown},
		{"预告类型X", "", TypeUnknown},
	}
	for _, c := range cases {
		if got := classifyForecast(c.typeCode, c.item); got != c.want {
			t.Fatalf("classifyForecast(%q,%q) = %v, want %v", c.typeCode, c.item, got, c.want)
		}
	}
}

func TestForecastTypeLabels(t *testing.T) {
	if TypePreLoss.String() == "" {
		t.Fatal("loss label must not be empty")
	}
	if TypeUnknown.String() == "" {
		t.Fatal("unknown label must not be empty")
	}
}

func TestVerdictLabels(t *testing.T) {
	seen := map[string]bool{}
	for _, v := range []Verdict{VerdictBeat, VerdictMet, VerdictMiss, VerdictNoForecast, VerdictUnavailable} {
		label := v.String()
		if label == "" {
			t.Fatalf("verdict %d must have a label", v)
		}
		if seen[label] {
			t.Fatalf("duplicate label %q", label)
		}
		seen[label] = true
	}
}

func TestBeatPct(t *testing.T) {
	if got := beatPct(100, 50); got != 1 {
		t.Fatalf("beatPct(100,50) = %f, want 1", got)
	}
	if got := beatPct(45, 50); got != -0.1 {
		t.Fatalf("beatPct(45,50) = %f, want -0.1", got)
	}
	if got := beatPct(100, 0); got != 0 {
		t.Fatalf("beatPct with zero midpoint must be 0, got %f", got)
	}
}

func TestJudgeBuckets(t *testing.T) {
	if got := judge(0.2); got != VerdictBeat {
		t.Fatalf("judge(0.2) = %v", got)
	}
	if got := judge(0.05); got != VerdictMet {
		t.Fatalf("judge(0.05) must be at the boundary, got %v", got)
	}
	if got := judge(0); got != VerdictMet {
		t.Fatalf("judge(0) = %v", got)
	}
	if got := judge(-0.05); got != VerdictMet {
		t.Fatalf("judge(-0.05) must be at the boundary, got %v", got)
	}
	if got := judge(-0.2); got != VerdictMiss {
		t.Fatalf("judge(-0.2) = %v", got)
	}
}

func TestPeriodLabel(t *testing.T) {
	cases := map[string]string{
		"2025-03-31": "Q1",
		"2025/06/30": "H1",
		"2025-09-30": "Q3",
		"2025-12-31": "FY",
		"20250331":   "Q1",
		"20250630":   "H1",
		"20250930":   "Q3",
		"20251231":   "FY",
		"20250115":   "0115",
		"":           "",
		"n/a":        "n/a",
	}
	for in, want := range cases {
		if got := PeriodLabel(in); got != want {
			t.Fatalf("PeriodLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFirstCode(t *testing.T) {
	if got := firstCode("", "", "000001.SZ"); got != "000001.SZ" {
		t.Fatalf("firstCode = %q", got)
	}
	if got := firstCode(); got != "" {
		t.Fatalf("firstCode with no args = %q", got)
	}
}

func TestProfitItem(t *testing.T) {
	if !profitItem("归属于上市公司股东的净利润") {
		t.Fatal("net profit must be a total-profit item")
	}
	if !profitItem("营业收入") {
		t.Fatal("revenue must be a total-profit item")
	}
	if profitItem("每股收益") {
		t.Fatal("EPS is per-share, not a total-profit item")
	}
}

func TestActualFor(t *testing.T) {
	full := periodActuals{Revenue: 900, NetProfit: 450, Has: true}
	absent := periodActuals{}

	if v, ok := actualFor("营业收入", full); !ok || v != 900 {
		t.Fatalf("actualFor(营业收入) = %v,%v want 900,true", v, ok)
	}
	if v, ok := actualFor("归属于上市公司股东的净利润", full); !ok || v != 450 {
		t.Fatalf("actualFor(net profit) = %v,%v want 450,true", v, ok)
	}
	if _, ok := actualFor("营业收入", absent); ok {
		t.Fatal("actualFor must report absent periods")
	}
	if _, ok := actualFor("每股收益", full); ok {
		t.Fatal("actualFor must not match EPS against a reported line")
	}
	if _, ok := actualFor("营业收入", periodActuals{Revenue: 0, NetProfit: 450, Has: true}); ok {
		t.Fatal("actualFor must not report a zero revenue as a match")
	}
}
