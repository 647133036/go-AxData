package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/electkismet/axdata-go/core/api"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

// NewAPIServeCmdForServer creates an API serve command with a pre-built server.
func NewAPIServeCmdForServer(server *api.APIServer) *cobra.Command {
	var port int

	cmd := &cobra.Command{
		Use:   "api",
		Short: "Start the HTTP API server",
		Long:  "Start the AxData HTTP API server for data querying and management.",
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()
			runAPIServeForServer(ctx, server, port)
		},
	}

	cmd.Flags().IntVar(&port, "port", 8080, "API server port")

	return cmd
}

func runAPIServeForServer(ctx context.Context, server *api.APIServer, port int) {
	fmt.Printf("Starting AxData API server on :%d\n", port)

	// Build routes
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case <-sigCh:
			server.Shutdown(ctx)
		case <-ctx.Done():
			server.Shutdown(ctx)
		}
	}()

	addr := fmt.Sprintf(":%d", port)
	fmt.Printf("AxData API server running at http://localhost%s\n", addr)

	if err := http.ListenAndServe(addr, mux); err != nil && err != http.ErrServerClosed {
		fmt.Printf("API server error: %v\n", err)
	}

	fmt.Println("API server stopped")
}

func newAPIServeCmd(r *RootCmd) *cobra.Command {
	var port int

	cmd := &cobra.Command{
		Use:   "api",
		Short: "Start the HTTP API server",
		Long:  "Start the AxData HTTP API server for data querying and management.",
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()
			r.runAPIServe(ctx, port)
		},
	}

	cmd.Flags().IntVar(&port, "port", 8080, "API server port")

	return cmd
}

func (r *RootCmd) runAPIServe(ctx context.Context, port int) {
	fmt.Printf("Starting AxData API server on :%d\n", port)

	// Build routes
	mux := http.NewServeMux()
	server := api.NewAPIServer(r.cfg, r.store, r.querier, r.collector, r.logger, r.pluginMgr)
	server.RegisterRoutes(mux)

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case <-sigCh:
			r.logger.Info("received shutdown signal")
			server.Shutdown(ctx)
		case <-ctx.Done():
			r.logger.Info("context cancelled")
			server.Shutdown(ctx)
		}
	}()

	addr := fmt.Sprintf(":%d", port)
	fmt.Printf("AxData API server running at http://localhost%s\n", addr)

	if err := http.ListenAndServe(addr, mux); err != nil && err != http.ErrServerClosed {
		r.logger.Fatal("API server error", zap.Error(err))
	}

	fmt.Println("API server stopped")
}
