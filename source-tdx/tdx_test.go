package tdx

import (
	"context"
	"testing"
	"time"
)

func TestNewTDXAdapter(t *testing.T) {
	a := NewDefaultTDXAdapter()
	if a == nil {
		t.Fatal("Adapter is nil")
	}
	if a.Name() != "tdx" {
		t.Errorf("Name: got %s, want tdx", a.Name())
	}
}

func TestNewTDXAdapterWithHosts(t *testing.T) {
	a := NewTDXAdapter([]string{"127.0.0.1:7709"})
	if a == nil {
		t.Fatal("Adapter is nil")
	}
	if len(a.hosts) != 1 {
		t.Errorf("Hosts: got %d, want 1", len(a.hosts))
	}
}

func TestTDXRequestUnknownInterface(t *testing.T) {
	a := NewDefaultTDXAdapter()
	_, err := a.Request(context.Background(), map[string]interface{}{
		"interface": "unknown_interface",
	})
	if err == nil {
		t.Fatal("Expected error for unknown interface")
	}
}

func TestTDXRequestNoInterface(t *testing.T) {
	a := NewDefaultTDXAdapter()
	_, err := a.Request(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Fatal("Expected error for missing interface")
	}
}

func TestCommandFromString(t *testing.T) {
	tests := []struct {
		name string
		want uint16
	}{
		{"kline", CMD_KLINES},
		{"security_list", CMD_SECURITY_LIST},
		{"category_quotes", CMD_CATEGORY_QUOTES},
		{"auction_process", CMD_AUCTION_PROCESS},
		{"stock_kline_daily_tdx", CMD_KLINES},
	}

	for _, tc := range tests {
		cmd, err := commandFromString(tc.name)
		if err != nil {
			t.Fatalf("commandFromString(%q): %v", tc.name, err)
		}
		if cmd != tc.want {
			t.Errorf("commandFromString(%q): got 0x%x, want 0x%x", tc.name, cmd, tc.want)
		}
	}

	_, err := commandFromString("unknown")
	if err == nil {
		t.Fatal("commandFromString(unknown): expected error")
	}
}

func TestPeriodPairForInterface(t *testing.T) {
	tests := []struct {
		iface string
		want  PeriodPair
	}{
		{"kline_daily", PeriodPair{PERIOD_DAILY, 1}},
		{"stock_kline_daily_tdx", PeriodPair{PERIOD_DAILY, 1}},
	}

	for _, tc := range tests {
		result := periodPairForInterface(tc.iface, map[string]interface{}{})
		if result != tc.want {
			t.Errorf("periodPairForInterface(%q): got %+v, want %+v", tc.iface, result, tc.want)
		}
	}
}

func TestMarketFromString(t *testing.T) {
	tests := []struct {
		s    string
		want uint8
	}{
		{"0", MARKET_SZ},
		{"1", MARKET_SH},
		{"2", MARKET_BJ},
	}

	for _, tc := range tests {
		result := marketFromString(tc.s)
		if result != tc.want {
			t.Errorf("marketFromString(%q): got %d, want %d", tc.s, result, tc.want)
		}
	}
}

func TestMarketToCode(t *testing.T) {
	tests := []struct {
		market int
		want   string
	}{
		{0, "sz"},
		{1, "sh"},
		{2, "bj"},
	}

	for _, tc := range tests {
		result := marketToCode(tc.market)
		if result != tc.want {
			t.Errorf("marketToCode(%d): got %s, want %s", tc.market, result, tc.want)
		}
	}
}

func TestMarketToExchange(t *testing.T) {
	tests := []struct {
		market int
		want   string
	}{
		{0, "SZSE"},
		{1, "SSE"},
		{2, "BSE"},
	}

	for _, tc := range tests {
		result := marketToExchange(tc.market)
		if result != tc.want {
			t.Errorf("marketToExchange(%d): got %s, want %s", tc.market, result, tc.want)
		}
	}
}

func TestSplitCSV(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"000001,000002", []string{"000001", "000002"}},
		{"000001", []string{"000001"}},
	}

	for _, tc := range tests {
		result := splitCSV(tc.input)
		if len(result) != len(tc.want) {
			t.Errorf("splitCSV(%q): got %d parts, want %d", tc.input, len(result), len(tc.want))
			continue
		}
		for i, s := range result {
			if s != tc.want[i] {
				t.Errorf("splitCSV(%q)[%d]: got %s, want %s", tc.input, i, s, tc.want[i])
			}
		}
	}
}

func TestStrval(t *testing.T) {
	params := map[string]interface{}{
		"key1": "value1",
	}
	if strval(params, "key1", "default") != "value1" {
		t.Errorf("strval: got unexpected value")
	}
	if strval(params, "missing", "default") != "default" {
		t.Errorf("strval: got unexpected default")
	}
}

func TestIntval(t *testing.T) {
	params := map[string]interface{}{
		"key1": 42,
		"str1": "7",
	}
	if intval(params, "key1", 0) != 42 {
		t.Errorf("intval(int): got unexpected value")
	}
	if intval(params, "str1", 0) != 7 {
		t.Errorf("intval(string): got unexpected value")
	}
	if intval(params, "missing", 99) != 99 {
		t.Errorf("intval(missing): got unexpected value")
	}
}

func TestBuildLimitLadderRequest(t *testing.T) {
	req, err := buildLimitLadderRequest(map[string]interface{}{
		"stock_code": "000001.SZ",
	})
	if err != nil {
		t.Fatalf("buildLimitLadderRequest failed: %v", err)
	}
	if req == nil {
		t.Fatal("Request is nil")
	}
}

func TestBuildThemeStrengthRequest(t *testing.T) {
	req, err := buildThemeStrengthRequest(map[string]interface{}{})
	if err != nil {
		t.Fatalf("buildThemeStrengthRequest failed: %v", err)
	}
	if req == nil {
		t.Fatal("Request is nil")
	}
}

func TestBuildQuotesRequest(t *testing.T) {
	req, err := buildQuotesRequest(map[string]interface{}{
		"market": "sz",
		"code":   "000001",
	}, false)
	if err != nil {
		t.Fatalf("buildQuotesRequest failed: %v", err)
	}
	if req == nil {
		t.Fatal("Request is nil")
	}
}

func TestBuildSecurityListRequest(t *testing.T) {
	req, err := buildSecurityListRequest(map[string]interface{}{})
	if err != nil {
		t.Fatalf("buildSecurityListRequest failed: %v", err)
	}
	if req == nil {
		t.Fatal("Request is nil")
	}
}

func TestBuildCategoryQuotesRequest(t *testing.T) {
	req, err := buildCategoryQuotesRequest(map[string]interface{}{
		"sort": "5",
	})
	if err != nil {
		t.Fatalf("buildCategoryQuotesRequest failed: %v", err)
	}
	if req == nil {
		t.Fatal("Request is nil")
	}
}

func TestEncodeRequest(t *testing.T) {
	req := &WireRequest{
		Command: CMD_HEARTBEAT,
		Payload: []byte("test"),
	}
	data, err := encodeRequest(req)
	if err != nil {
		t.Fatalf("encodeRequest failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("Encoded data is empty")
	}
}

func TestParseSecurityCountRows(t *testing.T) {
	resp := &WireResponse{
		Command: CMD_SECURITY_COUNT,
		MsgID:   1,
		Data:    []byte{5, 0}, // count = 5 in little-endian
	}
	result, err := parseSecurityCountRows(resp)
	if err != nil {
		t.Fatalf("parseSecurityCountRows failed: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(result))
	}
	if result[0]["count"] != 5 {
		t.Errorf("count: got %v, want 5", result[0]["count"])
	}
}

func TestCompactFloat(t *testing.T) {
	result := compactFloat(0)
	if result != 0.0 {
		t.Errorf("compactFloat(0): got %v, want 0.0", result)
	}
}

func TestGBK2Str(t *testing.T) {
	result := gbk2str([]byte{0x00, 0x00})
	if result != "" {
		t.Errorf("gbk2str: got %q, want empty", result)
	}
}

func TestAscii2Str(t *testing.T) {
	result := ascii2str([]byte("test"))
	if result != "test" {
		t.Errorf("ascii2str: got %q, want test", result)
	}
}

func TestDeadlineFromContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result := deadlineFromContext(ctx, 3)
	if result.IsZero() {
		t.Fatal("deadlineFromContext returned zero time")
	}
}
