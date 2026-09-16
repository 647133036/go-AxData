package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/electkismet/axdata-go/core/api"
	"github.com/spf13/cobra"
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
	fmt.Fprintf(os.Stderr, "Starting AxData API server on :%d\n", port)

	// Build routes
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	// http.ListenAndServe takes no context, so a signal can only interrupt it
	// by killing the process. An http.Server lets SIGINT/SIGTERM close
	// in-flight requests and exit cleanly.
	addr := fmt.Sprintf(":%d", port)
	srv := &http.Server{Addr: addr, Handler: mux}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		select {
		case <-sigCh:
		case <-ctx.Done():
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			fmt.Fprintf(os.Stderr, "API server shutdown error: %v\n", err)
			if ferr := srv.Close(); ferr != nil {
				fmt.Fprintf(os.Stderr, "API server close error: %v\n", ferr)
			}
		}
		server.Shutdown(ctx)
	}()

	fmt.Fprintf(os.Stderr, "AxData API server running at http://localhost%s\n", addr)

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "API server error: %v\n", err)
		return
	}

	fmt.Fprintln(os.Stderr, "API server stopped")
}
