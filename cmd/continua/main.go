package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
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
			fmt.Println("Starting Continua server...")
			fmt.Println("TODO: Implement server startup with Fx")
			return nil
		},
	}
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
