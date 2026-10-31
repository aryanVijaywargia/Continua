package api

import (
	"net/http"

	"github.com/continua-ai/continua/internal/api/middleware"
)

// GetAuthConfig tells the web debugger which credential-free access modes this
// connection may use: the public demo and loopback local single-user mode.
func (s *Server) GetAuthConfig(w http.ResponseWriter, r *http.Request) {
	var response AuthConfig

	// Only ever advertise the credential-free bypass to a caller that could
	// actually use it. A remote client must not learn the mode exists.
	if s.localSingleUserMode && middleware.IsLoopbackRequest(r) {
		response.LocalModeEnabled = boolValuePtr(true)
	}

	if s.publicDemoConfig.Enabled {
		response.PublicDemoEnabled = boolValuePtr(true)
		response.PublicDemoLabel = &s.publicDemoConfig.Label
	}

	writeJSON(w, http.StatusOK, response)
}

func boolValuePtr(v bool) *bool {
	return &v
}
