package collector

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/electkismet/axdata-go/core/config"
	"github.com/electkismet/axdata-go/core/storage"
	"go.uber.org/zap"
)

// flakyAdapter fails for the first failN Request calls, then succeeds.
// It counts every call via calls and records ctx errors via ctxErrors.
type flakyAdapter struct {
	name   string
	failN  int32
	calls  int32
	delays bool
	mu     sync.Mutex
}

func (a *flakyAdapter) Name() string        { return a.name }
func (a *flakyAdapter) Description() string { return "flaky adapter for retry tests" }
func (a *flakyAdapter) Request(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	n := atomic.AddInt32(&a.calls, 1)
	if n <= a.failN {
		return nil, fmt.Errorf("simulated failure %d", n)
	}
	out := make([]map[string]interface{}, 1)
	out[0] = map[string]interface{}{
		"ts_code": "000001.SZ",
		"vol":     float64(1000),
		"open":    float64(10.0),
		"high":    float64(11.0),
		"low":     float64(9.5),
		"close":   float64(10.8),
	}
	return out, nil
}

// slowAdapter blocks until ctx is cancelled or a short sleep elapses.
type slowAdapter struct {
	name string
	mu   sync.Mutex
	calls int32
}

func (a *slowAdapter) Name() string        { return a.name }
func (a *slowAdapter) Description() string { return "slow adapter for timeout tests" }
func (a *slowAdapter) Request(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	atomic.AddInt32(&a.calls, 1)
	select {
	case <-time.After(10 * time.Second):
		return nil, errors.New("should have been cancelled")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// newRetryCollector builds a collector on a temp dir with the given
// RetryCount and TimeoutMs, and registers adapter.
func newRetryCollector(t *testing.T, retryCount, timeoutMs int) (*Collector, *flakyAdapter) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "test-axdata-retry-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	cfg := config.DefaultConfig(tmpDir)
	cfg.Collector.RetryCount = retryCount
	cfg.Collector.TimeoutMs = timeoutMs
	store := storage.NewStore(cfg)
	col, err := NewCollector(cfg, store, zap.NewNop())
	if err != nil {
		t.Fatalf("NewCollector: %v", err)
	}
	adapter := &flakyAdapter{name: "flaky-src"}
	Register(adapter)
	return col, adapter
}

func addFlakyTask(t *testing.T, col *Collector) string {
	t.Helper()
	task, err := col.AddTask(
		"flaky-task", "flaky-src", "daily", "daily", "core",
		map[string]interface{}{"ts_code": "000001.SZ"},
		TaskSchedule{},
	)
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	task.Enabled = true
	return task.ID
}

// TestRetryCountSucceedsAfterFailures: with RetryCount=2 and an adapter that
// fails twice, the run succeeds on the third attempt.
func TestRetryCountSucceedsAfterFailures(t *testing.T) {
	col, adapter := newRetryCollector(t, 2, 0)
	adapter.failN = 2
	id := addFlakyTask(t, col)

	run, err := col.RunTask(context.Background(), id)
	if err != nil {
		t.Fatalf("RunTask: %v", err)
	}
	if run.Status != "success" {
		t.Fatalf("status = %s, want success", run.Status)
	}
	if got := atomic.LoadInt32(&adapter.calls); got != 3 {
		t.Fatalf("adapter calls = %d, want 3 (2 failures + 1 success)", got)
	}
}

// TestRetryCountExhausted: with RetryCount=2 and an adapter that always fails,
// the run fails after 3 total attempts.
func TestRetryCountExhausted(t *testing.T) {
	col, adapter := newRetryCollector(t, 2, 0)
	adapter.failN = 100
	id := addFlakyTask(t, col)

	run, err := col.RunTask(context.Background(), id)
	if err == nil {
		t.Fatal("RunTask should fail when all retries exhausted")
	}
	if run.Status != "failed" {
		t.Fatalf("status = %s, want failed", run.Status)
	}
	if run.Error == "" {
		t.Error("failed run did not record its error")
	}
	if got := atomic.LoadInt32(&adapter.calls); got != 3 {
		t.Fatalf("adapter calls = %d, want 3 (1 + RetryCount=2)", got)
	}
}

// TestRetryCountZeroNoRetry: with RetryCount=0, a failing adapter is called
// exactly once and the run fails.
func TestRetryCountZeroNoRetry(t *testing.T) {
	col, adapter := newRetryCollector(t, 0, 0)
	adapter.failN = 100
	id := addFlakyTask(t, col)

	run, err := col.RunTask(context.Background(), id)
	if err == nil {
		t.Fatal("RunTask should fail")
	}
	if run.Status != "failed" {
		t.Fatalf("status = %s, want failed", run.Status)
	}
	if got := atomic.LoadInt32(&adapter.calls); got != 1 {
		t.Fatalf("adapter calls = %d, want 1 (no retries)", got)
	}
}

// TestTimeoutMsCancelsLongRequest: with TimeoutMs=50 and an adapter that
// sleeps 10s, the request is cancelled well before the sleep finishes.
func TestTimeoutMsCancelsLongRequest(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-axdata-timeout-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	cfg := config.DefaultConfig(tmpDir)
	cfg.Collector.TimeoutMs = 50
	store := storage.NewStore(cfg)
	col, err := NewCollector(cfg, store, zap.NewNop())
	if err != nil {
		t.Fatalf("NewCollector: %v", err)
	}
	slow := &slowAdapter{name: "slow-src"}
	Register(slow)

	task, err := col.AddTask(
		"slow-task", "slow-src", "daily", "daily", "core",
		map[string]interface{}{"ts_code": "000001.SZ"},
		TaskSchedule{},
	)
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	task.Enabled = true

	start := time.Now()
	run, err := col.RunTask(context.Background(), task.ID)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("RunTask should fail on timeout")
	}
	if run.Status != "failed" {
		t.Fatalf("status = %s, want failed", run.Status)
	}
	// 50ms timeout + overhead, but must be well under the 10s sleep.
	if elapsed > 2*time.Second {
		t.Fatalf("elapsed = %v, want < 2s (timeout should cancel early)", elapsed)
	}
}

// TestRetryCountWithTimeout: retry+timeout interact correctly — each retry
// attempt gets its own timeout, and the run fails after all retries.
func TestRetryCountWithTimeout(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-axdata-retry-timeout-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	cfg := config.DefaultConfig(tmpDir)
	cfg.Collector.RetryCount = 1
	cfg.Collector.TimeoutMs = 50
	store := storage.NewStore(cfg)
	col, err := NewCollector(cfg, store, zap.NewNop())
	if err != nil {
		t.Fatalf("NewCollector: %v", err)
	}
	slow := &slowAdapter{name: "slow-src2"}
	Register(slow)

	task, err := col.AddTask(
		"slow-retry-task", "slow-src2", "daily", "daily", "core",
		map[string]interface{}{"ts_code": "000001.SZ"},
		TaskSchedule{},
	)
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	task.Enabled = true

	run, err := col.RunTask(context.Background(), task.ID)
	if err == nil {
		t.Fatal("RunTask should fail")
	}
	if run.Status != "failed" {
		t.Fatalf("status = %s, want failed", run.Status)
	}
	// 2 attempts (1 + RetryCount=1), each capped at 50ms.
	if got := atomic.LoadInt32(&slow.calls); got != 2 {
		t.Fatalf("slow adapter calls = %d, want 2 (1 + RetryCount=1)", got)
	}
}
