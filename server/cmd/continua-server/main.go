package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/aryanVijaywargia/Continua/server/internal/version"
	"go.uber.org/zap"
)

func main() {
	// Handle --version flag
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Println(version.String())
		os.Exit(0)
	}

	// Initialize logger
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	logger.Info("Starting Continua server",
		zap.String("version", version.Version),
		zap.String("commit", version.Commit),
		zap.String("build_time", version.BuildTime),
	)

	// Create HTTP mux
	mux := http.NewServeMux()

	// Health endpoint (includes all version info)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok","version":"%s","commit":"%s","build_time":"%s"}`,
			version.Version, version.Commit, version.BuildTime)
	})

	// Placeholder for Connect handlers
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"service":"continua","status":"running"}`)
	})

	// Start HTTP server (Connect/gRPC-Web)
	// Note: gRPC on separate port will be added in future phase
	server := &http.Server{
		Addr:    ":8243",
		Handler: mux,
	}

	// Graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		logger.Info("Shutting down server...")
		server.Shutdown(ctx)
	}()

	logger.Info("Server listening",
		zap.String("http", ":8243"),
		zap.String("note", "gRPC port (8233) will be added in future phase"),
	)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		logger.Fatal("Server error", zap.Error(err))
	}
}
