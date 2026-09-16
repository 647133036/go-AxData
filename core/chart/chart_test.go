package chart

import (
	"math"
	"strings"
	"testing"
)

// sample builds n deterministic bars with monotonically rising close prices.
func sample(n int) *ChartData {
	d := &ChartData{Code: "000001.SZ", Name: "Test"}
	dates := make([]string, n)
	for i := range dates {
		dates[i] = "2025"
	}
	d.Dates = dates
	d.Open = make([]float64, n)
	d.High = make([]float64, n)
	d.Low = make([]float64, n)
	d.Close = make([]float64, n)
	d.Volume = make([]float64, n)
	d.MA5 = make([]float64, n)
	d.RSI = make([]float64, n)
	d.DIF = make([]float64, n)
	d.DEA = make([]float64, n)
	d.HIST = make([]float64, n)
	for i := 0; i < n; i++ {
		c := 10 + float64(i)*0.05
		d.Close[i] = c
		d.Open[i] = c - 0.1
		d.High[i] = c + 0.2
		d.Low[i] = c - 0.2
		d.Volume[i] = 1e6 + float64(i)*1e4
		d.MA5[i] = c - 0.1
		d.RSI[i] = 50 + float64(i%20)
		d.DIF[i] = 0.01 * float64(i)
		d.DEA[i] = 0.01*float64(i) - 0.02
		d.HIST[i] = 2 * (d.DIF[i] - d.DEA[i])
	}
	return d
}

func TestLenAndSeries(t *testing.T) {
	d := sample(30)
	if d.Len() != 30 {
		t.Fatalf("Len: %d", d.Len())
	}
	if (*ChartData)(nil).Len() != 0 {
		t.Fatal("nil chart must report length 0")
	}
	if len(d.Series("MA5")) != 30 {
		t.Fatal("Series MA5 missing")
	}
	if len(d.Series("RSI")) != 30 {
		t.Fatal("Series RSI missing")
	}
	if d.Series("NOSUCH") != nil {
		t.Fatal("unknown series must return nil")
	}
}

func TestDetailTable(t *testing.T) {
	d := sample(30)
	rows := d.DetailTable(5)
	if len(rows) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(rows))
	}
	if rows[4]["date"] != d.Dates[29] {
		t.Fatalf("last row date mismatch: %q", rows[4]["date"])
	}
	if rows[0]["date"] != d.Dates[25] {
		t.Fatalf("first row should start at index 25, got %q", rows[0]["date"])
	}
	if rows[4]["close"] != "11.45" {
		t.Fatalf("close format: %q", rows[4]["close"])
	}
	if rows[4]["volume"] == "" {
		t.Fatal("volume should be populated")
	}

	overLimit := d.DetailTable(100)
	if len(overLimit) != 30 {
		t.Fatalf("limit beyond length should clamp, got %d rows", len(overLimit))
	}
	if d.DetailTable(0) != nil {
		t.Fatal("non-positive limit must return nil")
	}
	if (*ChartData)(nil).DetailTable(5) != nil {
		t.Fatal("nil chart must return nil table")
	}
}

func TestDetailTableMissingIndicators(t *testing.T) {
	d := &ChartData{
		Code:   "000001.SZ",
		Dates:  []string{"2025-01-01"},
		Open:   []float64{10},
		High:   []float64{11},
		Low:    []float64{9},
		Close:  []float64{10.5},
		Volume: []float64{1e6},
	}
	rows := d.DetailTable(1)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0]["ma5"] != "N/A" || rows[0]["rsi"] != "N/A" || rows[0]["dif"] != "N/A" {
		t.Fatalf("missing indicators must render as N/A: %v", rows[0])
	}
	if rows[0]["close"] != "10.50" {
		t.Fatalf("close format: %q", rows[0]["close"])
	}
}

func TestRenderASCII(t *testing.T) {
	d := sample(60)
	out := RenderASCII(d, 30, 100)
	if out == "" {
		t.Fatal("ASCII render must not be empty")
	}
	lines := strings.Split(out, "\n")
	if len(lines) < 5 {
		t.Fatalf("expected a multi-line chart, got %d lines", len(lines))
	}
	if !strings.Contains(out, "000001") {
		t.Fatalf("expected the code in the header: %q", lines[0])
	}
	if !strings.Contains(out, "MA5") {
		t.Fatal("expected MA5 label in the output")
	}
	if !strings.Contains(out, "RSI") {
		t.Fatal("expected RSI label in the output")
	}
	// sample() builds rising bars, so the price pane must contain up candles.
	// Before the candle test was inverted this pane rendered as blank.
	if !strings.Contains(out, "#") {
		t.Fatalf("expected up candles in the price pane:\n%s", out)
	}
}

// TestRenderASCIIDownCandle ensures falling bars render as down candles and
// that the header range comes from the OHLC data, not from indicator warmup.
func TestRenderASCIIDownCandle(t *testing.T) {
	d := sample(20)
	for i := range d.Close {
		d.Open[i] = d.Close[i] + 0.1
	}
	out := RenderASCII(d, 30, 100)
	if !strings.Contains(out, "~") {
		t.Fatalf("expected down candles in the price pane:\n%s", out)
	}
	// close starts at 10 and rises 0.05/bar; high = close+0.2, low = close-0.2.
	if !strings.Contains(out, "high 11.15") || !strings.Contains(out, "low 9.80") {
		t.Fatalf("header range must come from OHLC:\n%s", out)
	}
}

// TestRenderASCIIScaleIgnoresWarmup guards against indicator warmup zeros
// collapsing the price axis to zero.
func TestRenderASCIIScaleIgnoresWarmup(t *testing.T) {
	d := sample(20)
	d.MA5 = make([]float64, len(d.Close))
	d.MA10 = make([]float64, len(d.Close))
	d.MA20 = make([]float64, len(d.Close))
	for i := range d.Close {
		d.MA5[i] = d.Close[i] - 0.1
		d.MA10[i] = 0 // unmasked warmup
		d.MA20[i] = 0
	}
	out := RenderASCII(d, 30, 100)
	if strings.Contains(out, "low 0.00") {
		t.Fatalf("price scale must not be pulled to zero by warmup slots:\n%s", out)
	}
	if !strings.Contains(out, "#") {
		t.Fatalf("expected candles in the price pane:\n%s", out)
	}
}

func TestRenderASCIINilAndEmpty(t *testing.T) {
	if out := RenderASCII(nil, 20, 80); out == "" {
		t.Fatal("nil chart must still produce a message")
	}
	if out := RenderASCII(&ChartData{}, 20, 80); out == "" {
		t.Fatal("empty chart must still produce a message")
	}
}

func TestRenderSVG(t *testing.T) {
	d := sample(60)
	out := RenderSVG(d, 1000, 700)
	if !strings.HasPrefix(out, "<svg") {
		t.Fatalf("SVG must start with <svg, got: %q", out)
	}
	if !strings.HasSuffix(out, "</svg>") {
		t.Fatal("SVG must end with </svg>")
	}
	for _, needle := range []string{`class="grid"`, "<polyline", "000001"} {
		if !strings.Contains(out, needle) {
			t.Fatalf("SVG missing %q", needle)
		}
	}
}

func TestRenderSVGNoExternalAssets(t *testing.T) {
	d := sample(30)
	out := RenderSVG(d, 800, 500)
	for _, forbidden := range []string{`<img`, `src="`, `<script`, `<link`, `@import`} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("SVG must be self-contained, found %q", forbidden)
		}
	}
	if strings.Count(out, "http://www.w3.org/2000/svg") != 1 {
		t.Fatal("only the SVG namespace URI is allowed")
	}
}

func TestRenderSVGNilSafe(t *testing.T) {
	if out := RenderSVG(nil, 800, 500); out == "" {
		t.Fatal("nil chart must still render a placeholder")
	}
}

func TestRenderSVGEscapesLabels(t *testing.T) {
	d := sample(10)
	d.Code = `A&B<C">`
	d.Name = `O'Brien & Co`
	out := RenderSVG(d, 800, 500)
	if strings.Contains(out, `A&B<C">`) {
		t.Fatal("special characters must be XML-escaped")
	}
	if !strings.Contains(out, "&amp;") {
		t.Fatal("expected &amp; escaping")
	}
}

func TestEscapeXML(t *testing.T) {
	cases := map[string]string{
		"a":     "a",
		"&":     "&amp;",
		"<":     "&lt;",
		">":     "&gt;",
		`"`:     "&quot;",
		"'":     "&apos;",
		`<a&b>`: "&lt;a&amp;b&gt;",
	}
	for in, want := range cases {
		if got := escapeXML(in); got != want {
			t.Fatalf("escapeXML(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSortedDates(t *testing.T) {
	in := []string{"20250301", "20250101", "20250201"}
	out := SortedDates(in)
	if len(out) != 3 || out[0] != "20250101" || out[2] != "20250301" {
		t.Fatalf("sort wrong: %v", out)
	}
	if in[0] != "20250301" {
		t.Fatal("SortedDates must not mutate the input")
	}
	if SortedDates(nil) != nil {
		t.Fatal("nil input must return nil")
	}
	if SortedDates([]string{}) != nil {
		t.Fatal("empty input must return nil")
	}
}

func TestBarGlyph(t *testing.T) {
	if barGlyph(0) != ' ' {
		t.Fatalf("fraction 0 must be blank, got %q", barGlyph(0))
	}
	if barGlyph(1) != '█' {
		t.Fatalf("fraction 1 must be full block, got %q", barGlyph(1))
	}
	if barGlyph(-1) != ' ' {
		t.Fatal("negative fraction must clamp to blank")
	}
	if barGlyph(2) != '█' {
		t.Fatal("fraction above 1 must clamp to full block")
	}
}

func TestPriceRange(t *testing.T) {
	d := sample(20)
	hi, lo := priceRange(d)
	if math.IsNaN(lo) || math.IsNaN(hi) {
		t.Fatalf("range must not be NaN: %f %f", lo, hi)
	}
	if lo > hi {
		t.Fatalf("low must not exceed high: %f > %f", lo, hi)
	}
	// close starts at 10 and rises 0.05 per bar; low = close-0.2, high = close+0.2.
	approx(t, hi, 11.15, 1e-9)
	approx(t, lo, 9.8, 1e-9)
}

func approx(t *testing.T, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Fatalf("got %.10f, want %.10f (tol %.10f)", got, want, tol)
	}
}
