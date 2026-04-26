package api

import "net/http"

// GetAuthConfig returns the runtime Auth0 bootstrap configuration for the web debugger.
func (s *Server) GetAuthConfig(w http.ResponseWriter, _ *http.Request) {
	response := AuthConfig{
		Enabled: s.auth0Config.Enabled,
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
