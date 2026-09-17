package collector

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Due reports whether the schedule should fire now, given the task's last
// successful run. A zero lastRun means the task has never run, which is itself
// due for every schedule except manual: an interval task should not wait its
// first period after the scheduler starts, and a daily task should still fire
// if the wall clock is already past its target time.
//
// The comparison is inclusive on the elapsed side: at exactly lastRun+interval
// the task is due. Sub-second drift does not matter for interval schedules.
func (s TaskSchedule) Due(now time.Time, lastRun time.Time) bool {
	switch s.Type {
	case "", "manual":
		return false
	case "startup":
		return lastRun.IsZero()
	case "interval":
		d, err := time.ParseDuration(s.Interval)
		if err != nil || d <= 0 {
			return false
		}
		if lastRun.IsZero() {
			return true
		}
		return !now.Before(lastRun.Add(d))
	case "daily":
		target, err := dailyTarget(s.Time, now)
		if err != nil {
			return false
		}
		return !now.Before(target) && lastRun.Before(target)
	default:
		return false
	}
}

// dailyTarget maps "HH:MM" onto the calendar day that contains now, keeping
// now's location so a task configured for Asia/Shanghai fires on local time.
func dailyTarget(hhmm string, now time.Time) (time.Time, error) {
	if hhmm == "" {
		hhmm = "00:00"
	}
	hm, err := time.Parse("15:04", hhmm)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid schedule time %q: %w", hhmm, err)
	}
	return time.Date(now.Year(), now.Month(), now.Day(), hm.Hour(), hm.Minute(), 0, 0, now.Location()), nil
}

// scheduleFromMap builds a TaskSchedule from an untyped update value. It
// accepts either a TaskSchedule or a map, so the same updates map can come
// from a CLI flag or a JSON payload. nil means the value was not usable.
func scheduleFromMap(v interface{}) *TaskSchedule {
	if s, ok := v.(TaskSchedule); ok {
		return &s
	}
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil
	}
	s := &TaskSchedule{}
	for k, val := range m {
		str, ok := val.(string)
		if !ok {
			continue
		}
		switch k {
		case "type":
			s.Type = str
		case "interval":
			s.Interval = str
		case "time":
			s.Time = str
		}
	}
	return s
}

// Scheduler fires enabled tasks according to their TaskSchedule.
type Scheduler struct {
	collector *Collector
	logger    *zap.Logger
	poll      time.Duration
	now       func() time.Time

	mu      sync.Mutex
	running bool
	stopCh  chan struct{}
	doneCh  chan struct{}
}

// NewScheduler creates a scheduler that polls every second. The clock is
// injectable so tests can advance time without sleeping.
func NewScheduler(c *Collector, logger *zap.Logger) *Scheduler {
	return &Scheduler{
		collector: c,
		logger:    logger,
		poll:      time.Second,
		now:       time.Now,
	}
}

// SetClock overrides the wall clock. It must be called before Start.
func (s *Scheduler) SetClock(now func() time.Time) {
	if now == nil {
		now = time.Now
	}
	s.now = now
}

// SetPollInterval overrides the poll period. It must be called before Start.
func (s *Scheduler) SetPollInterval(d time.Duration) {
	if d > 0 {
		s.poll = d
	}
}

// Start launches the poll loop. It is idempotent and the first tick fires
// immediately, which is what runs startup tasks and first interval runs.
func (s *Scheduler) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.stopCh = make(chan struct{})
	s.doneCh = make(chan struct{})
	s.running = true
	stopCh := s.stopCh
	s.mu.Unlock()

	go s.loop(stopCh)
}

// Stop closes the poll loop and waits for it to exit. It is safe to call when
// the scheduler was never started or has already stopped.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	close(s.stopCh)
	doneCh := s.doneCh
	s.running = false
	s.mu.Unlock()
	<-doneCh
}

// Running reports whether the poll loop is live.
func (s *Scheduler) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func (s *Scheduler) loop(stopCh chan struct{}) {
	defer close(s.doneCh)

	s.tick()

	ticker := time.NewTicker(s.poll)
	defer ticker.Stop()
	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			s.tick()
		}
	}
}

// tick runs every enabled task whose schedule is due. Successful runs record
// their timestamp so the schedule does not fire again inside the same period.
// Failed runs leave LastRun untouched and therefore retry on the next tick:
// the poll interval is the retry rate, so a failing task is retried once per
// tick with no backoff.
func (s *Scheduler) tick() {
	now := s.now()
	ids := s.collector.DueTaskIDs(now)
	if len(ids) == 0 {
		return
	}

	s.logger.Info("scheduled run", zap.Int("tasks", len(ids)))

	runs, err := s.collector.RunTasks(context.Background(), ids)
	if err != nil {
		s.logger.Warn("scheduled run had failures", zap.Error(err))
	}

	done := make([]string, 0, len(runs))
	for _, r := range runs {
		if r != nil && r.Status == "success" {
			done = append(done, r.TaskID)
		}
	}
	if len(done) > 0 {
		if err := s.collector.MarkRuns(done, now); err != nil {
			s.logger.Warn("record scheduled run failed", zap.Error(err))
		}
	}
}

// DueTaskIDs returns the IDs of enabled tasks whose schedule is due, sorted so
// callers observe a stable order across ticks.
func (c *Collector) DueTaskIDs(now time.Time) []string {
	c.tasksMu.RLock()
	defer c.tasksMu.RUnlock()

	var ids []string
	for _, t := range c.tasks {
		if t.Enabled && t.Schedule.Due(now, t.LastRun) {
			ids = append(ids, t.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

// MarkRuns records now as the last successful run for each task. It is called
// only with tasks that ran successfully, so the metadata snapshot never
// contains a schedule that looks satisfied by a failed run.
func (c *Collector) MarkRuns(taskIDs []string, when time.Time) error {
	c.tasksMu.Lock()
	marked := 0
	for _, id := range taskIDs {
		if t, ok := c.tasks[id]; ok {
			t.LastRun = when
			t.UpdatedAt = when
			marked++
		}
	}
	c.tasksMu.Unlock()

	if marked == 0 {
		return nil
	}
	return c.saveMetadata()
}
