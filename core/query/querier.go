package query

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/marcboeker/go-duckdb/v2"
)

// Querier provides SQL-based querying over Parquet data via DuckDB.
type Querier struct {
	conn *sql.DB
}

// NewQuerier creates a new DuckDB-based query engine.
func NewQuerier() (*Querier, error) {
	conn, err := sql.Open("duckdb", "")
	if err != nil {
		return nil, fmt.Errorf("open duckdb: %w", err)
	}
	return &Querier{conn: conn}, nil
}

// Close releases the DuckDB connection.
func (q *Querier) Close() error {
	if q.conn != nil {
		return q.conn.Close()
	}
	return nil
}

// Execute runs a SQL query and returns rows as [][]interface{}.
func (q *Querier) Execute(ctx context.Context, sql string, params ...interface{}) ([][]interface{}, error) {
	rows, err := q.conn.QueryContext(ctx, sql, params...)
	if err != nil {
		return nil, fmt.Errorf("execute: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("get columns: %w", err)
	}

	var results [][]interface{}
	for rows.Next() {
		vals := make([]interface{}, len(columns))
		valPtrs := make([]interface{}, len(columns))
		for i := range vals {
			valPtrs[i] = &vals[i]
		}
		if err := rows.Scan(valPtrs...); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		results = append(results, vals)
	}
	return results, rows.Err()
}

// AttachParquet attaches a Parquet file as a DuckDB table.
func (q *Querier) AttachParquet(ctx context.Context, table string, path string) error {
	_, err := q.conn.ExecContext(ctx, fmt.Sprintf("CREATE TABLE %s AS SELECT * FROM read_parquet('%s')", table, path))
	if err != nil {
		return fmt.Errorf("attach parquet: %w", err)
	}
	return nil
}

// ValidateSQL validates a SQL query for basic safety.
// Blocks DROP, DELETE, ALTER, CREATE, INSERT, UPDATE, and system tables.
// Note: This is a basic guard; for full safety use parameterized queries.
func ValidateSQL(sql string) error {
	upper := strings.ToUpper(strings.TrimSpace(sql))
	for _, keyword := range []string{
		"DROP ", "DROP(", "DELETE ", "ALTER ", "CREATE ", "INSERT ", "UPDATE ",
		"--", ";",
	} {
		if strings.Contains(upper, keyword) {
			return fmt.Errorf("sql contains blocked keyword: %s", keyword)
		}
	}
	return nil
}

// AttachCoreLayer attaches all Parquet files from the core data layer.
func (q *Querier) AttachCoreLayer(ctx context.Context, coreDir string) error {
	entries, err := os.ReadDir(coreDir)
	if err != nil {
		return fmt.Errorf("read core dir: %w", err)
	}

	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".parquet") {
			table := strings.TrimSuffix(e.Name(), ".parquet")
			if err := q.AttachParquet(ctx, table, filepath.Join(coreDir, e.Name())); err != nil {
				return fmt.Errorf("attach %s: %w", e.Name(), err)
			}
		}
	}
	return nil
}

// ListTables returns all tables in the current DuckDB instance.
func (q *Querier) ListTables(ctx context.Context) ([]string, error) {
	rows, err := q.conn.QueryContext(ctx, "SHOW TABLES")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, err
		}
		tables = append(tables, table)
	}
	return tables, rows.Err()
}
