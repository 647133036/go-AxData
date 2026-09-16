// Package indicator implements technical indicator computations as pure functions.
// No I/O, no external dependencies beyond the standard library.
package indicator

import (
	"errors"
	"fmt"
	"math"
)

// ErrInsufficientData is returned when a series is shorter than the minimum
// samples an indicator requires. Match with errors.Is.
var ErrInsufficientData = errors.New("insufficient data")

// InsufficientDataError carries the sample counts needed to fix a caller bug.
type InsufficientDataError struct {
	Indicator string
	Required  int
	Actual    int
}

func (e *InsufficientDataError) Error() string {
	return fmt.Sprintf("insufficient data for %s: need %d samples, have %d", e.Indicator, e.Required, e.Actual)
}

func (e *InsufficientDataError) Is(target error) bool {
	return target == ErrInsufficientData
}

// Series pairs date labels with numeric values.
type Series struct {
	Dates  []string
	Values []float64
}

const maxPeriod = 500

func validatePeriod(period int, name string) error {
	if period < 1 || period > maxPeriod {
		return fmt.Errorf("invalid %s period %d: must be between 1 and %d", name, period, maxPeriod)
	}
	return nil
}

// sanitize replaces NaN and Inf with zero so bad inputs cannot poison a result.
func sanitize(values []float64) []float64 {
	out := make([]float64, len(values))
	for i, v := range values {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			out[i] = 0
			continue
		}
		out[i] = v
	}
	return out
}

func requireEqualHighLowClose(high, low, close []float64) error {
	if len(high) != len(low) || len(low) != len(close) {
		return fmt.Errorf("indicator requires equal length high/low/close series")
	}
	return nil
}

func meanStd(values []float64) (float64, float64) {
	n := len(values)
	if n == 0 {
		return 0, 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	mean := sum / float64(n)
	if n < 2 {
		return mean, 0
	}
	var sq float64
	for _, v := range values {
		d := v - mean
		sq += d * d
	}
	return mean, math.Sqrt(sq / float64(n-1))
}

// MA returns the simple moving average. Positions before the window fills are zero.
func MA(values []float64, period int) ([]float64, error) {
	if err := validatePeriod(period, "MA"); err != nil {
		return nil, err
	}
	values = sanitize(values)
	n := len(values)
	if n < period {
		return nil, &InsufficientDataError{Indicator: "MA", Required: period, Actual: n}
	}
	out := make([]float64, n)
	var sum float64
	for i, v := range values {
		sum += v
		if i >= period {
			sum -= values[i-period]
		}
		if i >= period-1 {
			out[i] = sum / float64(period)
		}
	}
	return out, nil
}

// EMA returns the exponential moving average seeded with the simple average
// of the first period values.
func EMA(values []float64, period int) ([]float64, error) {
	if err := validatePeriod(period, "EMA"); err != nil {
		return nil, err
	}
	values = sanitize(values)
	n := len(values)
	if n < period {
		return nil, &InsufficientDataError{Indicator: "EMA", Required: period, Actual: n}
	}
	k := 2.0 / float64(period+1)
	out := make([]float64, n)
	seed := 0.0
	for i := 0; i < period; i++ {
		seed += values[i]
	}
	out[period-1] = seed / float64(period)
	for i := period; i < n; i++ {
		out[i] = values[i]*k + out[i-1]*(1-k)
	}
	return out, nil
}

// RSI returns the relative strength index using Wilder smoothing.
func RSI(values []float64, period int) ([]float64, error) {
	if period == 0 {
		period = 14
	}
	if err := validatePeriod(period, "RSI"); err != nil {
		return nil, err
	}
	values = sanitize(values)
	n := len(values)
	if n < period+1 {
		return nil, &InsufficientDataError{Indicator: "RSI", Required: period + 1, Actual: n}
	}
	out := make([]float64, n)
	var gainSum, lossSum float64
	for i := 1; i <= period; i++ {
		d := values[i] - values[i-1]
		if d >= 0 {
			gainSum += d
		} else {
			lossSum += -d
		}
	}
	avgGain := gainSum / float64(period)
	avgLoss := lossSum / float64(period)
	out[period] = rsiFromAvg(avgGain, avgLoss)
	for i := period + 1; i < n; i++ {
		d := values[i] - values[i-1]
		var g, l float64
		if d >= 0 {
			g = d
		} else {
			l = -d
		}
		avgGain = (avgGain*float64(period-1) + g) / float64(period)
		avgLoss = (avgLoss*float64(period-1) + l) / float64(period)
		out[i] = rsiFromAvg(avgGain, avgLoss)
	}
	return out, nil
}

func rsiFromAvg(avgGain, avgLoss float64) float64 {
	if avgLoss == 0 {
		if avgGain == 0 {
			return 50
		}
		return 100
	}
	rs := avgGain / avgLoss
	return 100 - 100/(1+rs)
}

// MACDResult holds the three MACD series. HIST follows the A-share convention
// of 2*(DIF-DEA).
type MACDResult struct {
	DIF    []float64
	DEA    []float64
	HIST   []float64
	Fast   int
	Slow   int
	Signal int
}

// MACD returns DIF, DEA and the histogram.
func MACD(values []float64, fast, slow, signal int) (*MACDResult, error) {
	if fast == 0 {
		fast = 12
	}
	if slow == 0 {
		slow = 26
	}
	if signal == 0 {
		signal = 9
	}
	if err := validatePeriod(fast, "MACD fast"); err != nil {
		return nil, err
	}
	if err := validatePeriod(slow, "MACD slow"); err != nil {
		return nil, err
	}
	if err := validatePeriod(signal, "MACD signal"); err != nil {
		return nil, err
	}
	if slow <= fast {
		return nil, fmt.Errorf("MACD slow period %d must exceed fast period %d", slow, fast)
	}
	values = sanitize(values)
	n := len(values)
	need := slow + signal
	if n < need {
		return nil, &InsufficientDataError{Indicator: "MACD", Required: need, Actual: n}
	}
	emaFast, err := EMA(values, fast)
	if err != nil {
		return nil, err
	}
	emaSlow, err := EMA(values, slow)
	if err != nil {
		return nil, err
	}
	dif := make([]float64, n)
	for i := range values {
		dif[i] = emaFast[i] - emaSlow[i]
	}
	dea, err := EMA(dif, signal)
	if err != nil {
		return nil, err
	}
	hist := make([]float64, n)
	for i := range values {
		hist[i] = 2 * (dif[i] - dea[i])
	}
	return &MACDResult{DIF: dif, DEA: dea, HIST: hist, Fast: fast, Slow: slow, Signal: signal}, nil
}

// KDJResult holds the K, D and J series.
type KDJResult struct {
	K  []float64
	D  []float64
	J  []float64
	N  int
	M1 int
	M2 int
}

// KDJ returns the stochastic oscillator with SMA-style smoothing.
func KDJ(high, low, close []float64, n, m1, m2 int) (*KDJResult, error) {
	if n == 0 {
		n = 9
	}
	if m1 == 0 {
		m1 = 3
	}
	if m2 == 0 {
		m2 = 3
	}
	if err := validatePeriod(n, "KDJ n"); err != nil {
		return nil, err
	}
	if err := validatePeriod(m1, "KDJ m1"); err != nil {
		return nil, err
	}
	if err := validatePeriod(m2, "KDJ m2"); err != nil {
		return nil, err
	}
	if err := requireEqualHighLowClose(high, low, close); err != nil {
		return nil, err
	}
	high = sanitize(high)
	low = sanitize(low)
	close = sanitize(close)
	l := len(close)
	if l < n {
		return nil, &InsufficientDataError{Indicator: "KDJ", Required: n, Actual: l}
	}
	res := &KDJResult{
		K: make([]float64, l), D: make([]float64, l), J: make([]float64, l),
		N: n, M1: m1, M2: m2,
	}
	prevK, prevD := 50.0, 50.0
	for i := 0; i < l; i++ {
		start := i - n + 1
		if start < 0 {
			start = 0
		}
		hh, ll := math.Inf(-1), math.Inf(1)
		for j := start; j <= i; j++ {
			if high[j] > hh {
				hh = high[j]
			}
			if low[j] < ll {
				ll = low[j]
			}
		}
		rsv := 50.0
		if hh > ll {
			rsv = (close[i] - ll) / (hh - ll) * 100
		}
		k := float64(m1-1)/float64(m1)*prevK + 1/float64(m1)*rsv
		d := float64(m2-1)/float64(m2)*prevD + 1/float64(m2)*k
		res.K[i], res.D[i], res.J[i] = k, d, 3*k-2*d
		prevK, prevD = k, d
	}
	return res, nil
}

// BOLLResult holds the upper, middle and lower bands.
type BOLLResult struct {
	Upper  []float64
	Middle []float64
	Lower  []float64
	Period int
	Mult   float64
}

// BOLL returns Bollinger Bands using the sample standard deviation.
func BOLL(close []float64, period int, mult int) (*BOLLResult, error) {
	if period == 0 {
		period = 20
	}
	if mult == 0 {
		mult = 2
	}
	if err := validatePeriod(period, "BOLL"); err != nil {
		return nil, err
	}
	if mult < 1 {
		return nil, fmt.Errorf("BOLL multiplier %d must be >= 1", mult)
	}
	close = sanitize(close)
	n := len(close)
	if n < period {
		return nil, &InsufficientDataError{Indicator: "BOLL", Required: period, Actual: n}
	}
	m := float64(mult)
	res := &BOLLResult{
		Upper: make([]float64, n), Middle: make([]float64, n), Lower: make([]float64, n),
		Period: period, Mult: m,
	}
	for i := period - 1; i < n; i++ {
		mean, std := meanStd(close[i-period+1 : i+1])
		res.Middle[i] = mean
		res.Upper[i] = mean + m*std
		res.Lower[i] = mean - m*std
	}
	return res, nil
}

// ATR returns the average true range using Wilder smoothing.
func ATR(high, low, close []float64, period int) ([]float64, error) {
	if period == 0 {
		period = 14
	}
	if err := validatePeriod(period, "ATR"); err != nil {
		return nil, err
	}
	if err := requireEqualHighLowClose(high, low, close); err != nil {
		return nil, err
	}
	high = sanitize(high)
	low = sanitize(low)
	close = sanitize(close)
	n := len(close)
	if n < period+1 {
		return nil, &InsufficientDataError{Indicator: "ATR", Required: period + 1, Actual: n}
	}
	tr := make([]float64, n)
	for i := 0; i < n; i++ {
		if i == 0 {
			tr[i] = high[i] - low[i]
			continue
		}
		tr[i] = max3(high[i]-low[i], math.Abs(high[i]-close[i-1]), math.Abs(low[i]-close[i-1]))
	}
	out := make([]float64, n)
	seed := 0.0
	for i := 1; i <= period; i++ {
		seed += tr[i]
	}
	out[period] = seed / float64(period)
	for i := period + 1; i < n; i++ {
		out[i] = (out[i-1]*float64(period-1) + tr[i]) / float64(period)
	}
	return out, nil
}

func max3(a, b, c float64) float64 {
	m := a
	if b > m {
		m = b
	}
	if c > m {
		m = c
	}
	return m
}

// CrossEvent marks the index of a crossover between two series.
type CrossEvent struct {
	Index int
	Value float64
}

// GoldenCross returns indices where fast moves from below to above slow.
func GoldenCross(fast, slow []float64) []CrossEvent {
	return crosses(fast, slow, true)
}

// DeathCross returns indices where fast moves from above to below slow.
func DeathCross(fast, slow []float64) []CrossEvent {
	return crosses(fast, slow, false)
}

func crosses(fast, slow []float64, golden bool) []CrossEvent {
	var events []CrossEvent
	l := len(fast)
	if l > len(slow) {
		l = len(slow)
	}
	for i := 1; i < l; i++ {
		if math.IsNaN(fast[i]) || math.IsNaN(slow[i]) ||
			math.IsNaN(fast[i-1]) || math.IsNaN(slow[i-1]) {
			continue
		}
		if golden {
			if fast[i-1] <= slow[i-1] && fast[i] > slow[i] {
				events = append(events, CrossEvent{Index: i, Value: fast[i] - slow[i]})
			}
			continue
		}
		if fast[i-1] >= slow[i-1] && fast[i] < slow[i] {
			events = append(events, CrossEvent{Index: i, Value: fast[i] - slow[i]})
		}
	}
	return events
}
