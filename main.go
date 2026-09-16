package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

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
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	dataRoot := os.Getenv("AXDATA_ROOT")
	if dataRoot == "" {
		dataRoot = "./axdata_data"
	}

	cfg, err := config.Load(dataRoot)
	if err != nil {
		logger.Fatal("load config", zap.Error(err))
	}

	source.Register(srcTdx.NewDefaultTDXAdapter())
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

	collector, err := collector.NewCollector(cfg, store, logger)
	if err != nil {
		logger.Fatal("init collector", zap.Error(err))
	}

	rootCmd := cmd.NewRootCommand(cfg, store, querier, collector, logger, pluginManager)

	server := api.NewAPIServer(cfg, store, querier, collector, logger, pluginManager)
	rootCmd.AddCommand(cmd.NewAPIServeCmdForServer(server))

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		logger.Fatal("execute command", zap.Error(err))
	}
}
