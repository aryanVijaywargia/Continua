package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	pgmigrations "github.com/continua-ai/continua/db/platform/migrations/postgres"
	"github.com/continua-ai/continua/internal/api"
	"github.com/continua-ai/continua/internal/config"
	"github.com/continua-ai/continua/internal/enginecontrol"
	"github.com/continua-ai/continua/internal/ingest"
	"github.com/continua-ai/continua/internal/jobs"
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
	rootCmd.AddCommand(migrateCmd())
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
	app := newServerApp()
	app.Run()
	return nil
}

func newServerApp() *fx.App {
	return fx.New(
		// Provide configuration
		config.Module,

		// Provide database access
		store.Module,

		// Provide shared engine control orchestration
		enginecontrol.Module,

		// Provide async job processing (River)
		jobs.Module,

		// Provide ingest service
		ingest.Module,

		// Provide API handlers
		api.Module,

		// Start HTTP server
		fx.Invoke(startHTTPServer),
	)
}

func migrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Run database migrations",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "up",
		Short: "Apply all pending migrations",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMigrationsUp()
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "down [steps]",
		Short: "Rollback migrations (default: 1 step)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			steps := 1
			if len(args) == 1 {
				n, err := strconv.Atoi(args[0])
				if err != nil || n < 1 {
					return fmt.Errorf("invalid steps %q: must be a positive integer", args[0])
				}
				steps = n
			}
			return runMigrationsDown(steps)
		},
	})

	return cmd
}

func newMigrator(databaseURL string) (*migrate.Migrate, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create postgres migration driver: %w", err)
	}

	source, err := iofs.New(pgmigrations.Migrations, ".")
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create migration source: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", source, "postgres", driver)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create migrator: %w", err)
	}

	return m, nil
}

func runMigrationsUp() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	m, err := newMigrator(cfg.Database.URL)
	if err != nil {
		return err
	}
	defer closeMigrator(m)

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}

	fmt.Println("Migrations applied successfully")
	return nil
}

func runMigrationsDown(steps int) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	m, err := newMigrator(cfg.Database.URL)
	if err != nil {
		return err
	}
	defer closeMigrator(m)

	if err := m.Steps(-steps); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("rollback migrations: %w", err)
	}

	fmt.Printf("Rolled back %d migration step(s)\n", steps)
	return nil
}

func closeMigrator(m *migrate.Migrate) {
	srcErr, dbErr := m.Close()
	if srcErr != nil {
		fmt.Fprintf(os.Stderr, "warning: close migration source: %v\n", srcErr)
	}
	if dbErr != nil {
		fmt.Fprintf(os.Stderr, "warning: close migration db: %v\n", dbErr)
	}
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
