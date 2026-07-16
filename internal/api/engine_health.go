package api

import "net/http"

func (s *Server) GetEngineHealth(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented, "not_implemented", "engine health not implemented")
}
