package collector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/electkismet/axdata-go/core/config"
	"github.com/electkismet/axdata-go/core/schema"
	"github.com/electkismet/axdata-go/core/source"
	"github.com/electkismet/axdata-go/core/storage"
	"go.uber.org/zap"
	"golang.org/x/sync/semaphore"
)

// Task defines a collection task.
type Task struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Source    string                 `json:"source"`
	Interface string                 `json:"interface"`
	Table     string                 `json:"table"`
	Layer     string                 `json:"layer"`
	Enabled   bool                   `json:"enabled"`
	Schedule  TaskSchedule           `json:"schedule"`
	Params    map[string]interface{} `json:"params"`
	// LastRun is set by the scheduler after a successful run and is what makes
	// an interval or daily schedule fire only once per period. Manual runs do
	// not update it, so they never suppress a scheduled run.
	LastRun   time.Time `json:"last_run"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TaskSchedule defines when a task runs.
type TaskSchedule struct {
	Type     string `json:"type"`               // manual, interval, daily, startup
	Interval string `json:"interval,omitempty"` // duration for interval, e.g. "5m"
	Time     string `json:"time,omitempty"`     // wall clock for daily, "HH:MM"
}

// Run represents a single task execution.
type Run struct {
	TaskID    string    `json:"task_id"`
	RunID     string    `json:"run_id"`
	Status    string    `json:"status"` // pending, running, success, failed, cancelled
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
	Rows      int       `json:"rows"`
	Error     string    `json:"error"`
}

// Collector manages tasks and runs.
type Collector struct {
	logger  *zap.Logger
	config  *config.Config
	store   *storage.Store
	tasks   map[string]*Task
	tasksMu sync.RWMutex
	runs    map[string]*Run
	runsMu  sync.RWMutex
	// saveMu serializes metadata file writes. A snapshot is taken while saveMu
	// is held, so the last writer always holds the newest union of task and run
	// changes; without it, two saves could interleave and the earlier write
	// could clobber a later mutation.
	saveMu    sync.Mutex
	semaphore *semaphore.Weighted
}

// newIDSeq allocates monotonically increasing ID suffixes. Run and task IDs are
// timestamp-based, and two concurrent runs start within the same millisecond;
// a suffix keeps every record individually addressable.
var newIDSeq uint64

// Metadata holds the collector state.
type Metadata struct {
	Version string           `json:"version"`
	Tasks   map[string]*Task `json:"tasks"`
	Runs    map[string]*Run  `json:"runs"`
}

// NewCollector creates a new collector instance.
func NewCollector(cfg *config.Config, store *storage.Store, logger *zap.Logger) (*Collector, error) {
	c := &Collector{
		logger:    logger,
		config:    cfg,
		store:     store,
		tasks:     make(map[string]*Task),
		runs:      make(map[string]*Run),
		semaphore: semaphore.NewWeighted(int64(cfg.Collector.MaxConcurrentTasks)),
	}

	if err := c.loadMetadata(); err != nil {
		return nil, fmt.Errorf("load metadata: %w", err)
	}

	return c, nil
}

// loadMetadata loads the collector state from disk.
func (c *Collector) loadMetadata() error {
	data, err := os.ReadFile(c.config.Metadata.CollectorPath)
	if err != nil {
		if os.IsNotExist(err) {
			return c.saveMetadata()
		}
		return fmt.Errorf("read collector metadata: %w", err)
	}

	md := &Metadata{}
	if err := json.Unmarshal(data, md); err != nil {
		return fmt.Errorf("unmarshal collector metadata: %w", err)
	}
	if md.Tasks == nil {
		md.Tasks = make(map[string]*Task)
	}
	if md.Runs == nil {
		md.Runs = make(map[string]*Run)
	}

	// Hold both write locks while swapping the maps: ReloadMetadata can run
	// while a run is in flight.
	c.tasksMu.Lock()
	c.runsMu.Lock()
	c.tasks = md.Tasks
	c.runs = md.Runs
	c.runsMu.Unlock()
	c.tasksMu.Unlock()
	return nil
}

// ReloadMetadata re-reads collector state from the currently configured path.
// NewCollector loads eagerly, but the --data-root flag is only parsed later, so
// it reads the default root's state and silently discards the correct one.
// PersistentPreRun calls this after re-deriving the paths.
func (c *Collector) ReloadMetadata() error {
	return c.loadMetadata()
}

// saveMetadata persists the collector state. It deep-copies both maps while
// holding the read locks, then marshals and writes outside them. The write goes
// through a temp file and rename, so a torn metadata file cannot appear.
//
// The copy must be deep, not a pointer copy. Marshal dereferences every
// *Task and *Run, and the originals stay live: RunTask mutates Run fields
// after publishing the record, and UpdateTask mutates Task fields in place.
// Holding the map locks across the marshal would fix it too, but a marshal plus
// disk write per run would serialize the hot path for nothing.
//
// Callers must not hold tasksMu, runsMu or saveMu. Map mutators release their
// lock before calling here; saveMu orders the snapshots so the last writer
// holds the newest union of changes.
func (c *Collector) saveMetadata() error {
	c.saveMu.Lock()
	defer c.saveMu.Unlock()

	c.tasksMu.RLock()
	tasks := c.snapshotTasks()
	c.tasksMu.RUnlock()

	c.runsMu.RLock()
	runs := c.snapshotRuns()
	c.runsMu.RUnlock()

	dir := filepath.Dir(c.config.Metadata.CollectorPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create metadata dir: %w", err)
	}

	md := &Metadata{Version: "1.0.0", Tasks: tasks, Runs: runs}
	data, err := json.MarshalIndent(md, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	return writeFileAtomic(c.config.Metadata.CollectorPath, data)
}

// snapshotTasks deep-copies the task map. Copying Params breaks the shared
// reference so a later UpdateTask cannot be observed mid-marshal.
// copyTask clones a task so a caller holding the copy cannot observe a
// concurrent writer and cannot be mutated through the live record.
func copyTask(t *Task) *Task {
	cp := *t
	if t.Params != nil {
		cp.Params = make(map[string]interface{}, len(t.Params))
		for pk, pv := range t.Params {
			cp.Params[pk] = pv
		}
	}
	return &cp
}

// copyRun clones a run. RunTask writes Status, Rows and EndedAt after the run
// record is published to the map, so handing callers the live pointer is a
// data race even when the map itself is guarded.
func copyRun(r *Run) *Run {
	cp := *r
	return &cp
}

func (c *Collector) snapshotTasks() map[string]*Task {
	out := make(map[string]*Task, len(c.tasks))
	for k, t := range c.tasks {
		out[k] = copyTask(t)
	}
	return out
}

// snapshotRuns deep-copies the run map so marshalling cannot read a Run while
// RunTask updates its terminal status.
func (c *Collector) snapshotRuns() map[string]*Run {
	out := make(map[string]*Run, len(c.runs))
	for k, r := range c.runs {
		out[k] = copyRun(r)
	}
	return out
}

// writeFileAtomic writes data to path through a temp file in the same directory
// and rename, so a reader never observes a partial file and a crash leaves the
// previous version intact.
func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Chmod(tmpName, 0644); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}

// addRunID returns a unique run ID. Millisecond timestamps collide under
// concurrent runs, and a duplicate key would overwrite the earlier run record
// in both the in-memory map and the metadata file.
func addRunID() string {
	return fmt.Sprintf("%d-%d", time.Now().UnixMilli(), atomic.AddUint64(&newIDSeq, 1))
}

// addTaskID returns a unique task ID.
func addTaskID() string {
	return fmt.Sprintf("%d-%d", time.Now().UnixMilli(), atomic.AddUint64(&newIDSeq, 1))
}

// AddTask creates a new task. New tasks start disabled so a task can be
// reviewed and enabled deliberately rather than firing on its first save.
func (c *Collector) AddTask(name, source, interfaceName, table, layer string, params map[string]interface{}, schedule TaskSchedule) (*Task, error) {
	t := &Task{
		ID:        addTaskID(),
		Name:      name,
		Source:    source,
		Interface: interfaceName,
		Table:     table,
		Layer:     layer,
		Enabled:   false,
		Schedule:  schedule,
		Params:    params,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	c.tasksMu.Lock()
	c.tasks[t.ID] = t
	c.tasksMu.Unlock()

	if err := c.saveMetadata(); err != nil {
		return nil, err
	}

	c.logger.Info("task created",
		zap.String("id", t.ID),
		zap.String("name", t.Name),
		zap.String("table", t.Table))

	return t, nil
}

// ListTasks returns all tasks.
func (c *Collector) ListTasks() []*Task {
	c.tasksMu.RLock()
	defer c.tasksMu.RUnlock()

	var tasks []*Task
	for _, t := range c.tasks {
		tasks = append(tasks, copyTask(t))
	}
	return tasks
}

// GetTask retrieves a task by ID.
func (c *Collector) GetTask(id string) (*Task, bool) {
	c.tasksMu.RLock()
	defer c.tasksMu.RUnlock()
	t, ok := c.tasks[id]
	if !ok {
		return nil, false
	}
	return copyTask(t), true
}

// DeleteTask removes a task.
func (c *Collector) DeleteTask(id string) error {
	c.tasksMu.Lock()
	_, ok := c.tasks[id]
	if ok {
		delete(c.tasks, id)
	}
	c.tasksMu.Unlock()
	if !ok {
		return fmt.Errorf("task not found: %s", id)
	}

	return c.saveMetadata()
}

// UpdateTask modifies a task.
func (c *Collector) UpdateTask(id string, updates map[string]interface{}) error {
	c.tasksMu.Lock()
	t, ok := c.tasks[id]
	if ok {
		for k, v := range updates {
			switch k {
			case "enabled":
				if enabled, ok := v.(bool); ok {
					t.Enabled = enabled
				}
			case "params":
				if params, ok := v.(map[string]interface{}); ok {
					t.Params = params
				}
			case "schedule":
				schedule := scheduleFromMap(v)
				if schedule != nil {
					t.Schedule = *schedule
				}
			}
		}
		t.UpdatedAt = time.Now()
	}
	c.tasksMu.Unlock()
	if !ok {
		return fmt.Errorf("task not found: %s", id)
	}

	return c.saveMetadata()
}

// RunTask executes a collection task.
func (c *Collector) RunTask(ctx context.Context, taskID string) (*Run, error) {
	c.tasksMu.RLock()
	t, ok := c.tasks[taskID]
	c.tasksMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}

	if !t.Enabled {
		return nil, fmt.Errorf("task is disabled: %s", taskID)
	}

	// Check if context was cancelled before acquiring semaphore
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("context cancelled: %w", ctx.Err())
	default:
	}

	if err := c.semaphore.Acquire(ctx, 1); err != nil {
		return nil, fmt.Errorf("acquire semaphore: %w", err)
	}
	defer c.semaphore.Release(1)

	r := &Run{
		TaskID:    taskID,
		RunID:     addRunID(),
		Status:    "running",
		StartedAt: time.Now(),
	}

	c.runsMu.Lock()
	c.runs[r.RunID] = r
	c.runsMu.Unlock()

	c.logger.Info("run started",
		zap.String("task_id", taskID),
		zap.String("run_id", r.RunID))

	// Execute the task
	rows, err := c.executeTask(ctx, t)

	// Update the record under the lock. saveMetadata marshals every *Run in the
	// map, so writing these fields outside it would race with that read.
	c.runsMu.Lock()
	r.Rows = rows
	r.EndedAt = time.Now()
	if err != nil {
		r.Status = "failed"
		r.Error = err.Error()
	} else {
		r.Status = "success"
	}
	c.runs[r.RunID] = r
	c.runsMu.Unlock()

	if err != nil {
		c.logger.Error("run failed",
			zap.String("run_id", r.RunID),
			zap.Error(err))
	} else {
		c.logger.Info("run completed",
			zap.String("run_id", r.RunID),
			zap.Int("rows", rows))
	}

	if saveErr := c.saveMetadata(); saveErr != nil {
		c.logger.Error("save metadata failed", zap.Error(saveErr))
	}

	// Surface the execution error to the caller as well as the run record. A
	// nil return would let a scheduled job and a CLI invocation report success
	// for a run that collected nothing.
	if r.Error != "" {
		return r, errors.New(r.Error)
	}
	return r, nil
}

// RunTasks executes the named tasks concurrently and returns a result per ID.
// Concurrency is bounded by the collector semaphore (MaxConcurrentTasks), so
// this fans out to N goroutines without exceeding the configured limit.
//
// Context cancellation waits for the tasks already running to finish. That is
// the deliberate tradeoff: storage writes are whole-file rewrites, and
// abandoning one mid-rename would leave the table unwritable.
//
// Results are ordered by the requested IDs, not by completion order, so callers
// can zip the slice against the input.
func (c *Collector) RunTasks(ctx context.Context, taskIDs []string) ([]*Run, error) {
	runs := make([]*Run, len(taskIDs))
	errs := make([]error, len(taskIDs))
	var wg sync.WaitGroup
	for i, id := range taskIDs {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			runs[i], errs[i] = c.RunTask(ctx, id)
		}(i, id)
	}
	wg.Wait()

	var firstErr error
	for _, e := range errs {
		if e != nil {
			if firstErr == nil {
				firstErr = e
			}
			continue
		}
	}
	return runs, firstErr
}

// RunAll executes every enabled task concurrently. Disabled tasks are skipped
// rather than failed, so a partial schedule still runs the tasks it can.
func (c *Collector) RunAll(ctx context.Context) ([]*Run, error) {
	tasks := c.ListTasks()
	ids := make([]string, 0, len(tasks))
	for _, t := range tasks {
		if t.Enabled {
			ids = append(ids, t.ID)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	sort.Strings(ids)
	return c.RunTasks(ctx, ids)
}

// executeTask performs the actual data collection.
func (c *Collector) executeTask(ctx context.Context, task *Task) (int, error) {
	// Lookup holds the registry read lock; reading the map directly races with
	// Register, which is reachable from tests and from plugin loading.
	adapter := Lookup(task.Source)
	if adapter == nil {
		return 0, fmt.Errorf("unknown source: %s", task.Source)
	}

	// Build request params
	params := make(map[string]interface{})
	params["interface"] = task.Interface
	for k, v := range task.Params {
		params[k] = v
	}

	// Execute request
	data, err := adapter.Request(ctx, params)
	if err != nil {
		return 0, fmt.Errorf("source request: %w", err)
	}

	if data == nil || len(data) == 0 {
		return 0, nil
	}

	// Resolve output table: use task.Table if set, otherwise look up provider registry.
	tableName := task.Table
	if tableName == "" {
		tableName = source.GetOutputTable(task.Interface)
		if tableName == "" {
			return 0, fmt.Errorf("no output table configured for interface: %s", task.Interface)
		}
	}

	// Resolve output layer: use task.Layer if set, otherwise look up provider registry.
	layer := task.Layer
	if layer == "" {
		layer = source.GetOutputLayer(task.Interface)
		if layer == "" {
			layer = "core"
		}
	}

	// Validate the target table schema exists
	schemaDef := schema.TableRegistry[tableName]
	if schemaDef == nil {
		return 0, fmt.Errorf("unknown table schema: %s", tableName)
	}

	// Convert raw data to typed records
	records := make([]interface{}, 0, len(data))
	for _, row := range data {
		record := buildRecord(row, tableName)
		if record != nil {
			records = append(records, record)
		}
	}

	// Write to storage
	if err := c.store.Write(layer, tableName, records); err != nil {
		return 0, fmt.Errorf("write to storage: %w", err)
	}

	return len(records), nil
}

// buildRecord converts raw data to a typed record for the given table.
// For the 4 core tables, use typed structs. For all others, build a generic
// map[string]interface{} using ProviderRegistry field mapping or direct name match.
func buildRecord(data map[string]interface{}, tableName string) interface{} {
	// Typed struct tables (core tables with specific models)
	switch tableName {
	case "daily":
		return buildDailyRecord(data)
	case "adj_factor":
		return buildGenericRecord(data, tableName)
	default:
		// Generic: use ProviderRegistry field mapping or direct column match.
		return buildGenericRecord(data, tableName)
	}
}

// buildDailyRecord builds a DailyRecord, applying ProviderRegistry field mapping
// before reading fields so that adapters returning different field names work.
func buildDailyRecord(data map[string]interface{}) interface{} {
	// Apply field mapping from any ProviderInterface targeting "daily"
	mapped := applyFieldMapping(data, "daily")
	return schema.DailyRecord{
		TsCode:    getString(mapped, "ts_code"),
		TradeDate: getString(mapped, "trade_date"),
		Open:      getFloat(mapped, "open"),
		High:      getFloat(mapped, "high"),
		Low:       getFloat(mapped, "low"),
		Close:     getFloat(mapped, "close"),
		PreClose:  getFloat(mapped, "pre_close"),
		Change:    getFloat(mapped, "change"),
		PctChg:    getFloat(mapped, "pct_chg"),
		Vol:       getFloat(mapped, "vol"),
		Amount:    getFloat(mapped, "amount"),
	}
}

// applyFieldMapping applies ProviderRegistry field mapping to raw data.
// Returns a copy of the input with mapped field names; includes unmapped
// fields that match a schema column name.
func applyFieldMapping(data map[string]interface{}, tableName string) map[string]interface{} {
	schemaDef := schema.GetSchema(tableName)
	colMap := make(map[string]string)
	if schemaDef != nil {
		for _, col := range schemaDef.Columns {
			colMap[col.Name] = col.Type
		}
	}

	var fieldMapping map[string]string
	for _, pi := range source.ProviderRegistry {
		if pi.Table == tableName {
			fieldMapping = pi.FieldMapping
			break
		}
	}

	mapped := make(map[string]interface{})
	if fieldMapping != nil {
		for adapterKey, colName := range fieldMapping {
			if val, ok := data[adapterKey]; ok {
				mapped[colName] = val
			}
		}
		for key, val := range data {
			if _, hasMapping := fieldMapping[key]; !hasMapping {
				if _, hasColumn := colMap[key]; hasColumn {
					mapped[key] = val
				}
			}
		}
	} else {
		for key, val := range data {
			if _, ok := colMap[key]; ok {
				mapped[key] = val
			}
		}
	}
	return mapped
}

// buildGenericRecord creates a map[string]interface{} record for non-core tables.
// Uses ProviderRegistry field mapping if available, otherwise falls back to direct name match.
func buildGenericRecord(data map[string]interface{}, tableName string) interface{} {
	schemaDef := schema.GetSchema(tableName)
	if schemaDef == nil {
		return data
	}
	mapped := applyFieldMapping(data, tableName)
	return mapped
}

func getString(data map[string]interface{}, key string) string {
	if v, ok := data[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
		return fmt.Sprintf("%v", v)
	}
	return ""
}

func getFloat(data map[string]interface{}, key string) float64 {
	if v, ok := data[key]; ok {
		if f, ok := v.(float64); ok {
			return f
		}
		if i, ok := v.(int64); ok {
			return float64(i)
		}
		if s, ok := v.(string); ok {
			f, _ := strconv.ParseFloat(s, 64)
			return f
		}
	}
	return 0
}

func getInt64(data map[string]interface{}, key string) int64 {
	if v, ok := data[key]; ok {
		if i, ok := v.(int64); ok {
			return i
		}
		if f, ok := v.(float64); ok {
			return int64(f)
		}
		if s, ok := v.(string); ok {
			i, _ := strconv.ParseInt(s, 10, 64)
			return i
		}
	}
	return 0
}

// ListRuns returns all runs.
func (c *Collector) ListRuns() []*Run {
	c.runsMu.RLock()
	defer c.runsMu.RUnlock()

	var runs []*Run
	for _, r := range c.runs {
		runs = append(runs, copyRun(r))
	}
	return runs
}

// GetRun retrieves a run by ID.
func (c *Collector) GetRun(id string) (*Run, bool) {
	c.runsMu.RLock()
	defer c.runsMu.RUnlock()
	r, ok := c.runs[id]
	if !ok {
		return nil, false
	}
	return copyRun(r), true
}
