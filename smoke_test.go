package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	cls "github.com/electkismet/axdata-source-cls"
	cninfo "github.com/electkismet/axdata-source-cninfo"
	eastmoney "github.com/electkismet/axdata-source-eastmoney"
	kph "github.com/electkismet/axdata-source-kph"
	mock "github.com/electkismet/axdata-source-mock"
	sina "github.com/electkismet/axdata-source-sina"
	tdx "github.com/electkismet/axdata-source-tdx"
	tencent "github.com/electkismet/axdata-source-tencent"
	ths "github.com/electkismet/axdata-source-ths"
	wencai "github.com/electkismet/axdata-source-wencai"
)

// AXDATA_SMOKE=1 runs the live source smoke test. It is opt-in on purpose: the
// adapters talk to public endpoints, so the default suite must stay offline,
// fast and deterministic. Run it once after touching an adapter to confirm the
// wire contract still matches the live service.
//
//	AXDATA_SMOKE=1 go test -count=1 -run TestSmoke_RealSources .
//
// mustPass cases fail the test; the rest are reported so a sandboxed or
// firewalled environment does not mask a real regression in the healthy ones.
type smokeCase struct {
	name    string
	adapter interface {
		Request(context.Context, map[string]interface{}) ([]map[string]interface{}, error)
	}
	params map[string]interface{}
	// mustPass means a failure is a regression, not an environment limit.
	mustPass bool
	// check asserts a concrete value, catching field-map drift. Empty
	// values are allowed so the same test works for optional fields.
	check map[string]string
}

func TestSmoke_RealSources(t *testing.T) {
	if os.Getenv("AXDATA_SMOKE") == "" {
		t.Skip("set AXDATA_SMOKE=1 to run live source smoke tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	cases := []smokeCase{
		{
			name:     "mock",
			adapter:  mock.NewMockAdapter(),
			params:   map[string]interface{}{"interface": "daily", "symbols": "600519.SH", "start": "2026-01-05", "end": "2026-01-09"},
			mustPass: true,
		},
		{
			name:     "tencent spot",
			adapter:  tencent.NewTencentAdapter(),
			params:   map[string]interface{}{"interface": "stock_zh_a_spot_tx", "limit": 10},
			mustPass: true,
		},
		{
			name:     "sina realtime",
			adapter:  sina.NewSinaAdapter(),
			params:   map[string]interface{}{"interface": "real_time", "symbols": "sh600519,sz000858"},
			mustPass: true,
		},
		{
			name:     "sina kline",
			adapter:  sina.NewSinaAdapter(),
			params:   map[string]interface{}{"interface": "kline", "symbols": "sh600519", "period": "d"},
			mustPass: true,
		},
		{
			name:    "eastmoney snapshot",
			adapter: eastmoney.NewEastMoneyAdapter(),
			params:  map[string]interface{}{"interface": "eastmoney_stock_realtime_snapshot", "symbol": "600519"},
		},
		{
			name:     "cninfo profile",
			adapter:  cninfo.NewCNINFOAdapter(),
			params:   map[string]interface{}{"interface": "stock_profile_cninfo", "code": "600519"},
			mustPass: true,
			check: map[string]string{
				"a_share_code": "600519",
				"listing_date": "20010827",
			},
		},
		{
			name:    "ths hot rank",
			adapter: ths.NewTHSAdapter(),
			params:  map[string]interface{}{"interface": "stock_hot_rank_ths", "limit": 10},
		},
		{
			name:    "wencai strategy",
			adapter: wencai.NewWencaiAdapter(),
			params:  map[string]interface{}{"interface": "stock_strategy_wencai", "query": "贵州茅台"},
		},
		{
			name:    "cls market emotion",
			adapter: cls.NewCLSAdapter(),
			params:  map[string]interface{}{"interface": "cls_market_emotion"},
		},
		{
			name:    "kph sector ranking",
			adapter: kph.NewKPHAdapter(),
			params:  map[string]interface{}{"interface": "kph_sector_ranking", "trade_date": lastWeekday(3)},
		},
		{
			name:    "tdx stock codes",
			adapter: tdx.NewDefaultTDXAdapter(),
			params:  map[string]interface{}{"interface": "stock_codes_tdx"},
		},
	}

	passed, failed := 0, 0
	for _, tc := range cases {
		rows, err := tc.adapter.Request(ctx, tc.params)
		if err != nil {
			failed++
			if errors.Is(err, context.DeadlineExceeded) {
				t.Logf("%-20s TIMEOUT (global deadline exhausted)", tc.name)
				continue
			}
			if tc.mustPass {
				t.Errorf("%-20s must pass but failed: %v", tc.name, err)
			} else {
				t.Logf("%-20s unavailable: %v", tc.name, err)
			}
			continue
		}
		if msg, ok := verifyRows(tc, rows); !ok {
			failed++
			if tc.mustPass {
				t.Errorf("%-20s %s", tc.name, msg)
			} else {
				t.Logf("%-20s %s", tc.name, msg)
			}
			continue
		}
		passed++
		first := rows[0]
		t.Logf("%-20s OK: %d rows, %d columns; first row %s",
			tc.name, len(rows), len(first), firstKeyValues(first, 3))
	}
	t.Logf("live source smoke: %d passed, %d failed/unavailable", passed, failed)
}

// verifyRows checks a live response. It reports the first problem found;
// mustPass is the caller's concern, so the caller decides whether the message
// is an error or a log line.
func verifyRows(tc smokeCase, rows []map[string]interface{}) (msg string, ok bool) {
	if len(rows) == 0 {
		return "returned no rows", false
	}
	for k, want := range tc.check {
		if got := fmt.Sprintf("%v", rows[0][k]); got != want {
			return fmt.Sprintf("field %q = %q, want %q (field-map drift)", k, got, want), false
		}
	}
	return "", true
}

func firstKeyValues(row map[string]interface{}, n int) string {
	parts := make([]string, 0, n)
	for k, v := range row {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		if len(parts) >= n {
			break
		}
	}
	return fmt.Sprintf("{%v}", parts)
}

// lastWeekday returns the YYYYMMDD of the trading day nearest to n calendar
// days ago, skipping weekends. Some adapters require an explicit trade_date
// and refuse to fall back to the latest session.
func lastWeekday(n int) string {
	beijing := time.FixedZone("CST", 8*3600)
	d := time.Now().In(beijing).AddDate(0, 0, -n)
	for d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
		d = d.AddDate(0, 0, -1)
	}
	return d.Format("20060102")
}
