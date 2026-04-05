package api

import (
	"github.com/continua-ai/continua/internal/config"
	"github.com/continua-ai/continua/internal/ingest"
	"github.com/continua-ai/continua/internal/store"
)

// MaxBodySize is the maximum request body size (5MB).
const MaxBodySize = 5 * 1024 * 1024

const (
	defaultPageLimit = int32(50)
	maxPageLimit     = int32(200)
)

// Server implements the ServerInterface for the Continua API.
type Server struct {
	store                  *store.Store
	ingestService          *ingest.Service
	engineControl          *engineControlService
	enginePublicAPIEnabled bool
}

// NewServer creates a new API server with the given dependencies.
func NewServer(s *store.Store, ingestService *ingest.Service) *Server {
	return &Server{
		store:         s,
		ingestService: ingestService,
	}
}

func newConfiguredServer(s *store.Store, ingestService *ingest.Service, cfg *config.Config) *Server {
	server := NewServer(s, ingestService)
	server.engineControl = newEngineControlService(s)
	if cfg != nil {
		server.enginePublicAPIEnabled = cfg.Engine.PublicAPIEnabled
	}
	return server
}
