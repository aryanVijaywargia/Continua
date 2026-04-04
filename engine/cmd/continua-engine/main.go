package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/golang-migrate/migrate/v4"
	"github.com/spf13/cobra"

	enginepgmigrations "github.com/continua-ai/continua/engine/db/migrations/postgres"
	"github.com/continua-ai/continua/engine/internal/config"
	"github.com/continua-ai/continua/engine/internal/migrations"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	rootCmd := newRootCmd()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "continua-engine",
		Short: "Continua durable execution engine",
		Long:  "Continua Engine manages the durable execution engine schema and runtime foundations.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	rootCmd.AddCommand(versionCmd())
	rootCmd.AddCommand(migrateCmd())
	return rootCmd
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print engine version information",
		Run: func(cmd *cobra.Command, args []string) {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "continua-engine %s (commit: %s, built: %s)\n", version, commit, date)
		},
	}
}

func migrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Run engine database migrations",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "up",
		Short: "Apply all pending engine migrations",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMigrationsUp(cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "down [steps]",
		Short: "Rollback engine migrations",
		Args:  requireDownStepCount,
		RunE: func(cmd *cobra.Command, args []string) error {
			steps, err := strconv.Atoi(args[0])
			if err != nil || steps < 1 {
				return fmt.Errorf("invalid steps %q: must be a positive integer", args[0])
			}
			return runMigrationsDown(cmd.OutOrStdout(), cmd.ErrOrStderr(), steps)
		},
	})

	return cmd
}

func requireDownStepCount(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return errors.New("step count required: use continua-engine migrate down <steps>")
	}
	if len(args) > 1 {
		return fmt.Errorf("accepts 1 arg(s), received %d", len(args))
	}
	return nil
}

func newMigrator(databaseURL string) (*migrate.Migrate, error) {
	return migrations.New(databaseURL, enginepgmigrations.Migrations)
}

func runMigrationsUp(stdout, stderr io.Writer) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	migrator, err := newMigrator(cfg.Database.URL)
	if err != nil {
		return err
	}
	defer closeMigrator(migrator, stderr)

	if err := migrator.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply engine migrations: %w", err)
	}

	_, _ = fmt.Fprintln(stdout, "Engine migrations applied successfully")
	return nil
}

func runMigrationsDown(stdout, stderr io.Writer, steps int) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	migrator, err := newMigrator(cfg.Database.URL)
	if err != nil {
		return err
	}
	defer closeMigrator(migrator, stderr)

	if err := migrator.Steps(-steps); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("rollback engine migrations: %w", err)
	}

	_, _ = fmt.Fprintf(stdout, "Rolled back %d engine migration step(s)\n", steps)
	return nil
}

func closeMigrator(migrator *migrate.Migrate, stderr io.Writer) {
	if err := migrations.Close(migrator); err != nil {
		_, _ = fmt.Fprintf(stderr, "warning: close engine migrator: %v\n", err)
	}
}
