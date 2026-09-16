package market

import (
	"fmt"
	"math"
	"testing"
	"time"
)

func bars(n int) []Kline {
	out := make([]Kline, n)
	for i := range out {
		out[i] = Kline{
			Date:   fmt.Sprintf("2025%02d%02d", i/28+1, i%28+1),
			Open:   10 + float64(i)*0.05,
			High:   10.2 + float64(i)*0.05,
			Low:    9.8 + float64(i)*0.05,
			Close:  10.1 + float64(i)*0.05,
			Volume: 1e6,
			Amount: 1e7,
		}
	}
	return out
}

func TestBuildChart(t *testing.T) {
	c, err := BuildChart("000001.SZ", "Test", bars(60))
	if err != nil {
		t.Fatal(err)
	}
	if c.Len() != 60 {
		t.Fatalf("length: %d", c.Len())
	}
	if c.Code != "000001.SZ" {
		t.Fatalf("code: %q", c.Code)
	}
	for _, name := range []string{"MA5", "MA10", "MA20", "RSI", "DIF", "DEA", "HIST"} {
		if got := c.Series(name); len(got) != 60 {
			t.Fatalf("series %s has %d values", name, len(got))
		}
	}
	// RSI must stay within [0,100] where defined.
	for _, v := range c.RSI {
		if !math.IsNaN(v) && (v < 0 || v > 100) {
			t.Fatalf("RSI out of range: %f", v)
		}
	}
	// MACD histogram must follow the 2*(DIF-DEA) convention.
	for i := range c.DIF {
		want := 2 * (c.DIF[i] - c.DEA[i])
		if math.Abs(c.HIST[i]-want) > 1e-12 {
			t.Fatalf("HIST[%d] = %f, want %f", i, c.HIST[i], want)
		}
	}
}

func TestBuildChartTooFewBars(t *testing.T) {
	if _, err := BuildChart("000001.SZ", "", bars(4)); err == nil {
		t.Fatal("expected error for fewer than 5 bars")
	}
	if _, err := BuildChart("000001.SZ", "", nil); err == nil {
		t.Fatal("expected error for empty bars")
	}
}

func TestBuildChartShortSeriesIndicators(t *testing.T) {
	// Fewer bars than RSI/MACD need: MA5 still computes, the others are absent.
	c, err := BuildChart("000001.SZ", "Test", bars(6))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.MA5) != 6 {
		t.Fatalf("MA5 length: %d", len(c.MA5))
	}
	// Absent indicators must be nil, so renderers skip the pane. A zero-filled
	// series would be drawn as a flat line and read as a real 0.00 value.
	for _, name := range []string{"MA10", "MA20", "RSI", "DIF", "DEA", "HIST"} {
		if got := c.Series(name); got != nil {
			t.Fatalf("series %s must be nil, got %d values", name, len(got))
		}
	}
}

// TestBuildChartMasksWarmup verifies that pre-window indicator slots are NaN
// rather than 0, otherwise they collapse the chart's price scale to zero.
func TestBuildChartMasksWarmup(t *testing.T) {
	c, err := BuildChart("000001.SZ", "Test", bars(60))
	if err != nil {
		t.Fatal(err)
	}
	assertWarmupNaN := func(series []float64, warmup int, name string) {
		t.Helper()
		for i := 0; i < warmup; i++ {
			if !math.IsNaN(series[i]) {
				t.Fatalf("%s[%d] must be NaN during warmup, got %f", name, i, series[i])
			}
		}
		for i := warmup; i < len(series); i++ {
			if math.IsNaN(series[i]) {
				t.Fatalf("%s[%d] must be defined after warmup, got NaN", name, i)
			}
		}
	}
	assertWarmupNaN(c.MA5, 4, "MA5")
	assertWarmupNaN(c.MA10, 9, "MA10")
	assertWarmupNaN(c.MA20, 19, "MA20")
	assertWarmupNaN(c.RSI, 14, "RSI")
	assertWarmupNaN(c.DIF, 25, "DIF")
	assertWarmupNaN(c.DEA, 33, "DEA")
	assertWarmupNaN(c.HIST, 33, "HIST")
}

func TestLastBarDate(t *testing.T) {
	if LastBarDate(nil) != "" {
		t.Fatal("empty bars must return empty date")
	}
	in := []Kline{{Date: "20250101"}, {Date: "20250315"}, {Date: "20250201"}}
	if got := LastBarDate(in); got != "20250315" {
		t.Fatalf("LastBarDate = %q", got)
	}
}

func TestBarsAsOf(t *testing.T) {
	in := []Kline{{Date: "20250101"}, {Date: "20250201"}, {Date: "20250301"}}
	got := BarsAsOf(in, "20250201")
	if len(got) != 2 {
		t.Fatalf("expected 2 bars as of 20250201, got %d", len(got))
	}
	gotAll := BarsAsOf(in, "")
	if len(gotAll) != 3 {
		t.Fatalf("empty asOf must return all bars, got %d", len(gotAll))
	}
}

func TestRecentTradeDay(t *testing.T) {
	monday := time.Date(2025, 9, 15, 10, 0, 0, 0, time.UTC)
	if got := RecentTradeDay(monday); got != "20250915" {
		t.Fatalf("weekday: %q", got)
	}
	saturday := time.Date(2025, 9, 13, 10, 0, 0, 0, time.UTC)
	if got := RecentTradeDay(saturday); got != "20250912" {
		t.Fatalf("saturday: %q", got)
	}
	sunday := time.Date(2025, 9, 14, 10, 0, 0, 0, time.UTC)
	if got := RecentTradeDay(sunday); got != "20250912" {
		t.Fatalf("sunday: %q", got)
	}
}

func TestNormalizeCode(t *testing.T) {
	if got := normalizeCode("000001.SZ"); got != "000001" {
		t.Fatalf("normalizeCode = %q", got)
	}
	if got := normalizeCode("000001"); got != "000001" {
		t.Fatalf("normalizeCode plain = %q", got)
	}
	if got := normalizeCode(""); got != "" {
		t.Fatalf("normalizeCode empty = %q", got)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	m := map[string]interface{}{"a": "", "b": "x", "c": nil}
	if got := firstNonEmpty(m, "a", "b"); got != "x" {
		t.Fatalf("firstNonEmpty = %q", got)
	}
	if got := firstNonEmpty(m, "missing", "also_missing"); got != "" {
		t.Fatalf("firstNonEmpty missing = %q", got)
	}
	if got := firstNonEmpty(nil, "a"); got != "" {
		t.Fatalf("firstNonEmpty nil = %q", got)
	}
}

func TestF64(t *testing.T) {
	cases := []struct {
		val  interface{}
		keys []string
		want float64
	}{
		{1.5, []string{"a"}, 1.5},
		{int(7), []string{"a"}, 7},
		{int64(9), []string{"a"}, 9},
		{float32(2.25), []string{"a"}, 2.25},
		{"3.5", []string{"a"}, 3.5},
		{nil, []string{"a"}, 0},
		{"not-a-number", []string{"a"}, 0},
		{"5", []string{"missing", "a"}, 5},
	}
	for i, c := range cases {
		m := map[string]interface{}{"a": c.val}
		if got := f64(m, c.keys...); math.Abs(got-c.want) > 1e-9 {
			t.Fatalf("case %d: got %f, want %f", i, got, c.want)
		}
	}
}

func TestMinBars(t *testing.T) {
	// limit/5 below the 30-bar floor scales down, at or above it the floor applies.
	if got := minBars(120); got != 24 {
		t.Fatalf("minBars(120) = %d, want 24", got)
	}
	if got := minBars(150); got != 30 {
		t.Fatalf("minBars(150) = %d, want 30", got)
	}
	if got := minBars(1000); got != 30 {
		t.Fatalf("minBars(1000) = %d, want 30", got)
	}
}
