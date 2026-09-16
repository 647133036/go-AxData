package portfolio

import (
	"errors"
	"math"
	"math/rand"
	"testing"
)

// rng returns a deterministic pseudo-random series so tests are reproducible.
func rng(seed int64, n int, mean, sd float64) []float64 {
	r := rand.New(rand.NewSource(seed))
	out := make([]float64, n)
	for i := range out {
		out[i] = mean + sd*r.NormFloat64()
	}
	return out
}

func TestAnalyzeBasic(t *testing.T) {
	n := 60
	rets := map[string][]float64{
		"A": rng(1, n, 0.001, 0.02),
		"B": rng(2, n, 0.001, 0.03),
		"C": rng(3, n, 0.001, 0.015),
	}
	held := []Holding{{"A", 0.4}, {"B", 0.35}, {"C", 0.25}}
	res, err := Analyze(rets, held)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Codes) != 3 {
		t.Fatalf("expected 3 codes, got %v", res.Codes)
	}
	if res.Codes[0] != "A" || res.Codes[1] != "B" || res.Codes[2] != "C" {
		t.Fatalf("codes not sorted: %v", res.Codes)
	}
	if len(res.CorrelationMatrix) != 3 || len(res.CorrelationMatrix[0]) != 3 {
		t.Fatalf("correlation matrix shape wrong: %v", res.CorrelationMatrix)
	}
	for i := range res.CorrelationMatrix {
		if math.Abs(res.CorrelationMatrix[i][i]-1) > 1e-12 {
			t.Fatalf("diagonal must be 1: %v", res.CorrelationMatrix[i])
		}
	}
	if res.PortfolioVolatility <= 0 {
		t.Fatalf("portfolio volatility must be positive: %f", res.PortfolioVolatility)
	}
	if len(res.RiskContributions) != 3 {
		t.Fatalf("expected 3 risk contributions, got %d", len(res.RiskContributions))
	}
	for i := 1; i < len(res.RiskContributions); i++ {
		if res.RiskContributions[i].Percentage > res.RiskContributions[i-1].Percentage {
			t.Fatalf("risk contributions not sorted descending")
		}
	}
}

func TestAnalyzeWeightsNormalized(t *testing.T) {
	n := 40
	rets := map[string][]float64{
		"A": rng(1, n, 0.001, 0.02),
		"B": rng(2, n, 0.001, 0.02),
	}
	res, err := Analyze(rets, []Holding{{"A", 60}, {"B", 40}})
	if err != nil {
		t.Fatal(err)
	}
	var sum float64
	for _, rc := range res.RiskContributions {
		sum += rc.Weight
	}
	approx(t, sum, 1.0, 1e-9)
}

// TestRiskContributionsSumToPortfolio guards the division-by-variance fix.
// Risk contributions must partition the portfolio, so their shares total 1.
// Dividing by volatility instead of variance used to yield a few percent.
func TestRiskContributionsSumToPortfolio(t *testing.T) {
	n := 120
	rets := map[string][]float64{
		"A": rng(1, n, 0.001, 0.02),
		"B": rng(2, n, 0.001, 0.03),
		"C": rng(3, n, 0.001, 0.015),
	}
	res, err := Analyze(rets, []Holding{{"A", 50}, {"B", 30}, {"C", 20}})
	if err != nil {
		t.Fatal(err)
	}
	var share, contrib float64
	for _, rc := range res.RiskContributions {
		share += rc.Percentage
		contrib += rc.Contribution
	}
	approx(t, share, 1.0, 1e-9)
	// The raw contributions must add up to portfolio variance.
	approx(t, contrib, res.PortfolioVolatility*res.PortfolioVolatility, 1e-12)
}

func TestAnalyzeDiversificationBeatsConcentration(t *testing.T) {
	n := 60
	// Two perfectly uncorrelated assets.
	a := rng(1, n, 0.001, 0.02)
	b := rng(99, n, 0.001, 0.02)
	unbounded := map[string][]float64{"A": a, "B": b}
	unres, err := Analyze(unbounded, []Holding{{"A", 0.5}, {"B", 0.5}})
	if err != nil {
		t.Fatal(err)
	}

	// Two perfectly correlated assets (identical series).
	c := make([]float64, n)
	copy(c, a)
	d := make([]float64, n)
	copy(d, a)
	correlated := map[string][]float64{"C": c, "D": d}
	cres, err := Analyze(correlated, []Holding{{"C", 0.5}, {"D", 0.5}})
	if err != nil {
		t.Fatal(err)
	}
	if cres.PortfolioVolatility <= unres.PortfolioVolatility {
		t.Fatalf("correlated portfolio must be riskier: %.6f vs %.6f",
			cres.PortfolioVolatility, unres.PortfolioVolatility)
	}
	// A higher diversification ratio means less risk reduction.
	if cres.DiversificationRatio <= unres.DiversificationRatio {
		t.Fatalf("correlated portfolio must have a worse diversification ratio: %.6f vs %.6f",
			cres.DiversificationRatio, unres.DiversificationRatio)
	}
}

func TestAnalyzeHighCorrelationPairs(t *testing.T) {
	n := 60
	base := rng(7, n, 0.001, 0.02)
	clone := make([]float64, n)
	copy(clone, base)
	different := rng(50, n, 0.001, 0.02)
	rets := map[string][]float64{"X": base, "Y": clone, "Z": different}
	res, err := Analyze(rets, []Holding{{"X", 0.34}, {"Y", 0.33}, {"Z", 0.33}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.HighCorrelationPairs) != 1 {
		t.Fatalf("expected exactly one high-correlation pair, got %v", res.HighCorrelationPairs)
	}
	p := res.HighCorrelationPairs[0]
	if p.Correlation < 0.8 || p.Correlation > 1+1e-9 {
		t.Fatalf("unexpected correlation: %f", p.Correlation)
	}
}

func TestAnalyzeHHI(t *testing.T) {
	n := 40
	rets := map[string][]float64{}
	held := []Holding{}
	for i, c := range []string{"A", "B", "C", "D"} {
		rets[c] = rng(int64(i+1), n, 0.001, 0.02)
		held = append(held, Holding{c, 0.25})
	}
	res, err := Analyze(rets, held)
	if err != nil {
		t.Fatal(err)
	}
	approx(t, res.HHI, 4*0.25*0.25*10000, 1e-6)
	if len(res.HighCorrelationPairs) != 0 {
		t.Fatalf("equal-weight independent assets should have no high pairs: %v", res.HighCorrelationPairs)
	}
}

func TestAnalyzeRecommendationsCapped(t *testing.T) {
	n := 60
	rets := map[string][]float64{}
	held := []Holding{}
	for i, c := range []string{"A", "B", "C", "D", "E", "F"} {
		rets[c] = rng(int64(i+1), n, 0.001, 0.02)
		held = append(held, Holding{c, 1.0 / float64(i+1)})
	}
	res, err := Analyze(rets, held)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Recommendations) == 0 {
		t.Fatal("expected at least one recommendation")
	}
	if len(res.Recommendations) > 5 {
		t.Fatalf("recommendations must be capped at 5, got %d", len(res.Recommendations))
	}
}

func TestAnalyzeValidation(t *testing.T) {
	n := 40
	a := rng(1, n, 0.001, 0.02)
	b := rng(2, n, 0.001, 0.02)
	rets := map[string][]float64{"A": a, "B": b}

	if _, err := Analyze(rets, []Holding{{"A", 1}}); !errors.Is(err, ErrTooFewAssets) {
		t.Fatalf("expected ErrTooFewAssets, got %v", err)
	}
	if _, err := Analyze(rets, []Holding{{"A", 0.5}, {"B", 0.5}, {"MISSING", 0}}); err == nil {
		t.Fatal("expected missing data error")
	}
	if _, err := Analyze(rets, []Holding{{"A", 0.5}, {"A", 0.5}}); err == nil {
		t.Fatal("expected duplicate holding error")
	}
	if _, err := Analyze(rets, []Holding{{"A", 0.5}, {"B", -0.5}}); err == nil {
		t.Fatal("expected non-positive weight error")
	}
	short := map[string][]float64{"A": a, "B": rng(2, 5, 0.001, 0.02)}
	if _, err := Analyze(short, []Holding{{"A", 0.5}, {"B", 0.5}}); !errors.Is(err, ErrShortHistory) {
		t.Fatalf("expected ErrShortHistory, got %v", err)
	}
}

func TestMaxSingleWeight(t *testing.T) {
	code, w, breach := MaxSingleWeight(map[string]float64{"A": 0.2, "B": 0.45, "C": 0.35})
	if code != "B" || math.Abs(w-0.45) > 1e-12 || !breach {
		t.Fatalf("unexpected: %s %v %v", code, w, breach)
	}
	code2, w2, breach2 := MaxSingleWeight(map[string]float64{"A": 0.25, "B": 0.25, "C": 0.25, "D": 0.25})
	if breach2 {
		t.Fatalf("even weights should not breach: %s %v", code2, w2)
	}
}

func TestPearsonConstantSeries(t *testing.T) {
	constant := make([]float64, 10)
	for i := range constant {
		constant[i] = 0.5
	}
	if got := pearson(constant, rng(1, 10, 0, 0.1)); got != 0 {
		t.Fatalf("correlation with a constant series must be 0, got %f", got)
	}
	if got := pearson([]float64{1}, []float64{1}); got != 0 {
		t.Fatalf("single-observation correlation must be 0, got %f", got)
	}
}

func TestPearsonClosedForm(t *testing.T) {
	cases := []struct {
		name string
		a, b []float64
		want float64
	}{
		{"perfect positive", []float64{1, 2, 3, 4, 5}, []float64{2, 4, 6, 8, 10}, 1},
		{"perfect negative", []float64{1, 2, 3, 4, 5}, []float64{5, 4, 3, 2, 1}, -1},
		// Trailing junk beyond the shorter series must be ignored, so the
		// common-length truncation does not change the answer.
		{"unequal length", []float64{1, 2, 3, 4, 5, 999}, []float64{5, 4, 3, 2, 1}, -1},
		{"unequal positive", []float64{1, 2, 3, 4, 5, -1}, []float64{2, 4, 6, 8, 10}, 1},
	}
	for _, tc := range cases {
		if got := pearson(tc.a, tc.b); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("%s: got %.12f, want %v", tc.name, got, tc.want)
		}
	}
}

func TestCorrelationMatrixInvariants(t *testing.T) {
	// These are structural properties of Pearson correlation that hold for
	// any implementation, so they catch a broken estimator without needing a
	// known-good baseline to compare against.
	codes := []string{"A", "B", "C"}
	series := map[string][]float64{
		"A": rng(1, 60, 0.001, 0.02),
		"B": rng(2, 60, 0.001, 0.03),
		"C": rng(3, 60, 0.001, 0.015),
	}
	m := makeCorrelationMatrix(codes, series)

	n := len(codes)
	for i := 0; i < n; i++ {
		if math.Abs(m[i][i]-1) > 1e-9 {
			t.Errorf("diagonal [%d][%d] must be 1, got %f", i, i, m[i][i])
		}
		for j := 0; j < n; j++ {
			if math.Abs(m[i][j]-m[j][i]) > 1e-9 {
				t.Errorf("not symmetric: [%d][%d]=%f vs [%d][%d]=%f", i, j, m[i][j], j, i, m[j][i])
			}
			if m[i][j] < -1-1e-9 || m[i][j] > 1+1e-9 {
				t.Errorf("out of range [-1,1]: [%d][%d]=%f", i, j, m[i][j])
			}
		}
	}
}

func TestCorrelationInvariance(t *testing.T) {
	base := rng(1, 80, 0.001, 0.02)
	scaled := make([]float64, len(base))
	for i, v := range base {
		scaled[i] = 3*v + 5 // positive affine transform
	}
	negated := make([]float64, len(base))
	for i, v := range base {
		negated[i] = -v
	}
	if r := pearson(base, scaled); math.Abs(r-1) > 1e-9 {
		t.Errorf("correlation must be invariant to a positive affine transform, got %f", r)
	}
	if r := pearson(base, negated); math.Abs(r+1) > 1e-9 {
		t.Errorf("negating one series must flip the sign, got %f", r)
	}
	if r := pearson(base, base); math.Abs(r-1) > 1e-9 {
		t.Errorf("a series must correlate with itself, got %f", r)
	}
}

func TestCorrelationMonotonicDegradation(t *testing.T) {
	// Correlation must fall monotonically as independent noise is mixed into
	// one series: at zero noise the series correlate perfectly, and each added
	// increment of noise pushes the relationship toward zero. The signal must
	// carry more variance than the noise, otherwise the degradation bound is
	// not near zero and the ordering is not meaningful.
	signal := rng(1, 200, 0.001, 0.02)
	noise := rng(999, 200, 0, 0.005)
	if r := pearson(signal, noise); math.Abs(r) > 0.2 {
		t.Fatalf("noise seed correlated with signal: %f", r)
	}

	prev := math.Inf(1)
	for _, w := range []float64{0, 0.25, 0.5, 1, 2, 4, 8} {
		mixed := make([]float64, len(signal))
		for i := range signal {
			mixed[i] = signal[i] + w*noise[i]
		}
		r := pearson(signal, mixed)
		if math.Abs(r) > 1+1e-9 {
			t.Fatalf("correlation out of range at weight %v: %f", w, r)
		}
		if r >= prev {
			t.Fatalf("correlation must strictly decrease as noise weight rises: %f at weight %v follows %f", r, w, prev)
		}
		prev = r
	}
}

func TestVolatilityPositive(t *testing.T) {
	if volatility([]float64{1, 1, 1, 1}) != 0 {
		t.Fatal("constant series must have zero volatility")
	}
	if v := volatility(rng(1, 30, 0, 0.02)); v <= 0 {
		t.Fatalf("volatility must be positive, got %f", v)
	}
}

func approx(t *testing.T, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Fatalf("got %.10f, want %.10f (tol %.10f)", got, want, tol)
	}
}
