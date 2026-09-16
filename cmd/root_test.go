package cmd

import (
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/electkismet/axdata-go/core/collector"
	"github.com/electkismet/axdata-go/core/config"
	"github.com/electkismet/axdata-go/core/plugin"
	"github.com/electkismet/axdata-go/core/query"
	"github.com/electkismet/axdata-go/core/storage"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

// TestRootCommandFailureExitsNonZero guards the CLI contract that a failed
// command returns an error from ExecuteContext. Before this, failing commands
// printed "Error:" to stdout and returned, so scripts that chained axdata calls
// would continue past a broken step.
func TestRootCommandFailureExitsNonZero(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"data inspect unknown table", []string{"data", "inspect", "no_such_table"}},
		{"query unknown table", []string{"query", "--table", "no_such_table"}},
		{"plugin info unknown id", []string{"plugin", "info", "no.such.plugin"}},
		{"request unknown source", []string{"request", "--source", "no_such_source", "--interface", "x"}},
		{"collector task add missing source", []string{"collector", "task", "add", "t1"}},
		{"collector task add missing interface", []string{"collector", "task", "add", "t1", "--source", "mock", "--table", "daily"}},
		{"collector task add missing table", []string{"collector", "task", "add", "t1", "--source", "mock", "--interface", "daily"}},
		{"collector task add malformed params", []string{"collector", "task", "add", "t1", "--source", "mock", "--interface", "daily", "--table", "daily", "--params", "{not json"}},
		{"collector task info unknown id", []string{"collector", "task", "info", "no-such-id"}},
		{"collector task enable unknown id", []string{"collector", "task", "enable", "no-such-id"}},
		{"collector run info unknown id", []string{"collector", "run", "info", "no-such-id"}},
	}

	// A fresh command per case: cobra keeps parsed flag values on the command,
	// so reusing one would leak --table from an earlier case into this one.
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := newTestRootCommand(t)
			root.SetArgs(tc.args)
			if err := root.ExecuteContext(context.Background()); err == nil {
				t.Fatalf("expected an error for %v", tc.args)
			}
		})
	}
}

// newTestRootCommand builds the same wiring main() uses, against a temp data
// root so no test touches the real one.
func newTestRootCommand(t *testing.T) *cobra.Command {
	t.Helper()

	querier, err := query.NewQuerier()
	if err != nil {
		t.Fatalf("NewQuerier: %v", err)
	}
	t.Cleanup(func() { _ = querier.Close() })

	cfg := config.DefaultConfig(t.TempDir())
	store := storage.NewStore(cfg)
	logger := zap.NewNop()

	collectorInst, err := collector.NewCollector(cfg, store, logger)
	if err != nil {
		t.Fatalf("NewCollector: %v", err)
	}

	pm := plugin.NewPluginManager(filepath.Join(t.TempDir(), "metadata", "plugins.json"))
	root := NewRootCommand(cfg, store, querier, collectorInst, logger, pm)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	return root
}

// TestRootCommandSuccessReturnsNil covers the other side of the contract, so
// the failure cases above cannot pass by construction.
func TestRootCommandSuccessReturnsNil(t *testing.T) {
	root := newTestRootCommand(t)

	if err := runArgs(root, []string{"data", "inspect", "daily"}); err != nil {
		t.Fatalf("data inspect daily: %v", err)
	}
	if err := runArgs(root, []string{"data", "list"}); err != nil {
		t.Fatalf("data list: %v", err)
	}
	if err := runArgs(root, []string{"collector", "task", "list"}); err != nil {
		t.Fatalf("collector task list: %v", err)
	}
	if err := runArgs(root, []string{"collector", "status"}); err != nil {
		t.Fatalf("collector status: %v", err)
	}
}

func runArgs(root *cobra.Command, args []string) error {
	root.SetArgs(args)
	return root.ExecuteContext(context.Background())
}

// TestRootCommandTaskRunFailureExitsNonZero pins the collector's error
// propagation: a task that runs but collects nothing must surface as a non-zero
// exit, not a successful run record carrying an error field.
func TestRootCommandTaskRunFailureExitsNonZero(t *testing.T) {
	querier, err := query.NewQuerier()
	if err != nil {
		t.Fatalf("NewQuerier: %v", err)
	}
	t.Cleanup(func() { _ = querier.Close() })

	tmpDir := t.TempDir()
	cfg := config.DefaultConfig(tmpDir)
	store := storage.NewStore(cfg)
	logger := zap.NewNop()

	collectorInst, err := collector.NewCollector(cfg, store, logger)
	if err != nil {
		t.Fatalf("NewCollector: %v", err)
	}

	pm := plugin.NewPluginManager(filepath.Join(tmpDir, "metadata", "plugins.json"))
	root := NewRootCommand(cfg, store, querier, collectorInst, logger, pm)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)

	// No adapter is registered, so the run must fail inside the collector.
	task, err := collectorInst.AddTask("broken", "no.such.source", "x", "daily", "core", nil)
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	if err := collectorInst.UpdateTask(task.ID, map[string]interface{}{"enabled": true}); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	root.SetArgs([]string{"collector", "task", "run", task.ID})
	if err := root.ExecuteContext(context.Background()); err == nil {
		t.Fatal("expected a non-zero exit for a failed run")
	}
}
