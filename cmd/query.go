package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/electkismet/axdata-go/core/schema"
	"github.com/spf13/cobra"
)

// queryPreviewRows caps how many rows a query prints. The full count is reported
// separately so the cap cannot be mistaken for the table size.
const queryPreviewRows = 10

func newQueryCmd(r *RootCmd) *cobra.Command {
	return &cobra.Command{
		Use:   "query TABLE",
		Short: "Query data from a table",
		Long:  "Preview the first rows of an AxData table using DuckDB.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return r.runQuery(cmd.Context(), args[0])
		},
	}
}

func (r *RootCmd) runQuery(ctx context.Context, table string) error {
	// The table name lands in both a SQL statement and a file path, so an
	// arbitrary argument would allow SQL injection as well as path traversal.
	// Only registered table names are accepted.
	ts := schema.GetSchema(table)
	if ts == nil {
		return fmt.Errorf("unknown table %q; run 'axdata data list' for available tables", table)
	}

	parquetPath := r.cfg.CorePath(table)
	if _, err := os.Stat(parquetPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("table %q has no data at %s", table, parquetPath)
		}
		return fmt.Errorf("read table path: %w", err)
	}

	const staging = "axdata_query_stage"
	if err := r.querier.AttachParquet(ctx, staging, parquetPath); err != nil {
		return fmt.Errorf("attach table: %w", err)
	}

	results, columns, err := r.querier.ExecuteWithColumns(ctx,
		fmt.Sprintf("SELECT * FROM %s LIMIT %d", staging, queryPreviewRows))
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}

	if len(results) == 0 {
		fmt.Fprintln(os.Stderr, "No results")
		return nil
	}

	rows := make([][]string, len(results))
	for i, row := range results {
		rows[i] = make([]string, len(row))
		for j, v := range row {
			if v == nil {
				rows[i][j] = "NULL"
			} else {
				rows[i][j] = fmt.Sprintf("%v", v)
			}
		}
	}
	printTable(columns, rows)

	count, err := r.store.Count(ts.Layer, table)
	if err != nil {
		return fmt.Errorf("count rows: %w", err)
	}

	if count > queryPreviewRows {
		fmt.Fprintf(os.Stderr, "\nShowing %d of %d rows\n", len(rows), count)
		return nil
	}
	fmt.Fprintf(os.Stderr, "\n%d rows\n", count)
	return nil
}

const cellMaxWidth = 24

// printTable renders headers and rows with column widths derived from the
// widest value in each column.
func printTable(columns []string, rows [][]string) {
	widths := make([]int, len(columns))
	for i, col := range columns {
		w := len(col)
		for _, row := range rows {
			if i < len(row) && len(row[i]) > w {
				w = len(row[i])
			}
		}
		if w > cellMaxWidth {
			w = cellMaxWidth
		}
		widths[i] = w
	}

	printCells(columns, widths)
	fmt.Println(strings.Repeat("-", headerWidth(widths)))
	for _, row := range rows {
		padded := make([]string, len(widths))
		for i, w := range widths {
			if i < len(row) {
				padded[i] = row[i]
			}
			padded[i] = padRight(padded[i], w)
		}
		fmt.Println(strings.Join(padded, " "))
	}
}

func printCells(values []string, widths []int) {
	line := make([]string, len(values))
	for i, w := range widths {
		if i < len(values) {
			line[i] = values[i]
		}
		line[i] = padRight(line[i], w)
	}
	fmt.Println(strings.Join(line, " "))
}

func headerWidth(widths []int) int {
	total := 0
	for i, w := range widths {
		if i == 0 {
			total = w
		} else {
			total += w + 1
		}
	}
	return total
}

func padRight(s string, width int) string {
	if len(s) > width {
		s = s[:width]
	}
	return s + strings.Repeat(" ", width-len(s))
}
