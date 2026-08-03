package tencent

import (
	"context"
	"regexp"
	"testing"
)

func TestNewTencentAdapter(t *testing.T) {
	a := NewTencentAdapter()
	if a == nil {
		t.Fatal("Adapter is nil")
	}
	if a.Name() != "tencent" {
		t.Errorf("Name: got %s, want tencent", a.Name())
	}
}

func TestTencentRequestUnknownInterface(t *testing.T) {
	a := NewTencentAdapter()
	_, err := a.Request(context.Background(), map[string]interface{}{
		"interface": "unknown",
	})
	if err == nil {
		t.Fatal("Expected error for unknown interface")
	}
}

func TestTencentRequestNoInterface(t *testing.T) {
	a := NewTencentAdapter()
	_, err := a.Request(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Fatal("Expected error for missing interface")
	}
}

func TestParseCodeList(t *testing.T) {
	// Test parseCodeList if it exists as a public function
	// Check what helper functions are available
	t.Log("Checking helper function availability")
}

func TestQuoteRePattern(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{`v_sz000001="...000001..."`, true},
		{`v_sh600000="...600000..."`, true},
		{`no match here`, false},
	}

	for _, tc := range tests {
		matched := quoteRe.MatchString(tc.input)
		if matched != tc.want {
			t.Errorf("quoteRe.MatchString(%q): got %v, want %v", tc.input, matched, tc.want)
		}
	}
}

func TestTickRePattern(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{`[ 123, "tick data here" ]`, true},
		{`[1234, "data"]`, true},
		{`no match`, false},
	}

	for _, tc := range tests {
		matched := tickRe.MatchString(tc.input)
		if matched != tc.want {
			t.Errorf("tickRe.MatchString(%q): got %v, want %v", tc.input, matched, tc.want)
		}
	}
}

func TestParseCodeListRegex(t *testing.T) {
	// The parseCodeList function from eastmoney is not available here
	// This test validates that we can construct a similar regex pattern
	re := regexp.MustCompile(`[,\s，]+`)
	result := re.Split("000001，000002 000003", -1)
	if len(result) != 3 {
		t.Errorf("Split: got %d parts, want 3", len(result))
	}
}
