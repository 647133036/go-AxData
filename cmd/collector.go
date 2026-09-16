package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newCollectorCmd(r *RootCmd) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "collector",
		Short: "Collector management",
		Long:  "Manage data collection tasks and runs.",
	}

	// collector task
	taskCmd := &cobra.Command{
		Use:   "task",
		Short: "Task management",
		Long:  "Create, list, and manage collection tasks.",
	}

	// task templates
	taskCmd.AddCommand(&cobra.Command{
		Use:   "templates",
		Short: "Show task templates",
		Run: func(cmd *cobra.Command, args []string) {
			r.runTaskTemplates()
		},
	})

	// task list
	taskCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all tasks",
		Run: func(cmd *cobra.Command, args []string) {
			r.runTaskList()
		},
	})

	// task create
	addCmd := &cobra.Command{
		Use:   "add [task-name]",
		Short: "Create a new task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sourceName, _ := cmd.Flags().GetString("source")
			interfaceName, _ := cmd.Flags().GetString("interface")
			table, _ := cmd.Flags().GetString("table")
			paramsStr, _ := cmd.Flags().GetString("params")
			var params map[string]interface{}
			if paramsStr != "" {
				if err := json.Unmarshal([]byte(paramsStr), &params); err != nil {
					return fmt.Errorf("invalid params JSON: %w", err)
				}
			}
			return r.runTaskAdd(args[0], sourceName, interfaceName, table, params)
		},
	}
	addCmd.Flags().String("source", "", "Source name")
	addCmd.Flags().String("interface", "", "Interface name")
	addCmd.Flags().String("table", "", "Target table")
	addCmd.Flags().String("params", "", "Parameters as JSON string (e.g. '{\"symbols\":\"600519.SH\"}')")
	taskCmd.AddCommand(addCmd)

	// task info
	taskCmd.AddCommand(&cobra.Command{
		Use:   "info [task-id]",
		Short: "Show task details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return r.runTaskInfo(args[0])
		},
	})

	// task enable
	taskCmd.AddCommand(&cobra.Command{
		Use:   "enable [task-id]",
		Short: "Enable a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return r.runTaskEnable(args[0], true)
		},
	})

	// task disable
	taskCmd.AddCommand(&cobra.Command{
		Use:   "disable [task-id]",
		Short: "Disable a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return r.runTaskEnable(args[0], false)
		},
	})

	// task run
	taskCmd.AddCommand(&cobra.Command{
		Use:   "run [task-id]",
		Short: "Run a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			return r.runTaskRun(ctx, args[0])
		},
	})

	cmd.AddCommand(taskCmd)

	// collector run
	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Run management",
		Long:  "List and inspect task runs.",
	}

	// run list
	runCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all runs",
		Run: func(cmd *cobra.Command, args []string) {
			r.runRunList()
		},
	})

	// run info
	runCmd.AddCommand(&cobra.Command{
		Use:   "info [run-id]",
		Short: "Show run details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return r.runRunInfo(args[0])
		},
	})

	cmd.AddCommand(runCmd)

	// collector status
	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show collector status",
		Run: func(cmd *cobra.Command, args []string) {
			r.runCollectorStatus()
		},
	})

	return cmd
}

func (r *RootCmd) runTaskTemplates() {
	fmt.Println("Available task templates:")
	fmt.Println()
	fmt.Printf("%-20s %-20s %-25s %s\n", "ID", "SOURCE", "INTERFACE", "DESCRIPTION")
	fmt.Println(strings.Repeat("-", 90))

	templates := []struct{ id, source, iface, desc string }{
		{"daily", "tdx", "daily", "Daily OHLCV data"},
		{"quote", "tdx", "quote", "Real-time quotes"},
		{"stock_list", "tdx", "security_list", "Stock list"},
		{"trade_cal", "cninfo", "calendar", "Trading calendar"},
		{"tencent_quote", "tencent", "quote", "Tencent real-time quote"},
	}

	for _, t := range templates {
		fmt.Printf("%-20s %-20s %-25s %s\n", t.id, t.source, t.iface, t.desc)
	}
}

func (r *RootCmd) runTaskList() {
	tasks := r.collector.ListTasks()
	if len(tasks) == 0 {
		fmt.Println("No tasks configured. Use 'collector task add' to create one.")
		return
	}

	fmt.Printf("%-8s %-20s %-12s %-25s %-12s\n", "ID", "NAME", "SOURCE", "INTERFACE", "STATUS")
	fmt.Println(strings.Repeat("-", 80))
	for _, t := range tasks {
		enabled := "disabled"
		if t.Enabled {
			enabled = "enabled"
		}
		fmt.Printf("%-8s %-20s %-12s %-25s %-12s\n", t.ID, t.Name, t.Source, t.Interface, enabled)
	}
}

func (r *RootCmd) runTaskAdd(name, source, interfaceName, table string, params map[string]interface{}) error {
	if source == "" {
		return errors.New("--source flag required")
	}
	if interfaceName == "" {
		return errors.New("--interface flag required")
	}
	if table == "" {
		return errors.New("--table flag required")
	}

	task, err := r.collector.AddTask(name, source, interfaceName, table, "core", params)
	if err != nil {
		return fmt.Errorf("add task: %w", err)
	}

	fmt.Printf("Task created: %s (ID: %s)\n", task.Name, task.ID)
	return nil
}

func (r *RootCmd) runTaskInfo(taskID string) error {
	task, ok := r.collector.GetTask(taskID)
	if !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}

	fmt.Printf("Task ID:       %s\n", task.ID)
	fmt.Printf("Name:          %s\n", task.Name)
	fmt.Printf("Source:        %s\n", task.Source)
	fmt.Printf("Interface:     %s\n", task.Interface)
	fmt.Printf("Table:         %s\n", task.Table)
	fmt.Printf("Layer:         %s\n", task.Layer)
	fmt.Printf("Enabled:       %v\n", task.Enabled)
	fmt.Printf("Created:       %s\n", task.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Updated:       %s\n", task.UpdatedAt.Format("2006-01-02 15:04:05"))
	return nil
}

func (r *RootCmd) runTaskEnable(taskID string, enable bool) error {
	updates := map[string]interface{}{"enabled": enable}
	err := r.collector.UpdateTask(taskID, updates)
	if err != nil {
		return fmt.Errorf("update task: %w", err)
	}

	action := "enabled"
	if !enable {
		action = "disabled"
	}
	fmt.Printf("Task %s: %s\n", taskID, action)
	return nil
}

func (r *RootCmd) runTaskRun(ctx context.Context, taskID string) error {
	fmt.Printf("Running task: %s\n", taskID)
	run, err := r.collector.RunTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("run error: %w", err)
	}

	fmt.Printf("Run ID:    %s\n", run.RunID)
	fmt.Printf("Status:    %s\n", run.Status)
	fmt.Printf("Rows:      %d\n", run.Rows)
	if run.Error != "" {
		fmt.Printf("Error:     %s\n", run.Error)
	}
	return nil
}

func (r *RootCmd) runRunList() {
	runs := r.collector.ListRuns()
	if len(runs) == 0 {
		fmt.Println("No runs yet.")
		return
	}

	fmt.Printf("%-8s %-20s %-10s %-8s\n", "RUN_ID", "TASK_ID", "STATUS", "ROWS")
	fmt.Println(strings.Repeat("-", 50))
	for _, r := range runs {
		fmt.Printf("%-8s %-20s %-10s %-8d\n", r.RunID, r.TaskID, r.Status, r.Rows)
	}
}

func (r *RootCmd) runRunInfo(runID string) error {
	run, ok := r.collector.GetRun(runID)
	if !ok {
		return fmt.Errorf("run not found: %s", runID)
	}

	data, _ := json.MarshalIndent(run, "", "  ")
	fmt.Printf("%s\n", data)
	return nil
}

func (r *RootCmd) runCollectorStatus() {
	tasks := r.collector.ListTasks()
	runs := r.collector.ListRuns()

	fmt.Printf("Collector Status\n")
	fmt.Printf("================\n")
	fmt.Printf("Total tasks: %d\n", len(tasks))
	enabled := 0
	for _, t := range tasks {
		if t.Enabled {
			enabled++
		}
	}
	fmt.Printf("Enabled tasks: %d\n", enabled)
	fmt.Printf("Total runs: %d\n", len(runs))

	// Show recent runs
	fmt.Printf("\nRecent runs:\n")
	if len(runs) > 0 {
		fmt.Printf("  Last run: %s (status: %s, rows: %d)\n", runs[0].RunID, runs[0].Status, runs[0].Rows)
	}
}
