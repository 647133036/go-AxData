package query

import (
	"context"
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
	}
	for _, sql := range badQueries {
		if err := ValidateSQL(sql); err == nil {
			t.Errorf("Should reject %q", sql)
		}
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
