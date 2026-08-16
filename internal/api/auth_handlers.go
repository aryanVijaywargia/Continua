package api

import (
	"net/http"

	"github.com/continua-ai/continua/internal/api/middleware"
)

// GetAuthConfig returns the runtime Auth0 bootstrap configuration for the web debugger.
func (s *Server) GetAuthConfig(w http.ResponseWriter, r *http.Request) {
	response := AuthConfig{
		Enabled: s.auth0Config.Enabled,
	}

	// Only ever advertise the credential-free bypass to a caller that could
	// actually use it. A remote client must not learn the mode exists.
	if s.localSingleUserMode && middleware.IsLoopbackRequest(r) {
		response.LocalModeEnabled = boolValuePtr(true)
	}

	if s.publicDemoConfig.Enabled {
		response.Enabled = false
		response.PublicDemoEnabled = boolValuePtr(true)
		response.PublicDemoLabel = &s.publicDemoConfig.Label
		writeJSON(w, http.StatusOK, response)
		return
	}

	if s.auth0Config.Enabled {
		response.Domain = &s.auth0Config.Domain
		response.ClientId = &s.auth0Config.ClientID
		response.Audience = &s.auth0Config.Audience
	}

	writeJSON(w, http.StatusOK, response)
}

func boolValuePtr(v bool) *bool {
	return &v
}
