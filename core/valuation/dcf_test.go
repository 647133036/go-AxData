package valuation

import (
	"errors"
	"math"
	"testing"
)

func baseInputs() (CashFlows, Assumptions) {
	return CashFlows{FreeCashFlow: []float64{100}},
		Assumptions{SharesOut: 1000, NetDebt: 50}
}

func TestDCFDefaults(t *testing.T) {
	cash, a := baseInputs()
	res, err := DCF(cash, a, 100)
	if err != nil {
		t.Fatal(err)
	}
	if res.Assumptions.ExplicitYears != defaultExplicitYears {
		t.Fatalf("default years not applied: %d", res.Assumptions.ExplicitYears)
	}
	if res.Assumptions.WACC != defaultWACC || res.Assumptions.TerminalGrowth != defaultTerminalGrowth {
		t.Fatalf("default rates not applied: %+v", res.Assumptions)
	}
	if res.FreeCashFlowProxy != "free cash flow" {
		t.Fatalf("unexpected proxy: %q", res.FreeCashFlowProxy)
	}
}

func TestDCFInvariants(t *testing.T) {
	cash, a := baseInputs()
	res, err := DCF(cash, a, 100)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(res.EnterpriseValue-(res.ExplicitPeriodPV+res.TerminalValuePV)) > 1e-6 {
		t.Fatalf("EV mismatch: %.6f vs %.6f", res.EnterpriseValue, res.ExplicitPeriodPV+res.TerminalValuePV)
	}
	if math.Abs(res.EquityValue-(res.EnterpriseValue-a.NetDebt)) > 1e-6 {
		t.Fatalf("equity mismatch: %.6f", res.EquityValue)
	}
	wantPerShare := res.EquityValue / a.SharesOut
	if math.Abs(res.IntrinsicValuePerShare-wantPerShare) > 1e-9 {
		t.Fatalf("per share mismatch: %.6f vs %.6f", res.IntrinsicValuePerShare, wantPerShare)
	}
	wantMoS := (res.IntrinsicValuePerShare - res.CurrentPrice) / res.CurrentPrice
	if math.Abs(res.MarginOfSafety-wantMoS) > 1e-9 {
		t.Fatalf("MoS mismatch: %.6f vs %.6f", res.MarginOfSafety, wantMoS)
	}
}

func TestDCFTerminalValueDominates(t *testing.T) {
	cash, a := baseInputs()
	res, err := DCF(cash, a, 100)
	if err != nil {
		t.Fatal(err)
	}
	if res.TerminalValuePV <= res.ExplicitPeriodPV {
		t.Fatalf("terminal value should dominate in a 5-year model: tv=%.2f explicit=%.2f",
			res.TerminalValuePV, res.ExplicitPeriodPV)
	}
}

func TestDCFAccurateClosedForm(t *testing.T) {
	cash := CashFlows{FreeCashFlow: []float64{100}}
	a := Assumptions{ExplicitYears: 5, GrowthRate: 0.08, WACC: 0.09,
		TerminalGrowth: 0.025, SharesOut: 1000, NetDebt: 50}
	res, err := DCF(cash, a, 100)
	if err != nil {
		t.Fatal(err)
	}

	base := 100.0
	var pv float64
	fc := base
	for y := 1; y <= 5; y++ {
		fc *= 1 + 0.08
		pv += fc / math.Pow(1.09, float64(y))
	}
	tv := fc * 1.025 / (0.09 - 0.025)
	tvPV := tv / math.Pow(1.09, 5.0)
	wantEV := pv + tvPV
	if math.Abs(res.EnterpriseValue-wantEV) > 1e-6 {
		t.Fatalf("EV closed-form mismatch: %.8f vs %.8f", res.EnterpriseValue, wantEV)
	}
}

func TestDCFOCFProxy(t *testing.T) {
	cash := CashFlows{
		FreeCashFlow:      []float64{-100},
		OperatingCashFlow: []float64{80},
	}
	a := Assumptions{SharesOut: 1000, NetDebt: 50}
	res, err := DCF(cash, a, 100)
	if err != nil {
		t.Fatal(err)
	}
	if res.FreeCashFlowProxy != "operating cash flow" {
		t.Fatalf("expected OCF proxy, got %q", res.FreeCashFlowProxy)
	}
}

func TestDCFAllNegative(t *testing.T) {
	cash := CashFlows{
		FreeCashFlow:      []float64{-100},
		OperatingCashFlow: []float64{-50},
	}
	a := Assumptions{SharesOut: 1000}
	if _, err := DCF(cash, a, 100); err == nil {
		t.Fatal("expected negative FCF error")
	} else if !errors.Is(err, ErrNegativeFCF) {
		t.Fatalf("expected ErrNegativeFCF, got %v", err)
	}
}

func TestDCFValidation(t *testing.T) {
	cash, a := baseInputs()
	if _, err := DCF(cash, a, 0); err == nil {
		t.Fatal("expected price error")
	} else if !errors.Is(err, ErrPriceRequired) {
		t.Fatalf("expected ErrPriceRequired, got %v", err)
	}

	noShares := Assumptions{}
	if _, err := DCF(cash, noShares, 100); err == nil {
		t.Fatal("expected shares error")
	} else if !errors.Is(err, ErrSharesRequired) {
		t.Fatalf("expected ErrSharesRequired, got %v", err)
	}

	badRate := Assumptions{SharesOut: 1000, WACC: 0.02, TerminalGrowth: 0.05}
	if _, err := DCF(cash, badRate, 100); err == nil {
		t.Fatal("expected wacc>growth error")
	} else if !errors.Is(err, ErrWACCNotGreaterThanGrowth) {
		t.Fatalf("expected ErrWACCNotGreaterThanGrowth, got %v", err)
	}

	badYears := Assumptions{SharesOut: 1000, ExplicitYears: 11}
	if _, err := DCF(cash, badYears, 100); err == nil {
		t.Fatal("expected years range error")
	}

	highGrowth := Assumptions{SharesOut: 1000, GrowthRate: 0.10, WACC: 0.09}
	if _, err := DCF(cash, highGrowth, 100); err == nil {
		t.Fatal("expected growth>=wacc error")
	}
}

func TestDCFSensitivityGrid(t *testing.T) {
	cash, a := baseInputs()
	res, err := DCF(cash, a, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Sensitivity) != 5 {
		t.Fatalf("expected 5 growth rows, got %d", len(res.Sensitivity))
	}
	for _, row := range res.Sensitivity {
		if len(row) != 9 {
			t.Fatalf("expected 9 wacc columns, got %d", len(row))
		}
	}
	center := res.Sensitivity[2][4]
	base := res.Assumptions
	if math.Abs(center.WACC-base.WACC) > 1e-12 || math.Abs(center.Growth-base.TerminalGrowth) > 1e-12 {
		t.Fatalf("center cell is not the base case: %+v (base %+v)", center, base)
	}
	if math.Abs(center.Value-res.IntrinsicValuePerShare) > 1e-6 {
		t.Fatalf("center value mismatch: %.6f vs %.6f", center.Value, res.IntrinsicValuePerShare)
	}
	lowWACC := res.Sensitivity[2][0]
	if math.Abs(lowWACC.WACC-(base.WACC-0.02)) > 1e-12 {
		t.Fatalf("low wacc corner wrong: %+v", lowWACC)
	}
	if lowWACC.Value <= center.Value {
		t.Fatalf("lower wacc should raise value: %.6f vs %.6f", lowWACC.Value, center.Value)
	}
	highGrowth := res.Sensitivity[4][4]
	if highGrowth.Value <= center.Value {
		t.Fatalf("higher growth should raise value: %.6f vs %.6f", highGrowth.Value, center.Value)
	}
}

func TestDCFSensitivityMonotonic(t *testing.T) {
	cash, a := baseInputs()
	res, err := DCF(cash, a, 100)
	if err != nil {
		t.Fatal(err)
	}

	// The grid layout is the contract the renderer relies on: one growth per
	// row, one wacc per column. Assert it here so a relabelled renderer
	// cannot pass review on the model's behalf.
	for i, row := range res.Sensitivity {
		g := row[0].Growth
		for _, cell := range row {
			if math.Abs(cell.Growth-g) > 1e-12 {
				t.Fatalf("row %d mixes growth rates: %.6f vs %.6f", i, cell.Growth, g)
			}
		}
	}
	for j := range res.Sensitivity[0] {
		w := res.Sensitivity[0][j].WACC
		for i := range res.Sensitivity {
			if math.Abs(res.Sensitivity[i][j].WACC-w) > 1e-12 {
				t.Fatalf("col %d mixes wacc values: %.6f vs %.6f", j, res.Sensitivity[i][j].WACC, w)
			}
		}
	}

	// Value falls as WACC rises within a row and rises as terminal growth
	// rises down a column, across every feasible cell rather than only the
	// corners. Infeasible cells are contiguous (a wacc prefix in each row, a
	// growth suffix in each column), so dropping them leaves feasible pairs
	// adjacent and the strict order still has to hold.
	rows := len(res.Sensitivity)
	cols := len(res.Sensitivity[0])
	for i := 0; i < rows; i++ {
		vals := []float64{}
		for j := 0; j < cols; j++ {
			if v := res.Sensitivity[i][j].Value; v != 0 {
				vals = append(vals, v)
			}
		}
		if !isMonotonicDecreasing(vals) {
			t.Fatalf("row %d: value must fall as wacc rises: %v", i, vals)
		}
	}
	for j := 0; j < cols; j++ {
		vals := []float64{}
		for i := 0; i < rows; i++ {
			if v := res.Sensitivity[i][j].Value; v != 0 {
				vals = append(vals, v)
			}
		}
		if !isMonotonicIncreasing(vals) {
			t.Fatalf("col %d: value must rise as growth rises: %v", j, vals)
		}
	}
}

// isMonotonicDecreasing and isMonotonicIncreasing are one-pass deciders: one
// comparison per adjacent pair, a verdict and nothing else. stdlib
// slices.IsSorted covers only the non-strict case, which would accept a flat
// grid where the value stops moving.
func isMonotonicDecreasing(vals []float64) bool {
	for i := 1; i < len(vals); i++ {
		if vals[i-1] <= vals[i] {
			return false
		}
	}
	return true
}

func isMonotonicIncreasing(vals []float64) bool {
	for i := 1; i < len(vals); i++ {
		if vals[i-1] >= vals[i] {
			return false
		}
	}
	return true
}

func TestDCFSensitivitySkipsInfeasibleCells(t *testing.T) {
	cash := CashFlows{FreeCashFlow: []float64{100}}
	// WACC and growth are close enough that part of the sweep is infeasible.
	a := Assumptions{SharesOut: 1000, GrowthRate: 0.025, WACC: 0.04, TerminalGrowth: 0.03}
	res, err := DCF(cash, a, 100)
	if err != nil {
		t.Fatal(err)
	}
	foundInvalid := false
	for _, row := range res.Sensitivity {
		for _, cell := range row {
			if cell.Value == 0 {
				foundInvalid = true
				continue
			}
			if cell.WACC <= cell.Growth {
				t.Fatalf("cell computed below feasibility bound: %+v", cell)
			}
		}
	}
	if !foundInvalid {
		t.Fatal("expected at least one infeasible cell in a tight-spread sweep")
	}
}

func TestDCFMarginOfSafetySign(t *testing.T) {
	cash, a := baseInputs()
	cheap, err := DCF(cash, a, 1)
	if err != nil {
		t.Fatal(err)
	}
	// Margin of safety is measured against the market price, so a far-below-
	// value price yields a large positive multiple rather than a fraction.
	if cheap.MarginOfSafety <= 0 {
		t.Fatalf("MoS must be positive for a cheap price: %f", cheap.MarginOfSafety)
	}

	fair, err := DCF(cash, a, cheap.IntrinsicValuePerShare)
	if err != nil {
		t.Fatal(err)
	}
	if fair.MarginOfSafety != 0 {
		t.Fatalf("MoS must be 0 at fair value, got %f", fair.MarginOfSafety)
	}

	expensive, err := DCF(cash, a, 100000)
	if err != nil {
		t.Fatal(err)
	}
	if expensive.MarginOfSafety >= 0 {
		t.Fatalf("MoS must be negative for an expensive price: %f", expensive.MarginOfSafety)
	}
}

func TestFormattingHelpers(t *testing.T) {
	if got := ValueLabel(2e9); got != "20.00亿" {
		t.Fatalf("ValueLabel: %q", got)
	}
	if got := Percent(0.15); got != "15.0%" {
		t.Fatalf("Percent: %q", got)
	}
}
