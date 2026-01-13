package config

import (
	"errors"
	"os"
)

// Config holds the application configuration.
// For Phase 2, configuration is loaded from environment variables only.
type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
}

// ServerConfig holds HTTP server configuration.
type ServerConfig struct {
	Host string
	Port string
}

// DatabaseConfig holds database connection configuration.
type DatabaseConfig struct {
	URL string
}

// Address returns the server address in host:port format.
func (s ServerConfig) Address() string {
	return s.Host + ":" + s.Port
}

// Load loads configuration from environment variables.
// Required: DATABASE_URL
// Optional: HOST (default: 0.0.0.0), PORT (default: 8080)
func Load() (*Config, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, errors.New("DATABASE_URL environment variable is required")
	}

	host := os.Getenv("HOST")
	if host == "" {
		host = "0.0.0.0"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	return &Config{
		Server: ServerConfig{
			Host: host,
			Port: port,
		},
		Database: DatabaseConfig{
			URL: dbURL,
		},
	}, nil
}
