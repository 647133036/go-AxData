package mock

import (
	"context"
	"math"
	"testing"
)

func TestNewMockAdapter(t *testing.T) {
	a := NewMockAdapter()
	if a == nil {
		t.Fatal("Adapter is nil")
	}
	if a.Name() != "mock" {
		t.Errorf("Name: got %s, want mock", a.Name())
	}
}

func TestMockDaily(t *testing.T) {
	a := NewMockAdapter()
	results, err := a.Request(context.Background(), map[string]interface{}{
		"interface": "daily",
		"symbol":    "000001.SZ",
		"limit":     5,
	})
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if len(results) != 5 {
		t.Errorf("Expected 5 results, got %d", len(results))
	}
	for _, r := range results {
		if _, ok := r["ts_code"]; !ok {
			t.Error("Missing ts_code")
		}
		if _, ok := r["trade_date"]; !ok {
			t.Error("Missing trade_date")
		}
		if _, ok := r["close"]; !ok {
			t.Error("Missing close")
		}

		close := r["close"].(float64)
		vol := r["vol"].(float64)
		amount := r["amount"].(float64)
		if vol <= 0 || amount <= 0 || close <= 0 {
			t.Errorf("non-positive OHLCV component: close=%v vol=%v amount=%v", close, vol, amount)
		}
		// vol is volume/100 and amount is volume*close/1000, so amount must equal
		// vol*close/10. This ties the two fields to the same source volume, which
		// is what an integer-divided vol would break.
		if got := vol * close / 10; math.Abs(got-amount) > amount*1e-6 {
			t.Errorf("amount %v, want vol*close/10 = %v", amount, got)
		}
	}
}

func TestMockQuote(t *testing.T) {
	a := NewMockAdapter()
	results, err := a.Request(context.Background(), map[string]interface{}{
		"interface": "quote",
		"limit":     3,
	})
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(results))
	}
	for _, r := range results {
		if _, ok := r["instrument_id"]; !ok {
			t.Error("Missing instrument_id")
		}
		if _, ok := r["bid"]; !ok {
			t.Error("Missing bid")
		}
	}
}

func TestMockStockList(t *testing.T) {
	a := NewMockAdapter()
	results, err := a.Request(context.Background(), map[string]interface{}{
		"interface": "stock_list",
		"limit":     5,
	})
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if len(results) != 5 {
		t.Errorf("Expected 5 results, got %d", len(results))
	}
	for _, r := range results {
		if _, ok := r["instrument_id"]; !ok {
			t.Error("Missing instrument_id")
		}
		if _, ok := r["name"]; !ok {
			t.Error("Missing name")
		}
	}
}

func TestMockCalendar(t *testing.T) {
	a := NewMockAdapter()
	results, err := a.Request(context.Background(), map[string]interface{}{
		"interface": "calendar",
	})
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("Expected non-zero results")
	}
	for _, r := range results {
		if _, ok := r["cal_date"]; !ok {
			t.Error("Missing cal_date")
		}
		if _, ok := r["is_open"]; !ok {
			t.Error("Missing is_open")
		}
	}
}

func TestMockDefaultLimit(t *testing.T) {
	a := NewMockAdapter()
	results, err := a.Request(context.Background(), map[string]interface{}{
		"interface": "daily",
	})
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	// Default limit is 20
	if len(results) != 20 {
		t.Errorf("Expected 20 results (default limit), got %d", len(results))
	}
}
