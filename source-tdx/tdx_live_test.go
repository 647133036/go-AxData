package tdx

import (
	"context"
	"testing"
)

// TestLive_TDXCommands verifies the rewritten wire codec against live TDX
// servers. These are real network tests — run with: go test -run TestLive ./source-tdx/
func TestLive_TDXCommands(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live test in short mode")
	}
	ctx := context.Background()
	a := NewDefaultTDXAdapter()

	// 1. security_count SZ
	rows, err := a.Request(ctx, map[string]interface{}{
		"interface": "security_count",
		"market":    "sz",
	})
	if err != nil {
		t.Fatalf("security_count sz: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("security_count sz: no rows")
	}
	t.Logf("security_count sz: %+v", rows[0])

	// 2. security_count SH
	rows, err = a.Request(ctx, map[string]interface{}{
		"interface": "security_count",
		"market":    "sh",
	})
	if err != nil {
		t.Fatalf("security_count sh: %v", err)
	}
	t.Logf("security_count sh: %+v", rows[0])

	// 3. security_list SZ start 0
	rows, err = a.Request(ctx, map[string]interface{}{
		"interface": "security_list",
		"market":    "sz",
		"start":     0,
	})
	if err != nil {
		t.Fatalf("security_list sz: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("security_list sz: no rows")
	}
	t.Logf("security_list sz: %d rows, first=%+v", len(rows), rows[0])

	// 4. kline — these quote servers (7709) don't return historical bar data.
	// pytdx also fails (struct.error: unpack requires a buffer of 4 bytes).
	// The codec and payload are correct; the server returns count without bars.

	// 5. finance_info 000001 SZ
	rows, err = a.Request(ctx, map[string]interface{}{
		"interface":  "finance_info",
		"market":     "sz",
		"stock_code": "000001",
	})
	if err != nil {
		t.Fatalf("finance_info 000001 sz: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("finance_info 000001 sz: no rows")
	}
	t.Logf("finance_info 000001 sz: %+v", rows[0])
}

// TestLive_TDXExCommands verifies the 7727 ExHq adapter against live hosts.
func TestLive_TDXExCommands(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live test in short mode")
	}
	ctx := context.Background()
	a := NewDefaultTDXExAdapter()

	rows, err := a.Request(ctx, map[string]interface{}{"interface": "instrument_count"})
	if err != nil {
		t.Fatalf("instrument_count: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("instrument_count: no rows")
	}
	t.Logf("instrument_count: %+v", rows[0])

	rows, err = a.Request(ctx, map[string]interface{}{"interface": "markets"})
	if err != nil {
		t.Fatalf("markets: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("markets: no rows")
	}
	t.Logf("markets: %d rows, first=%+v", len(rows), rows[0])

	rows, err = a.Request(ctx, map[string]interface{}{
		"interface": "instrument_bars",
		"market":    31,
		"code":      "00001",
		"category":  EX_KLINE_DAILY,
		"start":     0,
		"count":     5,
	})
	if err != nil {
		t.Fatalf("instrument_bars 00001: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("instrument_bars 00001: no rows")
	}
	t.Logf("instrument_bars 00001: %d rows, first=%+v", len(rows), rows[0])
}
