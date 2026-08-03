package collector

import (
	"os"
	"testing"

	"github.com/electkismet/axdata-go/core/config"
	"github.com/electkismet/axdata-go/core/storage"
	"go.uber.org/zap"
)

func TestCollectorFullWorkflow(t *testing.T) {
	tmpDir := "/tmp/test-axdata-workflow-" + t.Name()
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	cfg := config.DefaultConfig(tmpDir)
	store := storage.NewStore(cfg)
	logger, _ := zap.NewDevelopment()

	collector, err := NewCollector(cfg, store, logger)
	if err != nil {
		t.Fatalf("NewCollector failed: %v", err)
	}

	task, err := collector.AddTask(
		"tdx-daily-test",
		"tdx",
		"stock_kline_daily_tdx",
		"daily",
		"core",
		map[string]interface{}{
			"instrument_id": "000001.SZ",
			"end_date":      "2024-01-15",
			"start_date":    "2024-01-01",
		},
	)
	if err != nil {
		t.Fatalf("AddTask failed: %v", err)
	}
	if task.ID == "" {
		t.Fatal("Task ID is empty")
	}
	if task.Interface != "stock_kline_daily_tdx" {
		t.Errorf("Interface: got %s, want stock_kline_daily_tdx", task.Interface)
	}
}
