package indicator

import (
	"errors"
	"math"
	"testing"
)

func approx(t *testing.T, got, want, tol float64) {
	t.Helper()
	if math.IsNaN(got) {
		t.Fatalf("got NaN, want %.6f", want)
	}
	if math.Abs(got-want) > tol {
		t.Fatalf("got %.10f, want %.10f (tol %.10f)", got, want, tol)
	}
}

func rising(n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = float64(i + 1)
	}
	return out
}

func TestMA(t *testing.T) {
	got, err := MA([]float64{1, 2, 3, 4, 5}, 3)
	if err != nil {
		t.Fatal(err)
	}
	approx(t, got[2], 2, 1e-9)
	approx(t, got[3], 3, 1e-9)
	approx(t, got[4], 4, 1e-9)
	if got[0] != 0 || got[1] != 0 {
		t.Fatalf("expected zero before window fills, got %v %v", got[0], got[1])
	}
	if _, err := MA([]float64{1}, 3); err == nil {
		t.Fatal("expected error for short series")
	} else if !errors.Is(err, ErrInsufficientData) {
		t.Fatalf("expected ErrInsufficientData, got %v", err)
	}
	if _, err := MA([]float64{1}, 0); err == nil {
		t.Fatal("expected period validation error")
	}
}

func TestMASanitizesNaN(t *testing.T) {
	got, err := MA([]float64{1, math.NaN(), 3}, 3)
	if err != nil {
		t.Fatal(err)
	}
	approx(t, got[2], 4/3.0, 1e-9)
}

func TestEMAMatchesSeed(t *testing.T) {
	values := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	got, err := EMA(values, 3)
	if err != nil {
		t.Fatal(err)
	}
	approx(t, got[2], 2, 1e-9)
	k := 2.0 / 4.0
	approx(t, got[3], values[3]*k+got[2]*(1-k), 1e-12)
}

func TestRSIRange(t *testing.T) {
	up, err := RSI(rising(30), 14)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range up[14:] {
		if v < 0 || v > 100 {
			t.Fatalf("RSI out of range: %f", v)
		}
	}
	approx(t, up[14], 100, 1e-9)

	down := make([]float64, 30)
	for i := range down {
		down[i] = float64(30 - i)
	}
	downRSI, err := RSI(down, 14)
	if err != nil {
		t.Fatal(err)
	}
	approx(t, downRSI[14], 0, 1e-9)

	flat := make([]float64, 30)
	for i := range flat {
		flat[i] = 5
	}
	flatRSI, err := RSI(flat, 14)
	if err != nil {
		t.Fatal(err)
	}
	approx(t, flatRSI[14], 50, 1e-9)
}

func TestMACDHistogramConvention(t *testing.T) {
	values := rising(40)
	res, err := MACD(values, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Fast != 12 || res.Slow != 26 || res.Signal != 9 {
		t.Fatalf("defaults not applied: %+v", res)
	}
	last := len(values) - 1
	want := 2 * (res.DIF[last] - res.DEA[last])
	approx(t, res.HIST[last], want, 1e-12)
	if _, err := MACD(values, 26, 12, 9); err == nil {
		t.Fatal("expected slow<=fast error")
	}
	if _, err := MACD(rising(10), 12, 26, 9); err == nil {
		t.Fatal("expected insufficient data error")
	} else if !errors.Is(err, ErrInsufficientData) {
		t.Fatalf("expected ErrInsufficientData, got %v", err)
	}
}

func TestKDJ(t *testing.T) {
	n := 20
	high := make([]float64, n)
	low := make([]float64, n)
	close_ := make([]float64, n)
	for i := 0; i < n; i++ {
		low[i] = float64(i)
		high[i] = float64(i) + 2
		close_[i] = float64(i) + 1
	}
	res, err := KDJ(high, low, close_, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.N != 9 || res.M1 != 3 || res.M2 != 3 {
		t.Fatalf("defaults not applied: %+v", res)
	}
	for i := range res.K {
		approx(t, res.J[i], 3*res.K[i]-2*res.D[i], 1e-12)
		if res.K[i] < 0 || res.K[i] > 100 {
			t.Fatalf("K out of [0,100] at %d: %f", i, res.K[i])
		}
	}
	// Monotone uptrend with close at the window high: K should sit near 100.
	if res.K[19] < 80 {
		t.Fatalf("K should be elevated in an uptrend, got %f", res.K[19])
	}
	if _, err := KDJ(high, low[:n-1], close_, 9, 3, 3); err == nil {
		t.Fatal("expected unequal length error")
	}
}

func TestBOLL(t *testing.T) {
	values := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	res, err := BOLL(values, 5, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Mult != 2 {
		t.Fatalf("default multiplier not applied: %f", res.Mult)
	}
	for i := 5; i < 10; i++ {
		if res.Lower[i] > res.Middle[i] || res.Middle[i] > res.Upper[i] {
			t.Fatalf("band order violated at %d: lower=%f middle=%f upper=%f", i, res.Lower[i], res.Middle[i], res.Upper[i])
		}
	}
	approx(t, res.Middle[9], 8, 1e-9)
	if _, err := BOLL(values, 5, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := BOLL(values, 5, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := BOLL([]float64{1}, 5, 2); err == nil {
		t.Fatal("expected insufficient data error")
	}
}

func TestATRPositive(t *testing.T) {
	n := 30
	high := make([]float64, n)
	low := make([]float64, n)
	close_ := make([]float64, n)
	for i := 0; i < n; i++ {
		close_[i] = 10 + float64(i)*0.1
		high[i] = close_[i] + 0.5
		low[i] = close_[i] - 0.5
	}
	got, err := ATR(high, low, close_, 0)
	if err != nil {
		t.Fatal(err)
	}
	// True range is dominated by the constant high-low spread of 1.0.
	approx(t, got[14], 1.0, 1e-9)
	if got[14] <= 0 {
		t.Fatalf("ATR must be positive, got %f", got[14])
	}
}

func TestCrosses(t *testing.T) {
	fast := []float64{1, 2, 4}
	slow := []float64{3, 3, 3}
	gc := GoldenCross(fast, slow)
	if len(gc) != 1 || gc[0].Index != 2 {
		t.Fatalf("expected golden cross at 2, got %+v", gc)
	}
	dc := DeathCross(fast, slow)
	if len(dc) != 0 {
		t.Fatalf("expected no death cross, got %+v", dc)
	}
	fast2 := []float64{4, 3, 1}
	dc2 := DeathCross(fast2, slow)
	if len(dc2) != 1 || dc2[0].Index != 2 {
		t.Fatalf("expected death cross at 2, got %+v", dc2)
	}
}

func TestInsufficientDataErrorIs(t *testing.T) {
	_, err := MA([]float64{1}, 5)
	if !errors.Is(err, ErrInsufficientData) {
		t.Fatalf("errors.Is failed: %v", err)
	}
	var target error = ErrInsufficientData
	if !errors.Is(err, target) {
		t.Fatalf("errors.Is via variable failed: %v", err)
	}
}

func TestMaxPeriodRejection(t *testing.T) {
	if _, err := MA([]float64{1}, 501); err == nil {
		t.Fatal("expected rejection of period > 500")
	}
	if _, err := MA([]float64{1}, -1); err == nil {
		t.Fatal("expected rejection of negative period")
	}
}
