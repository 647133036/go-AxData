package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/electkismet/axdata-go/core/cache"
	"github.com/electkismet/axdata-go/core/collector"
	"github.com/electkismet/axdata-go/core/config"
	"github.com/electkismet/axdata-go/core/plugin"
	"github.com/electkismet/axdata-go/core/query"
	"github.com/electkismet/axdata-go/core/schema"
	"github.com/electkismet/axdata-go/core/storage"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

// RootCmd holds the root CLI command.
type RootCmd struct {
	cfg       *config.Config
	store     *storage.Store
	querier   *query.Querier
	collector *collector.Collector
	pluginMgr *plugin.PluginManager
	logger    *zap.Logger
	getter    *cache.Getter

	cmd *cobra.Command
}

// NewRootCommand creates the root command with all subcommands.
func NewRootCommand(cfg *config.Config, store *storage.Store, querier *query.Querier, collector *collector.Collector, logger *zap.Logger, pluginMgr *plugin.PluginManager) *cobra.Command {
	root := &RootCmd{
		cfg:       cfg,
		store:     store,
		querier:   querier,
		collector: collector,
		pluginMgr: pluginMgr,
		logger:    logger,
		getter:    cache.New(cfg, store, querier, logger),
	}

	cmd := &cobra.Command{
		Use:   "axdata",
		Short: "AxData - Quantitative Data Platform",
		Long:  "AxData is a Go rewrite of the AxData quantitative data database framework for Chinese A-share market data.",
	}

	cmd.PersistentFlags().StringVar(&cfg.DataRoot, "data-root", cfg.DataRoot, "Root directory for AxData data")

	// Paths are derived from DataRoot at construction time, so re-derive them
	// once the flag has been parsed or they keep pointing at the default root.
	cmd.PersistentPreRun = func(c *cobra.Command, args []string) {
		cfg.SetDataRoot(cfg.DataRoot)

		// The collector and plugin manager were built before this flag existed
		// and already loaded metadata from the default root. Repoint and reload
		// so `task add` written under a custom root is visible to `task list`.
		pluginMgr.SetMetadataPath(cfg.Metadata.PluginsPath)
		if err := pluginMgr.Reload(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: reload plugin metadata: %v\n", err)
		}
		if err := collector.ReloadMetadata(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: reload collector metadata: %v\n", err)
		}

		// Written to stderr so that commands emitting JSON on stdout stay
		// machine-parseable when piped.
		fmt.Fprintf(os.Stderr, "AxData Go v2.0.0 - Quantitative Data Platform\n")
		fmt.Fprintf(os.Stderr, "Data root: %s\n", cfg.DataRoot)
		fmt.Fprintf(os.Stderr, "Tables available: %d\n", len(schema.TableRegistryNames()))
	}

	root.cmd = cmd
	root.registerCommands()

	return cmd
}

// RegisterAPICmd adds the API server command to the root.
func RegisterAPICmd(root *cobra.Command, apiCmd *cobra.Command) {
	root.AddCommand(apiCmd)
}

func (r *RootCmd) registerCommands() {
	// init
	r.cmd.AddCommand(newInitCmd(r))
	// config
	r.cmd.AddCommand(newConfigCmd(r))
	// doctor
	r.cmd.AddCommand(newDoctorCmd(r))
	// data
	r.cmd.AddCommand(newDataCmd(r))
	// query
	r.cmd.AddCommand(newQueryCmd(r))
	// request
	r.cmd.AddCommand(newRequestCmd(r))
	// collector
	r.cmd.AddCommand(newCollectorCmd(r))
	r.addPluginCmd()
	// analyst suite
	r.cmd.AddCommand(newMarketCmd(r))
	r.cmd.AddCommand(newFundamentalCmd(r))
	r.cmd.AddCommand(newEarningsCmd(r))
	r.cmd.AddCommand(newValuationCmd(r))
	r.cmd.AddCommand(newPortfolioCmd(r))
}

// initCmd initializes the data root.
func newInitCmd(r *RootCmd) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize AxData data root",
		Long:  "Create the directory structure and default configuration for AxData.",
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()
			r.runInit(ctx)
		},
	}
}

func (r *RootCmd) runInit(ctx context.Context) {
	dataRoot := r.cfg.DataRoot

	dirs := []string{
		dataRoot + "/data/raw",
		dataRoot + "/data/staging",
		dataRoot + "/data/core",
		dataRoot + "/data/factor",
		dataRoot + "/metadata",
		dataRoot + "/cache",
		dataRoot + "/logs",
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			r.logger.Fatal("create directory", zap.String("path", dir), zap.Error(err))
		}
	}

	if err := r.cfg.Save(); err != nil {
		r.logger.Fatal("save config", zap.Error(err))
	}

	fmt.Printf("AxData initialized at: %s\n", dataRoot)
	for _, dir := range dirs {
		fmt.Printf("  %s/\n", dir)
	}
}

// configCmd shows current configuration.
func newConfigCmd(r *RootCmd) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show AxData configuration",
		Long:  "Display the current AxData configuration.",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Show current config",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Data Root:  %s\n", r.cfg.DataRoot)
			fmt.Printf("API Port:   %d\n", r.cfg.APIPort)
			fmt.Printf("Log Level:  %s\n", r.cfg.LogLevel)
			fmt.Printf("Max Tasks:  %d\n", r.cfg.Collector.MaxConcurrentTasks)
		},
	})

	return cmd
}

// doctorCmd checks the environment.
func newDoctorCmd(r *RootCmd) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check AxData environment",
		Long:  "Verify the AxData installation and environment.",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			return r.runDoctor(ctx)
		},
	}
}

func (r *RootCmd) runDoctor(ctx context.Context) error {
	checks := []struct {
		name string
		fn   func() error
	}{
		{"data root exists", func() error {
			_, err := os.Stat(r.cfg.DataRoot)
			if err == nil {
				return nil
			}
			return fmt.Errorf("not found: %s", r.cfg.DataRoot)
		}},
		{"core directory", func() error {
			_, err := os.Stat(r.cfg.DataLayers.Core)
			return err
		}},
		{"metadata directory", func() error {
			_, err := os.Stat(filepath.Join(r.cfg.DataRoot, "metadata"))
			return err
		}},
		{"store available", func() error {
			_ = r.store
			return nil
		}},
	}

	allPassed := true
	for _, check := range checks {
		err := check.fn()
		if err != nil {
			fmt.Printf("  [FAIL] %s: %v\n", check.name, err)
			allPassed = false
		} else {
			fmt.Printf("  [OK]   %s\n", check.name)
		}
	}

	if allPassed {
		fmt.Printf("\nAll checks passed!\n")
		return nil
	}
	fmt.Printf("\nSome checks failed. Please fix the issues above.\n")
	return errors.New("environment checks failed")
}
