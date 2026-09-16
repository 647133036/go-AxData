// Package chart renders candlestick data with technical indicators as either
// terminal ASCII art or a standalone single-file SVG.
package chart

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// ChartData carries everything needed to draw a main candlestick pane plus
// RSI and MACD sub-panes.
type ChartData struct {
	Code   string
	Name   string
	Dates  []string
	Open   []float64
	High   []float64
	Low    []float64
	Close  []float64
	Volume []float64

	MA5  []float64
	MA10 []float64
	MA20 []float64
	RSI  []float64
	DIF  []float64
	DEA  []float64
	HIST []float64
}

// Len reports the number of bars.
func (c *ChartData) Len() int {
	if c == nil {
		return 0
	}
	return len(c.Close)
}

// Series returns one of the named indicator arrays, empty when unavailable.
func (c *ChartData) Series(name string) []float64 {
	switch name {
	case "MA5":
		return c.MA5
	case "MA10":
		return c.MA10
	case "MA20":
		return c.MA20
	case "RSI":
		return c.RSI
	case "DIF":
		return c.DIF
	case "DEA":
		return c.DEA
	case "HIST":
		return c.HIST
	default:
		return nil
	}
}

// DetailTable returns the last limit rows of OHLC plus indicator values.
func (c *ChartData) DetailTable(limit int) []map[string]string {
	if c == nil || limit <= 0 {
		return nil
	}
	start := c.Len() - limit
	if start < 0 {
		start = 0
	}
	var rows []map[string]string
	for i := start; i < c.Len(); i++ {
		rows = append(rows, map[string]string{
			"date":   c.date(i),
			"open":   fmtF(c.Open, i),
			"high":   fmtF(c.High, i),
			"low":    fmtF(c.Low, i),
			"close":  fmtF(c.Close, i),
			"volume": fmtInt(c.Volume, i),
			"ma5":    fmtF(c.MA5, i),
			"ma10":   fmtF(c.MA10, i),
			"ma20":   fmtF(c.MA20, i),
			"rsi":    fmtF(c.RSI, i),
			"dif":    fmtF(c.DIF, i),
			"dea":    fmtF(c.DEA, i),
			"hist":   fmtF(c.HIST, i),
		})
	}
	return rows
}

func (c *ChartData) date(i int) string {
	if c.Dates != nil && i < len(c.Dates) && c.Dates[i] != "" {
		return c.Dates[i]
	}
	return fmt.Sprintf("bar%d", i+1)
}

func fmtF(s []float64, i int) string {
	if s == nil || i >= len(s) || math.IsNaN(s[i]) {
		return "N/A"
	}
	return fmt.Sprintf("%.2f", s[i])
}

func fmtInt(s []float64, i int) string {
	if s == nil || i >= len(s) || math.IsNaN(s[i]) {
		return "N/A"
	}
	return fmt.Sprintf("%.0f", s[i])
}

// sparkRamp maps a relative height to a Unicode block glyph.
var sparkRamp = []rune(" ▁▂▃▄▅▆▇█")

func barGlyph(frac float64) rune {
	if math.IsNaN(frac) {
		return ' '
	}
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	idx := int(frac * float64(len(sparkRamp)-1))
	return sparkRamp[idx]
}

// RenderASCII draws the chart into a fixed-size text grid.
func RenderASCII(data *ChartData, rows, cols int) string {
	if data == nil || data.Len() == 0 {
		return "no data to render"
	}
	if rows < 12 {
		rows = 12
	}
	if cols < 40 {
		cols = 40
	}

	n := data.Len()
	if n > cols {
		step := n / cols
		if step < 1 {
			step = 1
		}
		sub := make([]int, 0, cols)
		for i := 0; i < n; i += step {
			sub = append(sub, i)
		}
		n = cols
		data = subsample(data, sub)
	}

	priceRows := rows - 9
	if priceRows < 6 {
		priceRows = 6
	}

	high, low := priceRange(data)
	if high == low {
		high, low = high+1, low-1
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s %s  %d bars  high %.2f  low %.2f  last %.2f\n",
		data.Code, data.Name, len(data.Close), high, low, data.Close[len(data.Close)-1]))
	b.WriteString(strings.Repeat("─", cols+2))
	b.WriteString("\n")

	for r := 0; r < priceRows; r++ {
		level := high - (high-low)*float64(r)/float64(priceRows-1)
		fmt.Fprintf(&b, "%8.2f │", level)
		for i := 0; i < n; i++ {
			// The bar occupies every row between its high and low, so a row
			// at `level` is filled when the level falls inside that range.
			if data.Low[i] <= level && data.High[i] >= level {
				if data.Close[i] >= data.Open[i] {
					b.WriteString("#")
					continue
				}
				b.WriteString("~")
				continue
			}
			b.WriteString(" ")
		}
		b.WriteString("\n")
	}

	b.WriteString(renderSparkLine("MA5 ", data.MA5, low, high, cols))
	b.WriteString(renderSparkLine("MA10", data.MA10, low, high, cols))
	b.WriteString(renderSparkLine("MA20", data.MA20, low, high, cols))

	b.WriteString("\n")
	b.WriteString(renderSparkLine("RSI ", data.RSI, 0, 100, cols))

	b.WriteString("\n")
	b.WriteString(renderHistogram("MACD", data.HIST, cols))

	return b.String()
}

func renderSparkLine(label string, series []float64, lo, hi float64, cols int) string {
	if series == nil {
		return ""
	}
	span := hi - lo
	if span <= 0 {
		span = 1
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s ", label))
	for i := 0; i < len(series) && i < cols; i++ {
		if math.IsNaN(series[i]) {
			b.WriteRune(' ')
			continue
		}
		frac := (series[i] - lo) / span
		b.WriteRune(barGlyph(frac))
	}
	b.WriteString("\n")
	return b.String()
}

func renderHistogram(label string, series []float64, cols int) string {
	if series == nil {
		return ""
	}
	mag := 0.0
	for _, v := range series {
		if math.IsNaN(v) {
			continue
		}
		if math.Abs(v) > mag {
			mag = math.Abs(v)
		}
	}
	if mag <= 0 {
		return fmt.Sprintf("%s %s\n", label, strings.Repeat("·", cols))
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s ", label))
	for i := 0; i < len(series) && i < cols; i++ {
		if math.IsNaN(series[i]) {
			b.WriteRune(' ')
			continue
		}
		frac := math.Abs(series[i]) / mag
		if series[i] >= 0 {
			b.WriteRune(barGlyph(frac))
			continue
		}
		b.WriteRune('▂')
	}
	b.WriteString("\n")
	return b.String()
}

func priceRange(data *ChartData) (float64, float64) {
	high, low := math.Inf(-1), math.Inf(1)
	for i := range data.Close {
		if data.High[i] > high {
			high = data.High[i]
		}
		if data.Low[i] < low {
			low = data.Low[i]
		}
		for _, s := range [][]float64{data.MA5, data.MA10, data.MA20} {
			if s == nil || i >= len(s) || math.IsNaN(s[i]) || s[i] <= 0 {
				continue
			}
			if s[i] > high {
				high = s[i]
			}
			if s[i] < low {
				low = s[i]
			}
		}
	}
	if math.IsInf(high, -1) {
		high = 0
	}
	if math.IsInf(low, 1) {
		low = 0
	}
	return high, low
}

func subsample(data *ChartData, idx []int) *ChartData {
	out := &ChartData{Code: data.Code, Name: data.Name}
	pick := func(s []float64) []float64 {
		if s == nil {
			return nil
		}
		var o []float64
		for _, i := range idx {
			if i < len(s) {
				o = append(o, s[i])
			}
		}
		return o
	}
	for _, i := range idx {
		if i < len(data.Dates) {
			out.Dates = append(out.Dates, data.Dates[i])
		}
	}
	out.Open, out.High, out.Low, out.Close, out.Volume =
		pick(data.Open), pick(data.High), pick(data.Low), pick(data.Close), pick(data.Volume)
	out.MA5, out.MA10, out.MA20 = pick(data.MA5), pick(data.MA10), pick(data.MA20)
	out.RSI, out.DIF, out.DEA, out.HIST = pick(data.RSI), pick(data.DIF), pick(data.DEA), pick(data.HIST)
	return out
}

// RenderSVG returns a standalone single-file SVG document with inline styles
// and no external resources.
func RenderSVG(data *ChartData, width, height int) string {
	if data == nil || data.Len() == 0 {
		return `<svg xmlns="http://www.w3.org/2000/svg" width="400" height="60"><text x="10" y="30">no data</text></svg>`
	}
	if width < 600 {
		width = 1000
	}
	if height < 400 {
		height = 620
	}

	n := data.Len()
	marginL, marginR, marginT, marginB := 60, 16, 40, 64
	mainH := int(float64(height) * 0.56)
	subH := int(float64(height-marginT-marginB-mainH) * 0.5)
	gap := 12

	high, low := priceRange(data)
	if high == low {
		high, low = high+1, low-1
	}

	plotW := width - marginL - marginR
	barW := float64(plotW) / float64(n)
	if barW < 2 {
		barW = 2
	}

	xOf := func(i int) float64 {
		return float64(marginL) + float64(i)*barW + barW/2
	}
	yPrice := func(v float64) float64 {
		return float64(marginT) + (high-v)/(high-low)*float64(mainH)
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`,
		width, height, width, height))
	b.WriteString(`<style>text{font:11px ui-monospace,monospace;fill:#333}.t{font-size:14px;font-weight:600;fill:#111}
.grid{stroke:#e8e8ec;stroke-width:1}.axis{stroke:#c9c9d1}.up{fill:#d64545}.down{fill:#2e9e5b}.wl{stroke:#888;fill:none;stroke-width:1.2}</style>`)

	title := data.Code
	if data.Name != "" {
		title = data.Code + " " + data.Name
	}
	b.WriteString(fmt.Sprintf(`<text class="t" x="%d" y="24">%s</text>`, marginL, escapeXML(title)))

	// price gridlines
	for g := 0; g <= 4; g++ {
		v := high - (high-low)*float64(g)/4
		y := yPrice(v)
		b.WriteString(fmt.Sprintf(`<line class="grid" x1="%d" y1="%.1f" x2="%d" y2="%.1f"/>`,
			marginL, y, width-marginR, y))
		b.WriteString(fmt.Sprintf(`<text x="4" y="%.1f">%s</text>`, y+4, fmt.Sprintf("%.2f", v)))
	}

	// candles
	for i := range data.Close {
		x := xOf(i)
		yO, yC := yPrice(data.Open[i]), yPrice(data.Close[i])
		yH, yL := yPrice(data.High[i]), yPrice(data.Low[i])
		b.WriteString(fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#999"/>`, x, yH, x, yL))
		top, h := min(yO, yC), math.Abs(yC-yO)
		if h < 1 {
			h = 1
		}
		cls := "up"
		if data.Close[i] < data.Open[i] {
			cls = "down"
		}
		b.WriteString(fmt.Sprintf(`<rect class="%s" x="%.1f" y="%.1f" width="%.1f" height="%.1f"/>`,
			cls, x-barW/2+0.5, top, barW-1, h))
	}

	// moving averages
	drawLine := func(series []float64, color string) {
		if series == nil {
			return
		}
		var pts []string
		for i := range data.Close {
			if i >= len(series) {
				continue
			}
			if math.IsNaN(series[i]) {
				continue
			}
			pts = append(pts, fmt.Sprintf("%.1f,%.1f", xOf(i), yPrice(series[i])))
		}
		if len(pts) > 1 {
			b.WriteString(fmt.Sprintf(`<polyline class="wl" stroke="%s" points="%s"/>`, color, strings.Join(pts, " ")))
		}
	}
	drawLine(data.MA5, "#e6a23c")
	drawLine(data.MA10, "#409eff")
	drawLine(data.MA20, "#9b59b6")

	legend := []struct {
		c, t string
	}{
		{"#e6a23c", "MA5"}, {"#409eff", "MA10"}, {"#9b59b6", "MA20"},
	}
	lx := float64(width-marginR) - 150
	for i, l := range legend {
		lxx := lx + float64(i)*52
		b.WriteString(fmt.Sprintf(`<rect x="%.1f" y="%.1f" width="12" height="2" fill="%s"/>`, lxx, float64(marginT)-14, l.c))
		b.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f">%s</text>`, lxx+16, float64(marginT)-11, l.t))
	}

	// x-axis labels
	step := n / 6
	if step < 1 {
		step = 1
	}
	b.WriteString(fmt.Sprintf(`<line class="axis" x1="%d" y1="%.1f" x2="%d" y2="%.1f"/>`,
		marginL, float64(marginT+mainH), width-marginR, float64(marginT+mainH)))
	for i := 0; i < n; i += step {
		b.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f">%s</text>`,
			xOf(i)-24, float64(marginT+mainH)+16, escapeXML(data.date(i))))
	}

	// RSI sub-pane
	rsiTop := marginT + mainH + gap + 16
	drawSubPane(&b, "RSI(14)", data.RSI, 0, 100, rsiTop, subH, xOf, barW, []float64{30, 70})

	// MACD sub-pane
	macdTop := rsiTop + subH + gap + 8
	mag := 0.0
	for _, v := range data.HIST {
		if math.Abs(v) > mag {
			mag = math.Abs(v)
		}
	}
	if mag <= 0 {
		mag = 1
	}
	drawSubPane(&b, "MACD(12,26,9)", data.DIF, -mag, mag, macdTop, subH, xOf, barW, []float64{0})
	drawSubPane(&b, "", data.DEA, -mag, mag, macdTop, subH, xOf, barW, nil)

	b.WriteString(`</svg>`)
	return b.String()
}

func drawSubPane(b *strings.Builder, label string, series []float64, lo, hi float64,
	top, h int, xOf func(int) float64, barW float64, marks []float64) {
	if hi == lo {
		hi, lo = hi+1, lo-1
	}
	if label != "" {
		fmt.Fprintf(b, `<text class="t" x="60" y="%d">%s</text>`, top, label)
	}
	_ = barW
	zero := func(v float64) float64 {
		return float64(top) + (hi-v)/(hi-lo)*float64(h)
	}
	for _, m := range marks {
		fmt.Fprintf(b, `<line class="grid" x1="60" y1="%.1f" x2="984" y2="%.1f"/>`, zero(m), zero(m))
	}
	if series == nil {
		return
	}
	var pts []string
	for i, v := range series {
		if math.IsNaN(v) {
			continue
		}
		pts = append(pts, fmt.Sprintf("%.1f,%.1f", xOf(i), zero(v)))
	}
	if len(pts) > 1 {
		fmt.Fprintf(b, `<polyline class="wl" stroke="#555" points="%s"/>`, strings.Join(pts, " "))
	}
}

func escapeXML(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")
	return r.Replace(s)
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// SortedDates returns a copy of dates sorted ascending, for callers that need
// stable ordering independent of source ordering.
func SortedDates(dates []string) []string {
	out := append([]string(nil), dates...)
	sort.Strings(out)
	return out
}
