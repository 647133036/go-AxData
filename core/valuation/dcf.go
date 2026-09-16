// Package valuation implements discounted cash flow valuation.
package valuation

import (
	"errors"
	"fmt"
	"math"
)

// ErrNegativeFCF signals the fall back to an operating cash flow proxy.
var ErrNegativeFCF = errors.New("negative free cash flow")

// ErrSharesRequired is returned when shares outstanding is missing.
var ErrSharesRequired = errors.New("shares outstanding required")

// ErrPriceRequired is returned when the current price is missing.
var ErrPriceRequired = errors.New("current price required")

// ErrWACCNotGreaterThanGrowth is returned when the model cannot converge.
var ErrWACCNotGreaterThanGrowth = errors.New("wacc must exceed terminal growth rate")

// CashFlows carries the cash flow series used by the model. FreeCashFlow is
// preferred; OperatingCashFlow is used as a proxy when FCF is negative.
type CashFlows struct {
	FreeCashFlow      []float64
	OperatingCashFlow []float64
}

// Series returns the series the model should discount, setting *proxy to the
// name of the chosen series.
func (c CashFlows) Series() ([]float64, string, error) {
	if len(c.FreeCashFlow) > 0 && lastNonNaN(c.FreeCashFlow) > 0 {
		return c.FreeCashFlow, "free cash flow", nil
	}
	if len(c.OperatingCashFlow) > 0 && lastNonNaN(c.OperatingCashFlow) > 0 {
		return c.OperatingCashFlow, "operating cash flow", nil
	}
	return nil, "", ErrNegativeFCF
}

func lastNonNaN(series []float64) float64 {
	for i := len(series) - 1; i >= 0; i-- {
		if !math.IsNaN(series[i]) {
			return series[i]
		}
	}
	return 0
}

// Assumptions are the DCF inputs. Zero values fall back to the documented
// defaults.
type Assumptions struct {
	ExplicitYears  int
	GrowthRate     float64
	WACC           float64
	TerminalGrowth float64
	NetDebt        float64
	SharesOut      float64
}

// SensitivityCell is one point of the WACC × growth matrix.
type SensitivityCell struct {
	WACC   float64
	Growth float64
	Value  float64
	MoSF   float64
}

// Result is the full DCF output.
type Result struct {
	EnterpriseValue        float64
	ExplicitPeriodPV       float64
	TerminalValuePV        float64
	EquityValue            float64
	IntrinsicValuePerShare float64
	CurrentPrice           float64
	MarginOfSafety         float64
	FreeCashFlowProxy      string
	Assumptions            Assumptions
	Sensitivity            [][]SensitivityCell
}

const (
	defaultExplicitYears  = 5
	defaultGrowthRate     = 0.08
	defaultWACC           = 0.09
	defaultTerminalGrowth = 0.025
)

// DCF runs a two-stage FCFF model: explicit forecast period plus a
// perpetuity-growth terminal value.
func DCF(cashflows CashFlows, a Assumptions, currentPrice float64) (*Result, error) {
	if err := a.normalize(); err != nil {
		return nil, err
	}
	if currentPrice <= 0 || math.IsNaN(currentPrice) {
		return nil, fmt.Errorf("valuation: %w (got %v)", ErrPriceRequired, currentPrice)
	}
	if a.SharesOut <= 0 {
		return nil, fmt.Errorf("valuation: %w", ErrSharesRequired)
	}
	if a.WACC <= a.TerminalGrowth {
		return nil, fmt.Errorf("valuation: %w (wacc=%.4f, g=%.4f)",
			ErrWACCNotGreaterThanGrowth, a.WACC, a.TerminalGrowth)
	}

	fcf, proxy, err := cashflows.Series()
	if err != nil {
		return nil, fmt.Errorf("valuation: %w", err)
	}

	explicitPV, explicitLast, err := explicitPeriod(fcf, a)
	if err != nil {
		return nil, err
	}

	tv, err := terminalValue(explicitLast, a)
	if err != nil {
		return nil, err
	}
	tvPV := tv / math.Pow(1+a.WACC, float64(a.ExplicitYears))

	ev := explicitPV + tvPV
	equity := ev - a.NetDebt
	perShare := equity / a.SharesOut

	return &Result{
		EnterpriseValue:        ev,
		ExplicitPeriodPV:       explicitPV,
		TerminalValuePV:        tvPV,
		EquityValue:            equity,
		IntrinsicValuePerShare: perShare,
		CurrentPrice:           currentPrice,
		MarginOfSafety:         (perShare - currentPrice) / currentPrice,
		FreeCashFlowProxy:      proxy,
		Assumptions:            a,
		Sensitivity:            buildSensitivity(fcf, a, currentPrice),
	}, nil
}

func (a *Assumptions) normalize() error {
	if a.ExplicitYears == 0 {
		a.ExplicitYears = defaultExplicitYears
	}
	if a.ExplicitYears < 1 || a.ExplicitYears > 10 {
		return fmt.Errorf("valuation: explicit years must be 1..10, got %d", a.ExplicitYears)
	}
	if a.GrowthRate == 0 {
		a.GrowthRate = defaultGrowthRate
	}
	if a.WACC == 0 {
		a.WACC = defaultWACC
	}
	if a.TerminalGrowth == 0 {
		a.TerminalGrowth = defaultTerminalGrowth
	}
	if a.WACC <= 0 {
		return fmt.Errorf("valuation: wacc must be positive, got %v", a.WACC)
	}
	if a.GrowthRate < 0 || a.GrowthRate >= a.WACC {
		return fmt.Errorf("valuation: %w (growth=%.4f, wacc=%.4f)",
			ErrWACCNotGreaterThanGrowth, a.GrowthRate, a.WACC)
	}
	return nil
}

func explicitPeriod(fcf []float64, a Assumptions) (pv float64, lastFCFValue float64, err error) {
	base := lastNonNaN(fcf)
	if math.IsNaN(base) {
		return 0, 0, fmt.Errorf("valuation: free cash flow base is NaN")
	}
	forecast := base
	for y := 1; y <= a.ExplicitYears; y++ {
		forecast *= 1 + a.GrowthRate
		pv += forecast / math.Pow(1+a.WACC, float64(y))
	}
	return pv, forecast, nil
}

func terminalValue(lastFCFValue float64, a Assumptions) (float64, error) {
	if a.TerminalGrowth < 0 {
		return 0, fmt.Errorf("valuation: terminal growth must be non-negative, got %v", a.TerminalGrowth)
	}
	if a.WACC <= a.TerminalGrowth {
		return 0, fmt.Errorf("valuation: %w", ErrWACCNotGreaterThanGrowth)
	}
	return lastFCFValue * (1 + a.TerminalGrowth) / (a.WACC - a.TerminalGrowth), nil
}

// buildSensitivity sweeps WACC ±2pp and terminal growth ±1pp in 0.5pp steps.
func buildSensitivity(fcf []float64, base Assumptions, price float64) [][]SensitivityCell {
	waccSteps := []float64{
		base.WACC - 0.02, base.WACC - 0.015, base.WACC - 0.01, base.WACC - 0.005,
		base.WACC, base.WACC + 0.005, base.WACC + 0.01, base.WACC + 0.015, base.WACC + 0.02,
	}
	gSteps := []float64{
		base.TerminalGrowth - 0.01, base.TerminalGrowth - 0.005, base.TerminalGrowth,
		base.TerminalGrowth + 0.005, base.TerminalGrowth + 0.01,
	}
	grid := make([][]SensitivityCell, 0, len(gSteps))
	for _, g := range gSteps {
		row := make([]SensitivityCell, 0, len(waccSteps))
		for _, w := range waccSteps {
			if w <= g {
				row = append(row, SensitivityCell{WACC: w, Growth: g})
				continue
			}
			a := base
			a.WACC, a.TerminalGrowth = w, g
			pv, last, err := explicitPeriod(fcf, a)
			if err != nil {
				row = append(row, SensitivityCell{WACC: w, Growth: g})
				continue
			}
			tv, err := terminalValue(last, a)
			if err != nil {
				row = append(row, SensitivityCell{WACC: w, Growth: g})
				continue
			}
			ev := pv + tv/math.Pow(1+w, float64(a.ExplicitYears)) - a.NetDebt
			perShare := ev / a.SharesOut
			mos := 0.0
			if price > 0 {
				mos = (perShare - price) / price
			}
			row = append(row, SensitivityCell{WACC: w, Growth: g, Value: perShare, MoSF: mos})
		}
		grid = append(grid, row)
	}
	return grid
}

// ValueLabel formats a yuan amount in 亿 for terminal output.
func ValueLabel(v float64) string {
	return fmt.Sprintf("%.2f亿", v/1e8)
}

// Percent formats a fraction as a percentage string.
func Percent(v float64) string {
	return fmt.Sprintf("%.1f%%", v*100)
}
