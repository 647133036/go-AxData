package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/electkismet/axdata-go/core/schema"
	"github.com/spf13/cobra"
)

func newDataCmd(r *RootCmd) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "data",
		Short: "Data table operations",
		Long:  "List, inspect, and preview data tables.",
	}

	// data list
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List available data tables",
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()
			r.runDataList(ctx)
		},
	})

	// data inspect
	cmd.AddCommand(&cobra.Command{
		Use:   "inspect [table]",
		Short: "Inspect a data table",
		Long:  "Show detailed information about a data table including schema and record count.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			return r.runDataInspect(ctx, args[0])
		},
	})

	// data preview
	cmd.AddCommand(&cobra.Command{
		Use:   "preview [table]",
		Short: "Preview data from a table",
		Long:  "Preview rows from a data table with optional filters.",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()
			r.runDataPreview(ctx, args[0])
		},
	})

	return cmd
}

func (r *RootCmd) runDataList(ctx context.Context) {
	fmt.Println("Available tables:")
	fmt.Println()
	fmt.Printf("%-25s %-12s %-10s %s\n", "TABLE", "LAYER", "WRITE_MODE", "DESCRIPTION")
	fmt.Println(strings.Repeat("-", 90))

	for name, ts := range schema.TableRegistry {
		fmt.Printf("%-25s %-12s %-10s %s\n", name, ts.Layer, ts.WriteMode, ts.Description)
	}
}

func (r *RootCmd) runDataInspect(ctx context.Context, table string) error {
	schemaDef := schema.GetSchema(table)
	if schemaDef == nil {
		return fmt.Errorf("table not found: %s", table)
	}

	fmt.Printf("Table: %s\n", table)
	fmt.Printf("Description: %s\n", schemaDef.Description)
	fmt.Printf("Layer: %s\n", schemaDef.Layer)
	fmt.Printf("Write Mode: %s\n", schemaDef.WriteMode)
	fmt.Printf("Primary Keys: %v\n", schemaDef.PrimaryKeys)
	fmt.Printf("Columns (%d):\n", len(schemaDef.Columns))

	fmt.Printf("  %-15s %-10s %-8s %s\n", "NAME", "TYPE", "PK", "DESCRIPTION")
	fmt.Println("  " + strings.Repeat("-", 60))
	for _, col := range schemaDef.Columns {
		pk := ""
		if col.IsPK {
			pk = "Y"
		}
		fmt.Printf("  %-15s %-10s %-8s %s\n", col.Name, col.Type, pk, col.Description)
	}

	// Show record count
	count, err := r.store.Count("core", table)
	if err != nil {
		return fmt.Errorf("count records: %w", err)
	}
	fmt.Printf("\nRecords in core layer: %d\n", count)
	return nil
}

func (r *RootCmd) runDataPreview(ctx context.Context, table string) {
	// Preview logic - query first few rows
	_ = ctx
	_ = table
	fmt.Printf("Preview for table: %s\n", table)
	fmt.Println("(Query implementation will use DuckDB to fetch preview rows)")
}
