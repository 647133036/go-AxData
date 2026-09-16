package cmd

import (
	"bytes"
	"math"
	"strings"
	"testing"
)

func TestSanitizeJSON(t *testing.T) {
	nested := map[string]interface{}{
		"code":   "600519.SH",
		"bad":    math.NaN(),
		"posInf": math.Inf(1),
		"negInf": math.Inf(-1),
		"ok":     1258.0,
		"rows": []interface{}{
			map[string]interface{}{"close": math.NaN()},
			map[string]interface{}{"close": 1.5},
			float32(math.NaN()),
			float32(math.Inf(1)),
			"text",
		},
	}

	out := sanitizeJSON(nested)
	om, ok := out.(map[string]interface{})
	if !ok {
		t.Fatalf("sanitizeJSON returned %T, want map", out)
	}
	if om["bad"] != nil || om["posInf"] != nil || om["negInf"] != nil {
		t.Errorf("non-finite floats not nullified: %v %v %v", om["bad"], om["posInf"], om["negInf"])
	}
	if om["ok"] != 1258.0 {
		t.Errorf("finite float changed: %v", om["ok"])
	}
	if om["code"] != "600519.SH" {
		t.Errorf("non-float value changed: %v", om["code"])
	}

	inner := om["rows"].([]interface{})
	if inner[1].(map[string]interface{})["close"] != 1.5 {
		t.Errorf("nested finite float changed: %v", inner[1])
	}
	if inner[0].(map[string]interface{})["close"] != nil {
		t.Errorf("nested NaN not nullified")
	}
	if inner[2] != nil || inner[3] != nil {
		t.Errorf("float32 non-finite values not nullified: %v %v", inner[2], inner[3])
	}
	if inner[4] != "text" {
		t.Errorf("string changed: %v", inner[4])
	}
}

func TestSanitizeJSONScalar(t *testing.T) {
	if sanitizeJSON(3.14) != 3.14 {
		t.Error("scalar finite float changed")
	}
	if sanitizeJSON(0) != 0 {
		t.Error("int changed")
	}
	if sanitizeJSON(nil) != nil {
		t.Error("nil changed")
	}
	if sanitizeJSON(math.Inf(-1)) != nil {
		t.Error("scalar Inf not nullified")
	}
}

func TestPrintJSONEmitsNullInsteadOfError(t *testing.T) {
	var buf bytes.Buffer
	err := printJSON(&buf, map[string]interface{}{
		"code":  "600519.SH",
		"close": math.NaN(),
	})
	if err != nil {
		t.Fatalf("printJSON failed on a NaN cell: %v", err)
	}
	if !strings.Contains(buf.String(), `"close": null`) {
		t.Errorf("NaN cell not emitted as null: %s", buf.String())
	}
}

func TestEarningsAmount(t *testing.T) {
	if got := earningsAmount("每股收益", 0.46); !strings.HasPrefix(got, "0.46元") {
		t.Errorf("per-share band formatted in 亿: %s", got)
	}
	if got := earningsAmount("营业总收入", 12.34e8); !strings.HasPrefix(got, "12.34亿") {
		t.Errorf("revenue band not converted to 亿: %s", got)
	}
}

func TestEarningsBandZeroRendersDash(t *testing.T) {
	if got := earningsBand("净利润", 0); got != "-" {
		t.Errorf("zero band rendered as %q, want -", got)
	}
	if got := earningsBand("净利润", 1e8); !strings.Contains(got, "亿") {
		t.Errorf("non-zero band not rendered: %s", got)
	}
}

func TestChangeOfZeroRendersDash(t *testing.T) {
	if got := changeOf(0, 2); got != "-" {
		t.Errorf("zero growth rendered as %q, want -", got)
	}
	if got := changeOf(12.34, 2); got != "+12.34%" {
		t.Errorf("growth rendered as %q, want +12.34%%", got)
	}
}

func TestRatioZeroDenominator(t *testing.T) {
	if got := ratio(1.0, 0); got != 0 {
		t.Errorf("ratio with zero denominator: got %v, want 0", got)
	}
	if got := ratio(1.0, 2); got != 0.5 {
		t.Errorf("ratio: got %v, want 0.5", got)
	}
}
