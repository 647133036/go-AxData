package cls

import (
	"context"
	"testing"
)

func TestNewCLSAdapter(t *testing.T) {
	a := NewCLSAdapter()
	if a == nil {
		t.Fatal("Adapter is nil")
	}
	if a.Name() != "cls" {
		t.Errorf("Name: got %s, want cls", a.Name())
	}
}

func TestCLSRequestUnknownInterface(t *testing.T) {
	a := NewCLSAdapter()
	_, err := a.Request(context.Background(), map[string]interface{}{
		"interface": "unknown_interface",
	})
	if err == nil {
		t.Fatal("Expected error for unknown interface")
	}
}

func TestCLSRequestNoInterface(t *testing.T) {
	a := NewCLSAdapter()
	_, err := a.Request(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Fatal("Expected error for missing interface")
	}
}

func TestCLSRequestInterfaceNames(t *testing.T) {
	a := NewCLSAdapter()
	supported := []string{
		"cls_market_emotion",
		"cls_market_wind",
		"cls_market_wind_stocks",
		"cls_market_mainline",
		"cls_market_mainline_stocks",
		"cls_market_sector_list",
	}
	for _, name := range supported {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := a.Request(ctx, map[string]interface{}{"interface": name})
		if err == nil {
			// OK, may have gotten cached data
		}
		// Verify no panic
	}
}
