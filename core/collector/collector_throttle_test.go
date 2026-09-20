package collector

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/electkismet/axdata-go/core/config"
	"github.com/electkismet/axdata-go/core/storage"
	"go.uber.org/zap"
)

// rowsAdapter returns a fixed number of daily-shaped rows and records the
// wall-clock time of each Request call, so a test can assert on row counts and
// the spacing between requests.
type rowsAdapter struct {
	name  string
	rows  int
	mu    sync.Mutex
	calls []time.Time
}

func (a *rowsAdapter) Name() string        { return a.name }
func (a *rowsAdapter) Description() string { return "rows adapter for throttle tests" }
func (a *rowsAdapter) Request(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	a.mu.Lock()
	a.calls = append(a.calls, time.Now())
	a.mu.Unlock()
	out := make([]map[string]interface{}, a.rows)
	for i := 0; i < a.rows; i++ {
		out[i] = map[string]interface{}{
			"ts_code": "000001.SZ",
			"vol":     float64(1000),
			"pct_chg": float64(2.5),
			"open":    float64(10.0),
			"high":    float64(11.0),
			"low":     float64(9.5),
			"close":   float64(10.8),
		}
	}
	return out, nil
}

func (a *rowsAdapter) callTimes() []time.Time {
	a.mu.Lock()
	defer a.mu.Unlock()
	cp := make([]time.Time, len(a.calls))
	copy(cp, a.calls)
	return cp
}

// newThrottleCollector builds a collector on a temp dir with the given
// BatchSize and RequestIntervalMs, and registers adapter.
func newThrottleCollector(t *testing.T, batchSize, intervalMs int) (*Collector, *rowsAdapter) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "test-axdata-throttle-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	cfg := config.DefaultConfig(tmpDir)
	cfg.Collector.BatchSize = batchSize
	cfg.Collector.RequestIntervalMs = intervalMs
	store := storage.NewStore(cfg)
	col, err := NewCollector(cfg, store, zap.NewNop())
	if err != nil {
		t.Fatalf("NewCollector: %v", err)
	}
	adapter := &rowsAdapter{name: "rows-src", rows: 10}
	Register(adapter)
	return col, adapter
}

func addRowsTask(t *testing.T, col *Collector) string {
	t.Helper()
	task, err := col.AddTask(
		"rows-task", "rows-src", "daily", "daily", "core",
		map[string]interface{}{"ts_code": "000001.SZ"},
		TaskSchedule{},
	)
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	task.Enabled = true
	return task.ID
}

// TestBatchSizeCapsRows: a source returning more rows than BatchSize lands only
// BatchSize rows in storage.
func TestBatchSizeCapsRows(t *testing.T) {
	col, _ := newThrottleCollector(t, 3, 0)
	id := addRowsTask(t, col)

	run, err := col.RunTask(context.Background(), id)
	if err != nil {
		t.Fatalf("RunTask: %v", err)
	}
	if run.Rows != 3 {
		t.Fatalf("RunTask rows = %d, want 3 (batch_size cap)", run.Rows)
	}

	got, err := col.store.Count("core", "daily")
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if got != 3 {
		t.Fatalf("stored rows = %d, want 3", got)
	}
}

// TestBatchSizeZeroKeepsAllRows: with no cap, every fetched row lands.
func TestBatchSizeZeroKeepsAllRows(t *testing.T) {
	col, _ := newThrottleCollector(t, 0, 0)
	id := addRowsTask(t, col)

	run, err := col.RunTask(context.Background(), id)
	if err != nil {
		t.Fatalf("RunTask: %v", err)
	}
	if run.Rows != 10 {
		t.Fatalf("RunTask rows = %d, want 10", run.Rows)
	}
}

// TestRequestIntervalMsThrottlesSequential: two sequential runs of one source
// are spaced at least RequestIntervalMs apart.
func TestRequestIntervalMsThrottlesSequential(t *testing.T) {
	interval := 60
	col, adapter := newThrottleCollector(t, 0, interval)
	id := addRowsTask(t, col)

	if _, err := col.RunTask(context.Background(), id); err != nil {
		t.Fatalf("RunTask #1: %v", err)
	}
	if _, err := col.RunTask(context.Background(), id); err != nil {
		t.Fatalf("RunTask #2: %v", err)
	}

	times := adapter.callTimes()
	if len(times) < 2 {
		t.Fatalf("want >=2 recorded requests, got %d", len(times))
	}
	gap := times[1].Sub(times[0])
	if gap < time.Duration(interval)*time.Millisecond {
		t.Fatalf("gap between sequential requests = %v, want >= %vms", gap, interval)
	}
}

// TestRequestIntervalMsThrottlesConcurrent: concurrent runs of one source still
// space out at the configured rate, not all bursting at once. The gap threshold
// carries slack for timer/scheduler jitter: the assertion proves the runs did
// not burst (a burst is ~0ms apart), not that the timer is sub-millisecond.
func TestRequestIntervalMsThrottlesConcurrent(t *testing.T) {
	interval := 80
	col, adapter := newThrottleCollector(t, 0, interval)
	id := addRowsTask(t, col)

	const n = 4
	var wg sync.WaitGroup
	var fail int32
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := col.RunTask(context.Background(), id); err != nil {
				atomic.StoreInt32(&fail, 1)
			}
		}()
	}
	wg.Wait()
	if atomic.LoadInt32(&fail) == 1 {
		t.Fatal("at least one concurrent RunTask failed")
	}

	times := adapter.callTimes()
	if len(times) != n {
		t.Fatalf("recorded %d requests, want %d", len(times), n)
	}
	minGap := time.Duration(interval) * time.Millisecond * 8 / 10
	for i := 1; i < len(times); i++ {
		gap := times[i].Sub(times[i-1])
		if gap < minGap {
			t.Fatalf("concurrent request %d->%d gap = %v, want >= %v", i-1, i, gap, minGap)
		}
	}
}

// TestRequestIntervalZeroNoThrottle: a zero interval disables pacing entirely,
// so no scheduled start time is ever recorded for the source. That map check is
// the load-independent proof; the gap check below carries slack for timer and
// scheduler jitter and only rules out a real wait.
func TestRequestIntervalZeroNoThrottle(t *testing.T) {
	col, adapter := newThrottleCollector(t, 0, 0)
	id := addRowsTask(t, col)

	if _, err := col.RunTask(context.Background(), id); err != nil {
		t.Fatalf("RunTask #1: %v", err)
	}
	if _, err := col.RunTask(context.Background(), id); err != nil {
		t.Fatalf("RunTask #2: %v", err)
	}

	col.throttleMu.Lock()
	scheduled, throttled := col.throttle["rows-src"]
	col.throttleMu.Unlock()
	if throttled {
		t.Fatalf("interval=0 recorded a scheduled start time: %v", scheduled)
	}

	times := adapter.callTimes()
	if len(times) < 2 {
		t.Fatalf("want >=2 recorded requests, got %d", len(times))
	}
	const maxGap = 500 * time.Millisecond
	if gap := times[1].Sub(times[0]); gap >= maxGap {
		t.Fatalf("gap = %v with interval=0, want < %v (pacing should not wait)", gap, maxGap)
	}
}
