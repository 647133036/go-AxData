// Package cache provides a local-first data access facade shared by all
// analysis modules.
//
// The strategy is: read the local Parquet cache first; only when the cached
// table is missing or holds too few rows does the facade call the source
// adapter and write the result back to the cache.
package cache

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/electkismet/axdata-go/core/config"
	"github.com/electkismet/axdata-go/core/query"
	"github.com/electkismet/axdata-go/core/schema"
	"github.com/electkismet/axdata-go/core/source"
	"github.com/electkismet/axdata-go/core/storage"
	"go.uber.org/zap"
)

// ErrNoLocalData indicates the table does not exist locally.
var ErrNoLocalData = errors.New("no local data")

// ErrAllSourcesFailed is returned when every configured data source fails.
type ErrAllSourcesFailed struct {
	Source string
	Detail []string
}

func (e *ErrAllSourcesFailed) Error() string {
	return fmt.Sprintf("all sources failed for %s: %s", e.Source, strings.Join(e.Detail, "; "))
}

// Row is a table row with typed accessors so analysis modules never deal with
// raw database/sql values.
type Row struct {
	Values map[string]interface{}
}

// Str returns a string column, "" when missing or nil.
func (r Row) Str(col string) string {
	if r.Values == nil {
		return ""
	}
	return normalizeString(r.Values[col])
}

// F64 returns a float column, 0 when missing or unparsable.
func (r Row) F64(col string) float64 {
	if r.Values == nil {
		return 0
	}
	return normalizeFloat(r.Values[col])
}

// I64 returns an int64 column, 0 when missing or unparsable.
func (r Row) I64(col string) int64 {
	if r.Values == nil {
		return 0
	}
	return int64(normalizeFloat(r.Values[col]))
}

// Has reports whether the column exists and is non-empty.
func (r Row) Has(col string) bool {
	if r.Values == nil {
		return false
	}
	v, ok := r.Values[col]
	if !ok || v == nil {
		return false
	}
	return normalizeString(v) != ""
}

// Getter is the facade used by all analysis modules.
type Getter struct {
	Cfg    *config.Config
	Store  *storage.Store
	Query  *query.Querier
	Logger *zap.Logger
}

// New creates a Getter.
func New(cfg *config.Config, store *storage.Store, querier *query.Querier, logger *zap.Logger) *Getter {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Getter{Cfg: cfg, Store: store, Query: querier, Logger: logger}
}

// Exists reports whether the local Parquet file for a table exists.
func (g *Getter) Exists(table string) bool {
	return g.Store.Exists("core", table)
}

// Rows returns local rows for a table, applying equality filters.
func (g *Getter) Rows(ctx context.Context, table string, where map[string]string) ([]Row, error) {
	if err := validateTable(table); err != nil {
		return nil, err
	}
	path := g.Cfg.CorePath(table)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("%w: %s", ErrNoLocalData, table)
	}

	var b strings.Builder
	b.WriteString("SELECT * FROM read_parquet('")
	b.WriteString(escapeSQL(path))
	b.WriteString("')")
	if len(where) > 0 {
		b.WriteString(" WHERE ")
		clauses := make([]string, 0, len(where))
		for k, v := range where {
			clauses = append(clauses, fmt.Sprintf("%s = '%s'", k, escapeSQL(v)))
		}
		b.WriteString(strings.Join(clauses, " AND "))
	}

	raw, cols, err := g.query(ctx, b.String())
	if err != nil {
		return nil, err
	}
	rows := make([]Row, 0, len(raw))
	for _, r := range raw {
		m := make(map[string]interface{}, len(cols))
		for i, c := range cols {
			if i < len(r) {
				m[c] = r[i]
			}
		}
		rows = append(rows, Row{Values: m})
	}
	return rows, nil
}

// query runs SQL and returns rows plus their column names.
func (g *Getter) query(ctx context.Context, sqlText string) ([][]interface{}, []string, error) {
	return g.Query.ExecuteWithColumns(ctx, sqlText)
}

// FetchTable applies the local-first strategy for the rows of table that match
// where. When the local rows meeting the filter hold at least minRows they are
// returned as-is; otherwise fetchFn is called, its result is merged into the
// table, and the matching content is returned.
//
// where must be set for snapshot tables. A snapshot write replaces the whole
// file, so a key-blind fetch lets the last-written security shadow every other
// one in the cache.
func (g *Getter) FetchTable(ctx context.Context, table string, where map[string]string, minRows int,
	fetchFn func(context.Context) ([]map[string]interface{}, error)) ([]Row, error) {
	if err := validateTable(table); err != nil {
		return nil, err
	}

	local, err := g.Rows(ctx, table, where)
	if err == nil {
		if minRows <= 0 || len(local) >= minRows {
			g.Logger.Debug("cache hit", zap.String("table", table), zap.Int("rows", len(local)))
			return local, nil
		}
		g.Logger.Warn("cache insufficient, falling back to source",
			zap.String("table", table), zap.Int("local", len(local)), zap.Int("required", minRows))
	}

	fresh, err := fetchFn(ctx)
	if err != nil {
		if len(local) > 0 {
			g.Logger.Warn("source fallback failed, using stale cache",
				zap.String("table", table), zap.Error(err))
			return local, nil
		}
		return nil, fmt.Errorf("fetch %s: %w", table, err)
	}

	if len(fresh) > 0 {
		merged := g.mergeRows(ctx, table, where, fresh)
		if werr := g.Store.Write("core", table, mapsToInterface(merged)); werr != nil {
			g.Logger.Error("write cache failed", zap.String("table", table), zap.Error(werr))
		}
	}

	merged, err := g.Rows(ctx, table, where)
	if err != nil {
		return filterRows(rowsFromMaps(fresh), where), nil
	}
	return merged, nil
}

// mergeRows returns the table content after replacing every row matching where
// with fresh. Snapshot writes are whole-file, so the other keys' rows have to be
// carried along explicitly or they are lost.
func (g *Getter) mergeRows(ctx context.Context, table string, where map[string]string,
	fresh []map[string]interface{}) []map[string]interface{} {
	if len(where) == 0 {
		return fresh
	}
	existing, err := g.Rows(ctx, table, nil)
	if err != nil {
		return fresh
	}
	kept := make([]map[string]interface{}, 0, len(existing))
	for _, r := range existing {
		if !matchesFilter(r, where) {
			kept = append(kept, rowToMap(r))
		}
	}
	return append(kept, fresh...)
}

// matchesFilter reports whether a row satisfies every key in where.
func matchesFilter(r Row, where map[string]string) bool {
	for k, v := range where {
		if r.Str(k) != v {
			return false
		}
	}
	return true
}

// filterRows keeps the rows satisfying where, or all rows when where is empty.
func filterRows(rows []Row, where map[string]string) []Row {
	if len(where) == 0 {
		return rows
	}
	out := make([]Row, 0, len(rows))
	for _, r := range rows {
		if matchesFilter(r, where) {
			out = append(out, r)
		}
	}
	return out
}

// rowToMap copies a row's values into a plain map for storage write-back.
func rowToMap(r Row) map[string]interface{} {
	out := make(map[string]interface{}, len(r.Values))
	for k, v := range r.Values {
		out[k] = v
	}
	return out
}

// SourceRequest calls a registered adapter by source name.
func (g *Getter) SourceRequest(ctx context.Context, sourceName, interfaceName string,
	params map[string]interface{}) ([]map[string]interface{}, error) {
	adapter, ok := source.LookupOk(sourceName)
	if !ok {
		return nil, fmt.Errorf("unknown source: %s", sourceName)
	}
	p := make(map[string]interface{}, len(params)+1)
	for k, v := range params {
		p[k] = v
	}
	p["interface"] = interfaceName
	data, err := adapter.Request(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("source %s/%s: %w", sourceName, interfaceName, err)
	}
	return data, nil
}

// Target pairs a source with the interface name and params to send it.
type Target struct {
	Source    string
	Interface string
	Params    map[string]interface{}
}

// FanoutRequest tries several source/interface targets in order and returns
// the first non-empty result, aggregating every failure into
// ErrAllSourcesFailed.
func (g *Getter) FanoutRequest(ctx context.Context, targets ...Target) ([]map[string]interface{}, string, error) {
	if len(targets) == 0 {
		return nil, "", fmt.Errorf("cache: no fanout targets provided")
	}
	var detail []string
	for _, t := range targets {
		data, err := g.SourceRequest(ctx, t.Source, t.Interface, t.Params)
		if err != nil {
			detail = append(detail, fmt.Sprintf("%s/%s: %v", t.Source, t.Interface, err))
			g.Logger.Warn("source failed",
				zap.String("source", t.Source), zap.String("interface", t.Interface),
				zap.Error(err))
			continue
		}
		if len(data) == 0 {
			detail = append(detail, fmt.Sprintf("%s/%s: empty result", t.Source, t.Interface))
			continue
		}
		return data, t.Source, nil
	}
	return nil, "", &ErrAllSourcesFailed{Source: targets[0].Interface, Detail: detail}
}

// CacheDir returns a per-module subdirectory under the data cache root.
func (g *Getter) CacheDir(name string) string {
	return g.Cfg.DataRoot + "/cache/" + name
}

func validateTable(table string) error {
	if table == "" {
		return fmt.Errorf("empty table name")
	}
	if schema.TableRegistry[table] == nil {
		return fmt.Errorf("unknown table schema: %s", table)
	}
	if strings.ContainsAny(table, "'\\/;\n\t") {
		return fmt.Errorf("unsafe table name: %s", table)
	}
	return nil
}

func escapeSQL(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func mapsToInterface(maps []map[string]interface{}) []interface{} {
	out := make([]interface{}, 0, len(maps))
	for _, m := range maps {
		out = append(out, m)
	}
	return out
}

func rowsFromMaps(maps []map[string]interface{}) []Row {
	out := make([]Row, 0, len(maps))
	for _, m := range maps {
		out = append(out, Row{Values: m})
	}
	return out
}

func normalizeString(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []byte:
		return string(t)
	case time.Time:
		return t.Format("2006-01-02")
	default:
		s := fmt.Sprintf("%v", t)
		if s == "<nil>" || s == "NULL" {
			return ""
		}
		return s
	}
}

func normalizeFloat(v interface{}) float64 {
	switch t := v.(type) {
	case nil:
		return 0
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case int32:
		return float64(t)
	case int16:
		return float64(t)
	case int8:
		return float64(t)
	case uint64:
		return float64(t)
	case bool:
		if t {
			return 1
		}
		return 0
	case sql.RawBytes:
		return parseNumeric(string(t))
	case []byte:
		return parseNumeric(string(t))
	case string:
		return parseNumeric(t)
	case time.Time:
		return float64(t.Unix())
	default:
		return parseNumeric(fmt.Sprintf("%v", t))
	}
}

func parseNumeric(s string) float64 {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")
	if s == "" {
		return 0
	}
	clean := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' || r == '.' || r == '-' || r == '+' || r == 'e' || r == 'E' {
			return r
		}
		return -1
	}, s)
	if clean == "" {
		return 0
	}
	f, err := strconv.ParseFloat(clean, 64)
	if err != nil {
		return 0
	}
	return f
}
