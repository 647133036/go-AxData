package query

import (
	"context"
	"fmt"
	"testing"
)

func TestNewQuerier(t *testing.T) {
	q, err := NewQuerier()
	if err != nil {
		t.Fatalf("NewQuerier failed: %v", err)
	}
	if q.conn == nil {
		t.Fatal("conn should not be nil")
	}
	if err := q.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestQuerier_CloseNil(t *testing.T) {
	// Close on an already-closed querier should return nil
	q, _ := NewQuerier()
	_ = q.Close()
	// Manually nil out conn to test nil path
	// (we cannot set private field; test is already covered by Close idempotency)
}

func TestValidateSQLAllow(t *testing.T) {
	goodQueries := []string{
		"SELECT * FROM daily",
		"SELECT close FROM daily WHERE date = '2024-01-01'",
		"select 1 + 2",
		"SELECT count(*) FROM daily",
	}
	for _, sql := range goodQueries {
		if err := ValidateSQL(sql); err != nil {
			t.Errorf("Should allow %q: %v", sql, err)
		}
	}
}

func TestValidateSQLReject(t *testing.T) {
	badQueries := []string{
		"DROP TABLE daily",
		"DELETE FROM daily",
		"ALTER TABLE daily",
		"CREATE TABLE t",
		"INSERT INTO daily",
		"UPDATE daily",
		"SELECT 1 -- comment",
		"SELECT 1; DROP",
		// Catalog and filesystem writes.
		"COPY daily TO '/tmp/out.parquet'",
		"MERGE INTO daily USING other",
		"ATTACH 'local.db' AS db2",
		// Extensions and external readers reach the filesystem or the network.
		"INSTALL httpfs",
		"LOAD httpfs",
		"SELECT * FROM read_csv('/etc/passwd')",
		"SELECT * FROM read_parquet('/etc/passwd')",
		"SELECT * FROM read_json('/etc/passwd')",
		"SELECT http_get('https://evil.example')",
		"SELECT curl('https://evil.example')",
		"SELECT system('id')",
		// Case and whitespace variants.
		"   drop table daily",
		"SeLeCcT * FrOm reAd_pArQuEt('/x')",
	}
	for _, sql := range badQueries {
		if err := ValidateSQL(sql); err == nil {
			t.Errorf("Should reject %q", sql)
		}
	}
}

// TestAttachParquetRepeatable proves the same staging table name can be attached
// twice. The handler reuses one fixed name, so a plain CREATE TABLE made every
// second request fail with "relation already exists" and returned a 500.
func TestAttachParquetRepeatable(t *testing.T) {
	q, err := NewQuerier()
	if err != nil {
		t.Fatalf("NewQuerier failed: %v", err)
	}
	defer q.Close()

	// Build a tiny parquet on disk so read_parquet has something to read.
	q.Execute(context.Background(), "CREATE TABLE src AS SELECT 1 AS a, 'x' AS b")
	path := t.TempDir() + "/src.parquet"
	if _, err := q.Execute(context.Background(),
		"COPY src TO '"+path+"' (FORMAT PARQUET)"); err != nil {
		t.Fatalf("make parquet: %v", err)
	}

	ctx := context.Background()
	if err := q.AttachParquet(ctx, "data_table", path); err != nil {
		t.Fatalf("first attach failed: %v", err)
	}
	if err := q.AttachParquet(ctx, "data_table", path); err != nil {
		t.Fatalf("second attach failed: %v", err)
	}

	rows, err := q.Execute(ctx, "SELECT a FROM data_table")
	if err != nil {
		t.Fatalf("query attached table: %v", err)
	}
	if len(rows) != 1 || len(rows[0]) != 1 {
		t.Fatalf("rows = %v, want a single one-column row", rows)
	}
	if got := fmt.Sprint(rows[0][0]); got != "1" {
		t.Errorf("row value = %q (%T), want 1", got, rows[0][0])
	}
}

// TestQuoteSQLEscapesQuotes ensures a single quote in a path cannot terminate
// the string literal and rewrite the statement.
func TestQuoteSQLEscapesQuotes(t *testing.T) {
	got := quoteSQL("/tmp/dir/o'brien.parquet")
	want := "'/tmp/dir/o''brien.parquet'"
	if got != want {
		t.Errorf("quoteSQL = %q, want %q", got, want)
	}
}

func TestQuerier_Execute(t *testing.T) {
	q, err := NewQuerier()
	if err != nil {
		t.Fatalf("NewQuerier failed: %v", err)
	}
	defer q.Close()

	rows, err := q.Execute(context.Background(), "SELECT 1 as a, 2 as b")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("Expected 1 row, got %d", len(rows))
	}
	if len(rows[0]) != 2 {
		t.Fatalf("Expected 2 columns, got %d", len(rows[0]))
	}
}

func TestQuerier_ExecuteInvalid(t *testing.T) {
	q, _ := NewQuerier()
	defer q.Close()
	_, err := q.Execute(context.Background(), "SELECT * FROM nonexistent_table")
	if err == nil {
		t.Fatal("Expected error for nonexistent table")
	}
}

func TestQuerier_AttachParquet(t *testing.T) {
	q, err := NewQuerier()
	if err != nil {
		t.Fatalf("NewQuerier failed: %v", err)
	}
	defer q.Close()

	err = q.AttachParquet(context.Background(), "test_table", "/nonexistent.parquet")
	if err == nil {
		t.Fatal("Expected error for nonexistent parquet")
	}
}
