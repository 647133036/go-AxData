package ths

import (
	"context"
	"strings"
	"testing"
)

func TestNewTHSAdapter(t *testing.T) {
	a := NewTHSAdapter()
	if a == nil {
		t.Fatal("Adapter is nil")
	}
	if a.Name() != "ths" {
		t.Errorf("Name: got %s, want ths", a.Name())
	}
}

func TestTHSRequestUnknownInterface(t *testing.T) {
	a := NewTHSAdapter()
	_, err := a.Request(context.Background(), map[string]interface{}{
		"interface": "unknown",
	})
	if err == nil {
		t.Fatal("Expected error for unknown interface")
	}
}

func TestTHSRequestNoInterface(t *testing.T) {
	a := NewTHSAdapter()
	_, err := a.Request(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Fatal("Expected error for missing interface")
	}
}

func TestBuildSecid(t *testing.T) {
	tests := []struct {
		code string
		want string
	}{
		{"600000", "1.600000"},
		{"000001", "0.000001"},
	}
	for _, tc := range tests {
		result := buildSecid(tc.code)
		if result != tc.want {
			t.Errorf("buildSecid(%s): got %s, want %s", tc.code, result, tc.want)
		}
	}
}

func TestCleanCode(t *testing.T) {
	tests := []struct {
		input interface{}
		want  string
	}{
		{"600000", "600000"},
		{"000001", "000001"},
		{nil, ""},
		{"undefined", ""},
		{"null", ""},
		{"6.00.00", ""}, // strips dots, leaves "60000" (5 digits), not 6-digit
		{"12345", ""},   // only 5 digits
	}
	for _, tc := range tests {
		result := cleanCode(tc.input)
		if result != tc.want {
			t.Errorf("cleanCode(%v): got %s, want %s", tc.input, result, tc.want)
		}
	}
}

func TestCleanText(t *testing.T) {
	tests := []struct {
		input interface{}
		want  string
	}{
		{"平安银行", "平安银行"},
		{nil, ""},
		{"null", ""},
		{"", ""},
	}
	for _, tc := range tests {
		result := cleanText(tc.input)
		if result != tc.want {
			t.Errorf("cleanText(%v): got %s, want %s", tc.input, result, tc.want)
		}
	}
}

func TestFloatVal(t *testing.T) {
	tests := []struct {
		input interface{}
		want  float64
	}{
		{float64(3.14), 3.14},
		{int(42), 42.0},
		{"100.5", 100.5},
		{"-", 0.0},
		{nil, 0.0},
	}
	for _, tc := range tests {
		result := floatVal(tc.input)
		if result != tc.want {
			t.Errorf("floatVal(%v): got %v, want %v", tc.input, result, tc.want)
		}
	}
}

func TestBuildQuery(t *testing.T) {
	params := map[string]string{
		"fields": "f2,f3,f4,f12",
		"secids": "1.600000,0.000001",
	}
	result := buildQuery(params)
	if result == "" {
		t.Fatal("buildQuery returned empty string")
	}
	if !strings.Contains(result, "fields") {
		t.Errorf("buildQuery missing fields key")
	}
}
