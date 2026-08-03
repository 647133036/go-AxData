package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/electkismet/axdata-go/core/collector"
	"github.com/electkismet/axdata-go/core/config"
	"github.com/electkismet/axdata-go/core/plugin"
	"github.com/electkismet/axdata-go/core/query"
	"github.com/electkismet/axdata-go/core/schema"
	"github.com/electkismet/axdata-go/core/storage"
	"go.uber.org/zap"
)

// APIServer provides the HTTP API for AxData.
type APIServer struct {
	mu         sync.RWMutex
	cfg        *config.Config
	store      *storage.Store
	querier    *query.Querier
	collector  *collector.Collector
	pluginMgr  *plugin.PluginManager
	logger     *zap.Logger
}

// NewAPIServer creates a new API server.
func NewAPIServer(cfg *config.Config, store *storage.Store, querier *query.Querier, collector *collector.Collector, logger *zap.Logger, pluginMgr *plugin.PluginManager) *APIServer {
	return &APIServer{
		cfg:       cfg,
		store:     store,
		querier:   querier,
		collector: collector,
		pluginMgr: pluginMgr,
		logger:    logger,
	}
}

// RegisterRoutes registers all API routes on the given mux.
func (s *APIServer) RegisterRoutes(mux *http.ServeMux) {
	// Health check
	mux.HandleFunc("GET /v1/health", s.healthHandler)

	// Schema
	mux.HandleFunc("GET /v1/schema", s.schemaHandler)

	// Data tables
	mux.HandleFunc("GET /v1/tables", s.tablesHandler)
	mux.HandleFunc("GET /v1/tables/{table}", s.tableHandler)
	mux.HandleFunc("GET /v1/tables/{table}/count", s.tableCountHandler)

	// Query
	mux.HandleFunc("POST /v1/query", s.queryHandler)

	// Data preview
	mux.HandleFunc("GET /v1/data/{table}", s.dataHandler)
	mux.HandleFunc("GET /v1/data/{table}/preview", s.previewHandler)

	// Collector
	mux.HandleFunc("GET /v1/collector/tasks", s.collectorTasksHandler)
	mux.HandleFunc("POST /v1/collector/tasks", s.collectorAddTaskHandler)
	mux.HandleFunc("GET /v1/collector/tasks/{id}", s.collectorTaskInfoHandler)
	mux.HandleFunc("PUT /v1/collector/tasks/{id}", s.collectorUpdateTaskHandler)
	mux.HandleFunc("DELETE /v1/collector/tasks/{id}", s.collectorDeleteTaskHandler)
	mux.HandleFunc("POST /v1/collector/tasks/{id}/run", s.collectorRunTaskHandler)
	mux.HandleFunc("GET /v1/collector/runs", s.collectorRunsHandler)
	mux.HandleFunc("GET /v1/collector/runs/{id}", s.collectorRunInfoHandler)
	mux.HandleFunc("GET /v1/collector/status", s.collectorStatusHandler)

	// Sources
	mux.HandleFunc("GET /v1/sources", s.sourcesHandler)
	mux.HandleFunc("POST /v1/sources/{source}/request", s.sourceRequestHandler)

	// Interface catalog from providers
	mux.HandleFunc("GET /v1/interfaces", s.interfacesHandler)
	mux.HandleFunc("GET /v1/interfaces/{name}", s.interfaceHandler)
	mux.HandleFunc("GET /v1/sources/{source}/interfaces", s.sourceInterfacesHandler)

	// Config
	mux.HandleFunc("GET /v1/config", s.configHandler)

	// Data browser
	mux.HandleFunc("GET /v1/browser/tables", s.browserTablesHandler)
	mux.HandleFunc("GET /v1/browser/tables/{table}/columns", s.browserColumnsHandler)
}

// healthHandler returns server health status.
func (s *APIServer) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"time":    time.Now().Format(time.RFC3339),
		"version": "1.0.0",
	})
}

// schemaHandler returns the schema registry.
func (s *APIServer) schemaHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	schemas := make(map[string]interface{})
	for name, ts := range schema.TableRegistry {
		schemas[name] = map[string]interface{}{
			"name":        ts.Name,
			"description": ts.Description,
			"layer":       ts.Layer,
			"write_mode":  ts.WriteMode,
			"primary_keys": ts.PrimaryKeys,
			"columns":     ts.Columns,
		}
	}
	json.NewEncoder(w).Encode(schemas)
}

// tablesHandler lists all tables.
func (s *APIServer) tablesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var tables []map[string]interface{}
	for name, ts := range schema.TableRegistry {
		exists := s.store.Exists("core", name)
		tables = append(tables, map[string]interface{}{
			"name":        name,
			"description": ts.Description,
			"layer":       ts.Layer,
			"exists":      exists,
			"count":       0, // placeholder
		})
	}
	json.NewEncoder(w).Encode(tables)
}

// tableHandler returns info for a specific table.
func (s *APIServer) tableHandler(w http.ResponseWriter, r *http.Request) {
	table := r.PathValue("table")
	ts := schema.GetSchema(table)
	if ts == nil {
		http.Error(w, "Table not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	count, _ := s.store.Count("core", table)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"name":         table,
		"description":  ts.Description,
		"layer":        ts.Layer,
		"write_mode":   ts.WriteMode,
		"primary_keys": ts.PrimaryKeys,
		"columns":      ts.Columns,
		"count":        count,
	})
}

// tableCountHandler returns the record count for a table.
func (s *APIServer) tableCountHandler(w http.ResponseWriter, r *http.Request) {
	table := r.PathValue("table")
	count, err := s.store.Count("core", table)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"count": count})
}

// queryHandler executes a SQL query.
func (s *APIServer) queryHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SQL string `json:"sql"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.SQL == "" {
		http.Error(w, "SQL parameter required", http.StatusBadRequest)
		return
	}

	if err := query.ValidateSQL(req.SQL); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	results, err := s.querier.Execute(ctx, req.SQL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"results": results,
		"count":   len(results),
	})
}

// dataHandler returns data rows for a table.
func (s *APIServer) dataHandler(w http.ResponseWriter, r *http.Request) {
	table := r.PathValue("table")
	if table == "" {
		http.Error(w, "table required", http.StatusBadRequest)
		return
	}

	// Validate table name
	if schema.GetSchema(table) == nil {
		http.Error(w, "unknown table: "+table, http.StatusBadRequest)
		return
	}

	parquetPath := s.cfg.CorePath(table)
	if _, err := os.Stat(parquetPath); os.IsNotExist(err) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"table": table,
			"rows":  []interface{}{},
			"count": 0,
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if err := s.querier.AttachParquet(ctx, "data_table", parquetPath); err != nil {
		http.Error(w, "attach parquet: "+err.Error(), http.StatusInternalServerError)
		return
	}

	results, err := s.querier.Execute(ctx, "SELECT * FROM data_table")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"table": table,
		"rows":  results,
		"count": len(results),
	})
}

// previewHandler returns preview rows from a table.
func (s *APIServer) previewHandler(w http.ResponseWriter, r *http.Request) {
	table := r.PathValue("table")
	if table == "" {
		http.Error(w, "table required", http.StatusBadRequest)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit == 0 {
		limit = 20
	}
	if limit > 1000 {
		limit = 1000
	}

	symbol := r.URL.Query().Get("symbol")
	start := r.URL.Query().Get("start")
	end := r.URL.Query().Get("end")

	// Check if the table exists in schema
	if schema.GetSchema(table) == nil {
		http.Error(w, "unknown table: "+table, http.StatusBadRequest)
		return
	}

	parquetPath := s.cfg.CorePath(table)
	if _, err := os.Stat(parquetPath); os.IsNotExist(err) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"table": table,
			"rows":  []interface{}{},
			"count": 0,
		})
		return
	}

	ctx := r.Context()
	// Attach parquet as a DuckDB table
	if err := s.querier.AttachParquet(ctx, "preview_table", parquetPath); err != nil {
		http.Error(w, "attach parquet: "+err.Error(), http.StatusInternalServerError)
		return
	}

	query := "SELECT * FROM preview_table"
	whereParts := []string{}
	params := []interface{}{}

	if symbol != "" {
		whereParts = append(whereParts, "ts_code = ?")
		params = append(params, symbol)
	}
	if start != "" {
		whereParts = append(whereParts, "trade_date >= ?")
		params = append(params, start)
	}
	if end != "" {
		whereParts = append(whereParts, "trade_date <= ?")
		params = append(params, end)
	}

	if len(whereParts) > 0 {
		query += " WHERE " + strings.Join(whereParts, " AND ")
	}
	query += fmt.Sprintf(" ORDER BY trade_date DESC LIMIT %d", limit)

	results, err := s.querier.Execute(ctx, query, params...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"table":  table,
		"rows":   results,
		"count":  len(results),
		"symbol": symbol,
		"start":  start,
		"end":    end,
	})
}

// collectorTasksHandler lists all tasks.
func (s *APIServer) collectorTasksHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	tasks := s.collector.ListTasks()
	json.NewEncoder(w).Encode(tasks)
}

// collectorAddTaskHandler creates a new task.
func (s *APIServer) collectorAddTaskHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string                 `json:"name"`
		Source      string                 `json:"source"`
		Interface   string                 `json:"interface"`
		Table       string                 `json:"table"`
		Layer       string                 `json:"layer"`
		Params      map[string]interface{} `json:"params"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	task, err := s.collector.AddTask(req.Name, req.Source, req.Interface, req.Table, req.Layer, req.Params)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(task)
}

// collectorTaskInfoHandler returns task details.
func (s *APIServer) collectorTaskInfoHandler(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	task, ok := s.collector.GetTask(taskID)
	if !ok {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

// collectorUpdateTaskHandler updates a task.
func (s *APIServer) collectorUpdateTaskHandler(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")

	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := s.collector.UpdateTask(taskID, req); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// collectorDeleteTaskHandler deletes a task.
func (s *APIServer) collectorDeleteTaskHandler(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")

	if err := s.collector.DeleteTask(taskID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// collectorRunTaskHandler runs a task.
func (s *APIServer) collectorRunTaskHandler(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")

	ctx := r.Context()
	run, err := s.collector.RunTask(ctx, taskID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(run)
}

// collectorRunsHandler lists all runs.
func (s *APIServer) collectorRunsHandler(w http.ResponseWriter, r *http.Request) {
	_ = r.URL.Query().Get("status")

	w.Header().Set("Content-Type", "application/json")
	runs := s.collector.ListRuns()
	json.NewEncoder(w).Encode(runs)
}

// collectorRunInfoHandler returns run details.
func (s *APIServer) collectorRunInfoHandler(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	run, ok := s.collector.GetRun(runID)
	if !ok {
		http.Error(w, "Run not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(run)
}

// collectorStatusHandler returns collector status.
func (s *APIServer) collectorStatusHandler(w http.ResponseWriter, r *http.Request) {
	tasks := s.collector.ListTasks()
	runs := s.collector.ListRuns()

	enabled := 0
	for _, t := range tasks {
		if t.Enabled {
			enabled++
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"total_tasks":    len(tasks),
		"enabled_tasks":  enabled,
		"total_runs":     len(runs),
		"max_concurrent": int(s.cfg.Collector.MaxConcurrentTasks),
		"batch_size":     int(s.cfg.Collector.BatchSize),
	})
}

// sourcesHandler lists all registered sources.
func (s *APIServer) sourcesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	sources := make(map[string]string)
	for _, name := range collector.List() {
		adapter := collector.Lookup(name)
		if adapter != nil {
			sources[name] = adapter.Description()
		}
	}
	json.NewEncoder(w).Encode(sources)
}

// sourceRequestHandler makes a request to a source.
func (s *APIServer) sourceRequestHandler(w http.ResponseWriter, r *http.Request) {
	sourceName := r.PathValue("source")

	var req struct {
		Interface string                 `json:"interface"`
		Params    map[string]interface{} `json:"params"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	adapter := collector.Lookup(sourceName)
	if adapter == nil {
		http.Error(w, "Source not found", http.StatusNotFound)
		return
	}

	ctx := r.Context()
	params := make(map[string]interface{})
	for k, v := range req.Params {
		params[k] = v
	}
	params["interface"] = req.Interface

	data, err := adapter.Request(ctx, params)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"source":  sourceName,
		"rows":    data,
		"count":   len(data),
	})
}

// configHandler returns the current config.
func (s *APIServer) configHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data_root":      s.cfg.DataRoot,
		"log_level":      s.cfg.LogLevel,
		"api_port":       s.cfg.APIPort,
		"max_concurrent": int(s.cfg.Collector.MaxConcurrentTasks),
		"batch_size":     int(s.cfg.Collector.BatchSize),
	})
}

// browserTablesHandler returns tables for the data browser.
func (s *APIServer) browserTablesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	tables := s.store.ListTables("core")
	json.NewEncoder(w).Encode(tables)
}

// browserColumnsHandler returns columns for a table.
func (s *APIServer) browserColumnsHandler(w http.ResponseWriter, r *http.Request) {
	table := r.PathValue("table")
	ts := schema.GetSchema(table)
	if ts == nil {
		http.Error(w, "Table not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ts.Columns)
}

// interfacesHandler lists all interfaces from enabled providers.
func (s *APIServer) interfacesHandler(w http.ResponseWriter, r *http.Request) {
	if s.pluginMgr == nil {
		http.NotFound(w, r)
		return
	}

	interfaces := s.pluginMgr.ListInterfaces()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(interfaces)
}

// interfaceHandler returns a single interface by name.
func (s *APIServer) interfaceHandler(w http.ResponseWriter, r *http.Request) {
	if s.pluginMgr == nil {
		http.NotFound(w, r)
		return
	}

	name := r.PathValue("name")
	iface, ok := s.pluginMgr.GetInterface(name)
	if !ok {
		http.Error(w, "Interface not found: "+name, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(iface)
}

// sourceInterfacesHandler returns interfaces for a specific source.
func (s *APIServer) sourceInterfacesHandler(w http.ResponseWriter, r *http.Request) {
	if s.pluginMgr == nil {
		http.NotFound(w, r)
		return
	}

	sourceCode := r.PathValue("source")
	var interfaces []plugin.SourceInterface

	for _, iface := range s.pluginMgr.ListInterfaces() {
		if iface.SourceCode == sourceCode {
			interfaces = append(interfaces, iface)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(interfaces)
}

// Shutdown gracefully shuts down the API server.
func (s *APIServer) Shutdown(ctx context.Context) {
	s.logger.Info("shutting down API server")
}
