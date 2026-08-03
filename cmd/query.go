package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newQueryCmd(r *RootCmd) *cobra.Command {
	return &cobra.Command{
		Use:   "query [table]",
		Short: "Query data from tables",
		Long:  "Execute SQL queries against AxData tables using DuckDB.",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()
			r.runQuery(ctx, args[0])
		},
	}
}

func (r *RootCmd) runQuery(ctx context.Context, table string) {
	// Build a simple SELECT query
	query := fmt.Sprintf("SELECT * FROM %s LIMIT 10", table)

	// Attach parquet file
	corePath := r.cfg.CorePath(table)

	results, err := r.querier.Execute(ctx,
		fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s AS SELECT * FROM read_parquet('%s')
	`, table, corePath))

	if err != nil {
		fmt.Printf("Error attaching table: %v\n", err)
		// Try listing available tables
		r.runDataList(ctx)
		return
	}

	results, err = r.querier.Execute(ctx, query)
	if err != nil {
		fmt.Printf("Query error: %v\n", err)
		return
	}

	// Format output
	if len(results) == 0 {
		fmt.Println("No results")
		return
	}

	// Print header
	if len(results) > 0 {
		fmt.Printf("%-25s %-12s %s\n", "TS_CODE", "TRADE_DATE", "COLUMNS")
		fmt.Println(strings.Repeat("-", 70))
		for _, row := range results {
			if len(row) >= 2 {
				fmt.Printf("%-25s %-12s %v\n", row[0], row[1], row[2:])
			}
		}
	}

	fmt.Printf("\nTotal rows: %d\n", len(results))
}

func formatRow(row []interface{}) []string {
	var result []string
	for _, v := range row {
		if v == nil {
			result = append(result, "NULL")
		} else {
			result = append(result, fmt.Sprintf("%v", v))
		}
	}
	return result
}
