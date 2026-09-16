// Package portfolio analyses portfolio diversification and concentration.
package portfolio

import (
	"fmt"
	"math"
	"sort"
)

// ErrTooFewAssets is returned when the portfolio has fewer than two assets.
var ErrTooFewAssets = fmt.Errorf("portfolio: at least two assets required")

// ErrShortHistory is returned when the return series is too short.
var ErrShortHistory = fmt.Errorf("portfolio: return series too short")

const minObservations = 20

// Holding is one position with a percentage weight.
type Holding struct {
	Code   string
	Weight float64
}

// AssetPair records a high-correlation pair flagged for redundancy.
type AssetPair struct {
	A, B        string
	Correlation float64
}

// RiskContribution reports how much one asset contributes to portfolio risk.
type RiskContribution struct {
	Code         string
	Weight       float64
	Volatility   float64
	Contribution float64
	Percentage   float64
}

// Result is the full diversification analysis output.
type Result struct {
	Codes                []string
	CorrelationMatrix    [][]float64
	VolatilityMatrix     [][]float64
	PortfolioVolatility  float64
	RiskContributions    []RiskContribution
	HHI                  float64
	DiversificationRatio float64
	HighCorrelationPairs []AssetPair
	Recommendations      []string
}

const (
	highCorrelationThreshold = 0.8
	maxSingleWeight          = 0.30
	maxRecommendations       = 5
)

// Analyze computes correlation, risk contribution and concentration metrics.
func Analyze(returns map[string][]float64, holdings []Holding) (*Result, error) {
	if len(holdings) < 2 {
		return nil, ErrTooFewAssets
	}

	codes := make([]string, 0, len(holdings))
	seen := map[string]bool{}
	weights := map[string]float64{}
	total := 0.0
	for _, h := range holdings {
		if _, ok := returns[h.Code]; !ok {
			return nil, fmt.Errorf("portfolio: no return data for %s", h.Code)
		}
		if seen[h.Code] {
			return nil, fmt.Errorf("portfolio: duplicate holding %s", h.Code)
		}
		seen[h.Code] = true
		codes = append(codes, h.Code)
		weights[h.Code] = h.Weight
		total += h.Weight
	}
	if total <= 0 {
		return nil, fmt.Errorf("portfolio: total weight must be positive, got %v", total)
	}
	for i := range codes {
		weights[codes[i]] /= total
	}
	sort.Strings(codes)

	series := make(map[string][]float64)
	for _, c := range codes {
		s := returns[c]
		if len(s) < minObservations {
			return nil, fmt.Errorf("%w: %s has %d observations, need %d",
				ErrShortHistory, c, len(s), minObservations)
		}
		series[c] = s
	}

	vols := make(map[string]float64)
	for _, c := range codes {
		vols[c] = volatility(series[c])
	}

	corr := makeCorrelationMatrix(codes, series)
	volM := makeVarianceMatrix(codes, vols, corr)

	portVol := portfolioVolatility(codes, weights, volM)
	res := &Result{
		Codes:               codes,
		CorrelationMatrix:   corr,
		VolatilityMatrix:    volM,
		PortfolioVolatility: portVol,
	}
	res.RiskContributions = riskContributions(codes, weights, vols, volM, portVol)
	res.HHI = hhi(codes, weights)
	res.DiversificationRatio = diversificationRatio(codes, weights, vols, portVol)
	res.HighCorrelationPairs = highPairs(codes, corr)
	res.Recommendations = buildRecommendations(res)
	return res, nil
}

func volatility(series []float64) float64 {
	n := len(series)
	mean := 0.0
	for _, v := range series {
		mean += v
	}
	mean /= float64(n)
	var sq float64
	for _, v := range series {
		d := v - mean
		sq += d * d
	}
	return math.Sqrt(sq / float64(n-1))
}

func pearson(a, b []float64) float64 {
	n := len(a)
	if n > len(b) {
		n = len(b)
	}
	if n < 2 {
		return 0
	}
	var ma, mb float64
	for i := 0; i < n; i++ {
		ma += a[i]
		mb += b[i]
	}
	ma /= float64(n)
	mb /= float64(n)
	var num, da, db float64
	for i := 0; i < n; i++ {
		x := a[i] - ma
		y := b[i] - mb
		num += x * y
		da += x * x
		db += y * y
	}
	if da <= 0 || db <= 0 {
		return 0
	}
	return num / math.Sqrt(da*db)
}

func makeCorrelationMatrix(codes []string, series map[string][]float64) [][]float64 {
	n := len(codes)
	m := make([][]float64, n)
	for i := 0; i < n; i++ {
		m[i] = make([]float64, n)
		for j := 0; j < n; j++ {
			if i == j {
				m[i][j] = 1
				continue
			}
			if j < i {
				m[i][j] = m[j][i]
				continue
			}
			m[i][j] = pearson(series[codes[i]], series[codes[j]])
		}
	}
	return m
}

func makeVarianceMatrix(codes []string, vols map[string]float64, corr [][]float64) [][]float64 {
	n := len(codes)
	m := make([][]float64, n)
	for i := 0; i < n; i++ {
		m[i] = make([]float64, n)
		for j := 0; j < n; j++ {
			m[i][j] = vols[codes[i]] * vols[codes[j]] * corr[i][j]
		}
	}
	return m
}

func portfolioVolatility(codes []string, weights map[string]float64, volM [][]float64) float64 {
	sum := 0.0
	for _, a := range codes {
		for _, b := range codes {
			ai, bi := indexOf(codes, a), indexOf(codes, b)
			sum += weights[a] * weights[b] * volM[ai][bi]
		}
	}
	if sum < 0 {
		return 0
	}
	return math.Sqrt(sum)
}

func riskContributions(codes []string, weights map[string]float64,
	vols map[string]float64, volM [][]float64, portVol float64) []RiskContribution {
	// Risk contribution is w_c * (Σw)_c, so it must sum to portfolio variance,
	// not volatility. Dividing by variance is what makes the shares total 100%.
	variance := portVol * portVol
	out := make([]RiskContribution, 0, len(codes))
	for _, c := range codes {
		w := weights[c]
		var cov float64
		for _, b := range codes {
			cov += weights[b] * volM[indexOf(codes, c)][indexOf(codes, b)]
		}
		contribution := w * cov
		pct := 0.0
		if variance > 0 {
			pct = contribution / variance
		}
		out = append(out, RiskContribution{
			Code: c, Weight: w, Volatility: vols[c],
			Contribution: contribution, Percentage: pct,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Percentage > out[j].Percentage })
	return out
}

func indexOf(codes []string, code string) int {
	for i, c := range codes {
		if c == code {
			return i
		}
	}
	return 0
}

// hhi returns the Herfindahl index on a 1..10000 scale.
func hhi(codes []string, weights map[string]float64) float64 {
	sum := 0.0
	for _, c := range codes {
		sum += weights[c] * weights[c]
	}
	return sum * 10000
}

func diversificationRatio(codes []string, weights map[string]float64,
	vols map[string]float64, portVol float64) float64 {
	weighted := 0.0
	for _, c := range codes {
		weighted += weights[c] * vols[c]
	}
	if weighted <= 0 {
		return 0
	}
	return portVol / weighted
}

func highPairs(codes []string, corr [][]float64) []AssetPair {
	var pairs []AssetPair
	n := len(codes)
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if corr[i][j] > highCorrelationThreshold {
				pairs = append(pairs, AssetPair{A: codes[i], B: codes[j], Correlation: corr[i][j]})
			}
		}
	}
	return pairs
}

func buildRecommendations(r *Result) []string {
	var recs []string
	add := func(s string) {
		if len(recs) >= maxRecommendations {
			return
		}
		recs = append(recs, s)
	}

	n := len(r.Codes)
	if len(r.HighCorrelationPairs) > 0 {
		p := r.HighCorrelationPairs[0]
		add(fmt.Sprintf("降低 %s 与 %s 之一（相关系数 %.2f），二者提供重叠风险敞口", p.A, p.B, p.Correlation))
	}

	if r.HHI >= 5000 {
		add(fmt.Sprintf("集中度 HHI=%.0f，组合过度集中，建议引入新资产类别", r.HHI))
	} else if r.HHI >= 2500 {
		add(fmt.Sprintf("集中度 HHI=%.0f，处于中等集中区间，适度分散可降低非系统性风险", r.HHI))
	}

	if r.DiversificationRatio > 0.95 && r.DiversificationRatio < 1 {
		add(fmt.Sprintf("分散化比率 %.2f 接近 1，组合已充分分散，进一步调整边际收益有限", r.DiversificationRatio))
	}

	if len(r.RiskContributions) > 0 {
		top := r.RiskContributions[0]
		if top.Percentage > 0.5 {
			add(fmt.Sprintf("%s 贡献 %.1f%% 的组合风险，是最大单一风险来源", top.Code, top.Percentage*100))
		}
	}

	if r.HHI < 2500 {
		add(fmt.Sprintf("组合共 %d 个资产，分散度健康，维持当前配置", n))
	}

	if len(recs) == 0 {
		add(fmt.Sprintf("组合共 %d 个资产，各资产风险贡献均衡", n))
	}
	return recs
}

// MaxSingleWeight returns the single largest weight and whether it breaches
// the concentration limit.
func MaxSingleWeight(weights map[string]float64) (string, float64, bool) {
	best, bestW := "", 0.0
	for c, w := range weights {
		if w > bestW {
			best, bestW = c, w
		}
	}
	return best, bestW, bestW > maxSingleWeight
}

// MaxSingleWeightLimit returns the concentration threshold applied by
// MaxSingleWeight, as a fraction. Callers formatting it for display convert it
// to a percentage themselves.
func MaxSingleWeightLimit() float64 {
	return maxSingleWeight
}
