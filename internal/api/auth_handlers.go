package api

import (
	"net/http"

	"github.com/continua-ai/continua/internal/api/middleware"
)

// GetAuthConfig returns the runtime bootstrap configuration for the web debugger.
// Hosted operator login was removed, so the Auth0 fields are always empty; the
// response shape stays so existing debuggers keep parsing it.
func (s *Server) GetAuthConfig(w http.ResponseWriter, r *http.Request) {
	response := AuthConfig{
		Enabled: false,
	}

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
