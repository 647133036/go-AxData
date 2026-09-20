package collector

import (
	"errors"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestScheduleDueManualAndStartup(t *testing.T) {
	now := time.Date(2026, 1, 6, 12, 0, 0, 0, time.UTC)
	never := time.Time{}

	tests := []struct {
		schedule TaskSchedule
		lastRun  time.Time
		want     bool
	}{
		{TaskSchedule{}, never, false},
		{TaskSchedule{Type: "manual"}, never, false},
		{TaskSchedule{Type: "manual"}, now.Add(-time.Hour), false},
		{TaskSchedule{Type: "startup"}, never, true},
		{TaskSchedule{Type: "startup"}, now.Add(-time.Second), false},
		{TaskSchedule{Type: "unknown_type"}, never, false},
	}
	for _, tt := range tests {
		if got := tt.schedule.Due(now, tt.lastRun); got != tt.want {
			t.Errorf("schedule %+v lastRun=%v Due=%v, want %v", tt.schedule, tt.lastRun, got, tt.want)
		}
	}
}

func TestScheduleDueInterval(t *testing.T) {
	now := time.Date(2026, 1, 6, 12, 0, 0, 0, time.UTC)
	interval := TaskSchedule{Type: "interval", Interval: "5m"}

	tests := []struct {
		name     string
		schedule TaskSchedule
		lastRun  time.Time
		want     bool
	}{
		{"never run", interval, time.Time{}, true},
		{"exactly due", interval, now.Add(-5 * time.Minute), true},
		{"past due", interval, now.Add(-1 * time.Hour), true},
		{"not yet due", interval, now.Add(-1 * time.Minute), false},
		{"future last run", interval, now.Add(time.Minute), false},
		{"missing interval", TaskSchedule{Type: "interval"}, time.Time{}, false},
		{"malformed interval", TaskSchedule{Type: "interval", Interval: "five minutes"}, time.Time{}, false},
		{"zero interval", TaskSchedule{Type: "interval", Interval: "0s"}, time.Time{}, false},
		{"negative interval", TaskSchedule{Type: "interval", Interval: "-5s"}, time.Time{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.schedule.Due(now, tt.lastRun); got != tt.want {
				t.Errorf("Due(now, %v) = %v, want %v", tt.lastRun, got, tt.want)
			}
		})
	}
}

func TestScheduleDueDaily(t *testing.T) {
	now := time.Date(2026, 1, 6, 9, 30, 0, 0, time.UTC)
	daily := TaskSchedule{Type: "daily", Time: "09:00"}

	tests := []struct {
		name     string
		now      time.Time
		schedule TaskSchedule
		lastRun  time.Time
		want     bool
	}{
		{"before target", now.Add(-time.Hour), daily, time.Time{}, false},
		{"at target first run", now, daily, time.Time{}, true},
		{"after target first run", now.Add(time.Hour), daily, time.Time{}, true},
		{"already ran today", now, daily, time.Date(2026, 1, 6, 9, 0, 0, 0, time.UTC), false},
		{"ran yesterday", now, daily, time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC), true},
		{"next day fires again", now.AddDate(0, 0, 1), daily, time.Date(2026, 1, 6, 9, 0, 0, 0, time.UTC), true},
		{"malformed time", now, TaskSchedule{Type: "daily", Time: "9am"}, time.Time{}, false},
		{"missing time defaults to midnight", time.Date(2026, 1, 6, 0, 5, 0, 0, time.UTC), TaskSchedule{Type: "daily"}, time.Time{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.schedule.Due(tt.now, tt.lastRun); got != tt.want {
				t.Errorf("Due(%v, %v) = %v, want %v", tt.now, tt.lastRun, got, tt.want)
			}
		})
	}
}

func TestDueTaskIDsSortedAndEnabledOnly(t *testing.T) {
	c, _ := newTestCollectorWithMock(t)
	now := time.Date(2026, 1, 6, 12, 0, 0, 0, time.UTC)

	mk := func(name string, enabled bool) *Task {
		task, err := c.AddTask(name, "mock-data", "daily", "daily", "core", nil, TaskSchedule{Type: "interval", Interval: "1m"})
		if err != nil {
			t.Fatalf("AddTask %s: %v", name, err)
		}
		if err := c.UpdateTask(task.ID, map[string]interface{}{"enabled": enabled}); err != nil {
			t.Fatalf("UpdateTask %s: %v", name, err)
		}
		return task
	}

	tasks := []*Task{mk("charlie", true), mk("alpha", true), mk("bravo", false), mk("delta", true)}
	manual, err := c.AddTask("manual", "mock-data", "daily", "daily", "core", nil, TaskSchedule{Type: "manual"})
	if err != nil {
		t.Fatalf("AddTask manual: %v", err)
	}
	if err := c.UpdateTask(manual.ID, map[string]interface{}{"enabled": true}); err != nil {
		t.Fatalf("enable manual: %v", err)
	}

	got := c.DueTaskIDs(now)

	// Ground truth straight from the map, so the assertion cannot drift from the
	// fixture: every enabled interval task must be due, and no other task may be.
	want := make(map[string]bool)
	for _, task := range tasks {
		if task.Enabled {
			want[task.ID] = true
		}
	}
	if len(got) != len(want) {
		t.Fatalf("DueTaskIDs = %v, want %d ids", got, len(want))
	}
	seen := make(map[string]bool)
	for i, id := range got {
		if !want[id] {
			t.Fatalf("DueTaskIDs[%d] = %s, which is not an enabled interval task", i, id)
		}
		if i > 0 && got[i-1] > id {
			t.Fatalf("DueTaskIDs not sorted: %s then %s", got[i-1], id)
		}
		seen[id] = true
	}
	for id := range want {
		if !seen[id] {
			t.Fatalf("DueTaskIDs omitted %s", id)
		}
	}

	// An enabled manual task must never be scheduled, even once it has run.
	if err := c.MarkRuns([]string{manual.ID}, now); err != nil {
		t.Fatalf("MarkRuns: %v", err)
	}
	if ids := c.DueTaskIDs(now); len(ids) != len(want) {
		t.Fatalf("DueTaskIDs after a manual run = %v, manual must stay unscheduled", ids)
	}
}

func TestMarkRunsSkipsUnknownAndSetsLastRun(t *testing.T) {
	c, _ := newTestCollectorWithMock(t)
	when := time.Date(2026, 1, 6, 12, 0, 0, 0, time.UTC)

	task, err := c.AddTask("target", "mock-data", "daily", "daily", "core", nil, TaskSchedule{Type: "interval", Interval: "1m"})
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	if err := c.MarkRuns([]string{task.ID, "does-not-exist"}, when); err != nil {
		t.Fatalf("MarkRuns: %v", err)
	}

	got, ok := c.GetTask(task.ID)
	if !ok {
		t.Fatal("task missing after MarkRuns")
	}
	if !got.LastRun.Equal(when) {
		t.Errorf("LastRun = %v, want %v", got.LastRun, when)
	}

	// A task that never ran stays zero, so it remains due.
	idle, err := c.AddTask("idle", "mock-data", "daily", "daily", "core", nil, TaskSchedule{Type: "interval", Interval: "1m"})
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	if !idle.LastRun.IsZero() {
		t.Errorf("fresh task LastRun = %v, want zero", idle.LastRun)
	}
}

func TestUpdateTaskSchedule(t *testing.T) {
	c, _ := newTestCollectorWithMock(t)
	now := time.Date(2026, 1, 6, 12, 0, 0, 0, time.UTC)

	task, err := c.AddTask("reschedule", "mock-data", "daily", "daily", "core", nil, TaskSchedule{Type: "manual"})
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	if err := c.UpdateTask(task.ID, map[string]interface{}{"enabled": true}); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if len(c.DueTaskIDs(now)) != 0 {
		t.Fatal("manual task must not be due")
	}

	// A struct value is the shape the CLI and API use.
	if err := c.UpdateTask(task.ID, map[string]interface{}{
		"schedule": TaskSchedule{Type: "interval", Interval: "1m"},
	}); err != nil {
		t.Fatalf("UpdateTask schedule struct: %v", err)
	}
	if ids := c.DueTaskIDs(now); len(ids) != 1 || ids[0] != task.ID {
		t.Fatalf("DueTaskIDs after struct update = %v, want [%s]", ids, task.ID)
	}

	// A map is the shape a JSON payload decodes into.
	if err := c.UpdateTask(task.ID, map[string]interface{}{
		"schedule": map[string]interface{}{"type": "manual"},
	}); err != nil {
		t.Fatalf("UpdateTask schedule map: %v", err)
	}
	if ids := c.DueTaskIDs(now); len(ids) != 0 {
		t.Fatalf("DueTaskIDs after map update = %v, want none", ids)
	}

	// A wrong-typed value must be ignored, not applied as a zero schedule.
	if err := c.UpdateTask(task.ID, map[string]interface{}{
		"schedule": "not-a-schedule",
	}); err != nil {
		t.Fatalf("UpdateTask bad schedule: %v", err)
	}
	got, ok := c.GetTask(task.ID)
	if !ok {
		t.Fatal("task missing")
	}
	if got.Schedule.Type != "manual" {
		t.Errorf("Schedule = %+v, want the previous manual schedule", got.Schedule)
	}
}

// testClock is a race-safe movable clock for scheduler tests.
type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) Set(t time.Time) {
	c.mu.Lock()
	c.now = t
	c.mu.Unlock()
}

func newScheduledCollector(t *testing.T) (*Collector, *testClock, *Scheduler) {
	t.Helper()
	c, _ := newTestCollectorWithMock(t)

	clock := &testClock{now: time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC)}
	s := NewScheduler(c, zap.NewNop())
	s.SetClock(clock.Now)
	s.SetPollInterval(5 * time.Millisecond)
	return c, clock, s
}

func enableIntervalTask(t *testing.T, c *Collector, name, interval string) *Task {
	t.Helper()
	task, err := c.AddTask(name, "mock-data", "daily", "daily", "core", nil, TaskSchedule{Type: "interval", Interval: interval})
	if err != nil {
		t.Fatalf("AddTask %s: %v", name, err)
	}
	if err := c.UpdateTask(task.ID, map[string]interface{}{"enabled": true}); err != nil {
		t.Fatalf("enable %s: %v", name, err)
	}
	return task
}

func runsForTask(runs []*Run, taskID string) []*Run {
	var out []*Run
	for _, r := range runs {
		if r != nil && r.TaskID == taskID && r.Status == "success" {
			out = append(out, r)
		}
	}
	return out
}

func eventually(t *testing.T, d time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out: %s", msg)
}

func TestSchedulerFiresIntervalOncePerPeriod(t *testing.T) {
	c, clock, s := newScheduledCollector(t)
	task := enableIntervalTask(t, c, "interval-task", "10s")

	s.Start()
	defer s.Stop()

	clock.Set(clock.Now().Add(1 * time.Second))
	eventually(t, 2*time.Second, func() bool { return len(runsForTask(c.ListRuns(), task.ID)) == 1 }, "first run")

	// The clock has not crossed the interval boundary, so no second run is due.
	time.Sleep(200 * time.Millisecond)
	if got := len(runsForTask(c.ListRuns(), task.ID)); got != 1 {
		t.Fatalf("interval task ran %d times within one period, want 1", got)
	}

	// Crossing the interval boundary fires once more.
	clock.Set(clock.Now().Add(11 * time.Second))
	eventually(t, 2*time.Second, func() bool { return len(runsForTask(c.ListRuns(), task.ID)) == 2 }, "second run")
}

func TestSchedulerRetriesFailedRun(t *testing.T) {
	c, clock, s := newScheduledCollector(t)
	// A slower poll keeps the retry count small: the task is due on every tick
	// because a failed run never advances LastRun, so the poll interval is the
	// retry rate.
	s.SetPollInterval(50 * time.Millisecond)

	Register(&errorAdapter{name: "failing-source", err: errors.New("boom")})
	task, err := c.AddTask("failing", "failing-source", "daily", "daily", "core", nil, TaskSchedule{Type: "interval", Interval: "5s"})
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	if err := c.UpdateTask(task.ID, map[string]interface{}{"enabled": true}); err != nil {
		t.Fatalf("enable: %v", err)
	}

	s.Start()
	defer s.Stop()

	// Advance past the interval. The task retries because the failure did not
	// satisfy the schedule.
	clock.Set(clock.Now().Add(60 * time.Second))
	eventually(t, 3*time.Second, func() bool {
		return len(c.ListRuns()) >= 2
	}, "retried failed run")

	got, ok := c.GetTask(task.ID)
	if !ok {
		t.Fatal("task missing")
	}
	if !got.LastRun.IsZero() {
		t.Errorf("LastRun = %v, want zero: a failed run must not satisfy the schedule", got.LastRun)
	}
}

func TestSchedulerIgnoresDisabledAndManualTasks(t *testing.T) {
	c, clock, s := newScheduledCollector(t)

	disabled := enableIntervalTask(t, c, "disabled-task", "1s")
	if err := c.UpdateTask(disabled.ID, map[string]interface{}{"enabled": false}); err != nil {
		t.Fatalf("disable: %v", err)
	}
	manual, err := c.AddTask("manual-task", "mock-data", "daily", "daily", "core", nil, TaskSchedule{Type: "manual"})
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	if err := c.UpdateTask(manual.ID, map[string]interface{}{"enabled": true}); err != nil {
		t.Fatalf("enable: %v", err)
	}

	s.Start()
	defer s.Stop()

	clock.Set(clock.Now().Add(2 * time.Second))
	time.Sleep(150 * time.Millisecond)

	if runs := runsForTask(c.ListRuns(), disabled.ID); len(runs) != 0 {
		t.Errorf("disabled task ran %d times", len(runs))
	}
	if runs := runsForTask(c.ListRuns(), manual.ID); len(runs) != 0 {
		t.Errorf("manual task ran %d times", len(runs))
	}
	if len(c.ListRuns()) != 0 {
		t.Errorf("unexpected runs: %v", c.ListRuns())
	}
}

func TestSchedulerStartupRunsOnce(t *testing.T) {
	c, clock, s := newScheduledCollector(t)
	task := enableIntervalTask(t, c, "startup-task", "1s")
	if err := c.UpdateTask(task.ID, map[string]interface{}{"schedule": TaskSchedule{Type: "startup"}}); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	s.Start()
	defer s.Stop()

	clock.Set(clock.Now().Add(5 * time.Second))
	// Poll for both the run record and the MarkRuns commit. RunTask publishes
	// the run to the map before the scheduler records LastRun, so a fixed
	// sleep can land inside that gap and observe a zero LastRun.
	eventually(t, 3*time.Second, func() bool {
		if len(runsForTask(c.ListRuns(), task.ID)) != 1 {
			return false
		}
		got, ok := c.GetTask(task.ID)
		return ok && !got.LastRun.IsZero()
	}, "startup run recorded with LastRun")
}

func TestSchedulerStopIsIdempotentAndBlocking(t *testing.T) {
	c, _, s := newScheduledCollector(t)
	enableIntervalTask(t, c, "task", "100ms")

	s.Start()
	if !s.Running() {
		t.Fatal("scheduler not running after Start")
	}

	s.Stop()
	s.Stop()
	if s.Running() {
		t.Fatal("scheduler still running after Stop")
	}

	s.Start()
	if !s.Running() {
		t.Fatal("scheduler did not restart after Stop")
	}
	s.Start()
	s.Stop()
}
