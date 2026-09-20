package main

import (
	"context"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/electkismet/axdata-go/cmd"
	"github.com/electkismet/axdata-go/core/api"
	"github.com/electkismet/axdata-go/core/collector"
	"github.com/electkismet/axdata-go/core/config"
	"github.com/electkismet/axdata-go/core/plugin"
	"github.com/electkismet/axdata-go/core/plugin/tencent"
	"github.com/electkismet/axdata-go/core/query"
	"github.com/electkismet/axdata-go/core/source"
	"github.com/electkismet/axdata-go/core/storage"
	srcCls "github.com/electkismet/axdata-source-cls"
	srcCNINFO "github.com/electkismet/axdata-source-cninfo"
	srcEastmoney "github.com/electkismet/axdata-source-eastmoney"
	srcKph "github.com/electkismet/axdata-source-kph"
	srcMock "github.com/electkismet/axdata-source-mock"
	srcSina "github.com/electkismet/axdata-source-sina"
	srcTdx "github.com/electkismet/axdata-source-tdx"
	srcTencent "github.com/electkismet/axdata-source-tencent"
	"github.com/electkismet/axdata-source-ths"
	"github.com/electkismet/axdata-source-wencai"
	"go.uber.org/zap"
)

func main() {
	// This is a CLI: the fatal message is the whole user-facing output, so
	// caller annotations and stacktraces only add noise and leak the local
	// build path. zap recommends both be disabled for production binaries.
	zapCfg := zap.NewProductionConfig()
	zapCfg.DisableCaller = true
	zapCfg.DisableStacktrace = true
	logger, err := zapCfg.Build()
	if err != nil {
		logger, _ = zap.NewProduction()
	}
	defer logger.Sync()

	dataRoot := os.Getenv("AXDATA_ROOT")
	if dataRoot == "" {
		dataRoot = "./axdata_data"
	}
	// --data-root must reach config.Load, so the config file is read from the
	// data root the caller asked for. cobra parses flags inside its command tree,
	// after this point, so the flag is pre-scanned from os.Args here.
	//
	// A hand scan rather than pflag: a subcommand flag such as --port is unknown
	// at this level, and pflag stops there, which would drop a --data-root that
	// merely appears after it.
	dataRoot = prescanDataRoot(dataRoot, os.Args[1:])

	cfg, err := config.Load(dataRoot)
	if err != nil {
		logger.Fatal("load config", zap.Error(err))
	}

	source.Register(srcTdx.NewDefaultTDXAdapter())
	source.Register(srcTdx.NewDefaultTDXExAdapter())
	source.Register(srcTencent.NewTencentAdapter())
	source.Register(srcCNINFO.NewCNINFOAdapter())
	source.Register(srcSina.NewSinaAdapter())
	source.Register(srcEastmoney.NewEastMoneyAdapter())
	source.Register(srcCls.NewCLSAdapter())
	source.Register(srcKph.NewKPHAdapter())
	source.Register(srcMock.NewMockAdapter())

	srcTHS := ths.NewTHSAdapter()
	source.Register(srcTHS)
	srcWencai := wencai.NewWencaiAdapter()
	source.Register(srcWencai)

	pluginManager := plugin.NewPluginManager(cfg.Metadata.PluginsPath)
	if err := pluginManager.Load(); err != nil {
		logger.Error("load plugin metadata", zap.Error(err))
	}

	pluginManager.RegisterProvider(tencent.NewProvider())

	store := storage.NewStore(cfg)

	querier, err := query.NewQuerier()
	if err != nil {
		logger.Fatal("init querier", zap.Error(err))
	}
	defer querier.Close()

	col, err := collector.NewCollector(cfg, store, logger)
	if err != nil {
		logger.Fatal("init collector", zap.Error(err))
	}

	rootCmd := cmd.NewRootCommand(cfg, store, querier, col, logger, pluginManager)

	server := api.NewAPIServer(cfg, store, querier, col, logger, pluginManager)

	// The scheduler only runs while a process lives long enough to own it. axdata
	// api is that process, and scheduler.enabled is opt-in, so the flag is the
	// only thing that starts it.
	var scheduler *collector.Scheduler
	if cfg.Scheduler.Enabled {
		scheduler = collector.NewScheduler(col, logger)
		if cfg.Scheduler.PollIntervalMs > 0 {
			scheduler.SetPollInterval(time.Duration(cfg.Scheduler.PollIntervalMs) * time.Millisecond)
		}
	}
	rootCmd.AddCommand(cmd.NewAPIServeCmdForServer(server, scheduler))

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		logger.Fatal("execute command", zap.Error(err))
	}
}

// prescanDataRoot finds --data-root anywhere in argv and returns the directory
// it names, or fallback. Both --data-root <dir> and --data-root=<dir> work, in
// either position, because cobra binds the flag only inside its own command
// tree and runs after this.
func prescanDataRoot(fallback string, args []string) string {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--data-root" || arg == "-data-root" {
			if i+1 < len(args) {
				i++
				if v := args[i]; v != "" && !strings.HasPrefix(v, "-") {
					fallback = v
				}
			}
			continue
		}
		if v, ok := strings.CutPrefix(arg, "--data-root="); ok {
			fallback = v
		}
	}
	return fallback
}
