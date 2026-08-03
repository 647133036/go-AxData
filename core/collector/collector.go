package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
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
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Source      string        `json:"source"`
	Interface   string        `json:"interface"`
	Table       string        `json:"table"`
	Layer       string        `json:"layer"`
	Enabled     bool          `json:"enabled"`
	Schedule    TaskSchedule  `json:"schedule"`
	Params      map[string]interface{} `json:"params"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

// TaskSchedule defines when a task runs.
type TaskSchedule struct {
	Type     string `json:"type"` // manual, interval, daily, startup
	Interval string `json:"interval,omitempty"`
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
	logger     *zap.Logger
	config     *config.Config
	store      *storage.Store
	tasks      map[string]*Task
	tasksMu    sync.RWMutex
	runs       map[string]*Run
	runsMu     sync.RWMutex
	semaphore  *semaphore.Weighted
	metadata   *Metadata
}

// Metadata holds the collector state.
type Metadata struct {
	Version string            `json:"version"`
	Tasks   map[string]*Task  `json:"tasks"`
	Runs    map[string]*Run   `json:"runs"`
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
		metadata: &Metadata{
			Version: "1.0.0",
			Tasks:   make(map[string]*Task),
			Runs:    make(map[string]*Run),
		},
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

	if err := json.Unmarshal(data, c.metadata); err != nil {
		return fmt.Errorf("unmarshal collector metadata: %w", err)
	}

	c.tasks = c.metadata.Tasks
	c.runs = c.metadata.Runs
	return nil
}

// saveMetadata persists the collector state.
func (c *Collector) saveMetadata() error {
	dir := filepath.Dir(c.config.Metadata.CollectorPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create metadata dir: %w", err)
	}
	c.metadata.Tasks = c.tasks
	c.metadata.Runs = c.runs
	data, err := json.MarshalIndent(c.metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	return os.WriteFile(c.config.Metadata.CollectorPath, data, 0644)
}

// AddTask creates a new task.
func (c *Collector) AddTask(name, source, interfaceName, table, layer string, params map[string]interface{}) (*Task, error) {
	c.tasksMu.Lock()
	defer c.tasksMu.Unlock()

	t := &Task{
		ID:        fmt.Sprintf("%d", time.Now().UnixMilli()),
		Name:      name,
		Source:    source,
		Interface: interfaceName,
		Table:     table,
		Layer:     layer,
		Enabled:   false,
		Params:    params,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	c.tasks[t.ID] = t

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
		tasks = append(tasks, t)
	}
	return tasks
}

// GetTask retrieves a task by ID.
func (c *Collector) GetTask(id string) (*Task, bool) {
	c.tasksMu.RLock()
	defer c.tasksMu.RUnlock()
	t, ok := c.tasks[id]
	return t, ok
}

// DeleteTask removes a task.
func (c *Collector) DeleteTask(id string) error {
	c.tasksMu.Lock()
	defer c.tasksMu.Unlock()

	if _, ok := c.tasks[id]; !ok {
		return fmt.Errorf("task not found: %s", id)
	}

	delete(c.tasks, id)
	return c.saveMetadata()
}

// UpdateTask modifies a task.
func (c *Collector) UpdateTask(id string, updates map[string]interface{}) error {
	c.tasksMu.Lock()
	defer c.tasksMu.Unlock()

	t, ok := c.tasks[id]
	if !ok {
		return fmt.Errorf("task not found: %s", id)
	}

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
		}
	}
	t.UpdatedAt = time.Now()
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
		RunID:     fmt.Sprintf("%d", time.Now().UnixMilli()),
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
	r.Rows = rows
	r.EndedAt = time.Now()

	if err != nil {
		r.Status = "failed"
		r.Error = err.Error()
		c.logger.Error("run failed",
			zap.String("run_id", r.RunID),
			zap.Error(err))
	} else {
		r.Status = "success"
		c.logger.Info("run completed",
			zap.String("run_id", r.RunID),
			zap.Int("rows", rows))
	}

	c.runsMu.Lock()
	c.runs[r.RunID] = r
	c.runsMu.Unlock()

	if saveErr := c.saveMetadata(); saveErr != nil {
		c.logger.Error("save metadata failed", zap.Error(saveErr))
	}

	return r, nil
}

// executeTask performs the actual data collection.
func (c *Collector) executeTask(ctx context.Context, task *Task) (int, error) {
	// Get the source adapter
	adapter, ok := SourceAdapters[task.Source]
	if !ok {
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
		runs = append(runs, r)
	}
	return runs
}

// GetRun retrieves a run by ID.
func (c *Collector) GetRun(id string) (*Run, bool) {
	c.runsMu.RLock()
	defer c.runsMu.RUnlock()
	r, ok := c.runs[id]
	return r, ok
}
