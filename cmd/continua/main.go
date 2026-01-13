package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/continua-ai/continua/internal/api"
	"github.com/continua-ai/continua/internal/config"
	"github.com/continua-ai/continua/internal/ingest"
	"github.com/continua-ai/continua/internal/store"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "continua",
		Short: "Continua - AI Agent Observability Platform",
		Long:  `Continua helps you debug AI agents by capturing and replaying their execution traces.`,
	}

	rootCmd.AddCommand(serveCmd())
	rootCmd.AddCommand(versionCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func serveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the Continua server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServer()
		},
	}
}

func runServer() error {
	app := fx.New(
		// Provide configuration
		config.Module,

		// Provide database access
		store.Module,

		// Provide ingest service
		ingest.Module,

		// Provide API handlers
		api.Module,

		// Start HTTP server
		fx.Invoke(startHTTPServer),
	)

	app.Run()
	return nil
}

// startHTTPServer starts the HTTP server with graceful shutdown.
func startHTTPServer(lc fx.Lifecycle, cfg *config.Config, handler http.Handler) {
	server := &http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// Start server in a goroutine
			go func() {
				fmt.Printf("Starting Continua server on %s\n", cfg.Server.Address())
				if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
				}
			}()

			return nil
		},
		OnStop: func(ctx context.Context) error {
			fmt.Println("Stopping HTTP server...")
			return server.Shutdown(ctx)
		},
	})
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("continua %s (commit: %s, built: %s)\n", version, commit, date)
		},
	}
}
