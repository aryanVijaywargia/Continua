package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The SPA fallback answers every path the router did not claim. Only a page read may
// get the SPA; an API path or a write must fail visibly instead of a 200 HTML page.
func TestHandlerServesTheSPAOnlyForPageReads(t *testing.T) {
	t.Parallel()

	tests := []struct {
		method     string
		path       string
		wantStatus int
	}{
		{http.MethodGet, "/traces", http.StatusOK},
		{http.MethodHead, "/traces", http.StatusOK},
		{http.MethodPost, "/traces", http.StatusMethodNotAllowed},
		{http.MethodGet, "/v1/ingest", http.StatusNotFound},
		{http.MethodPut, "/v1/ingest", http.StatusNotFound},
		{http.MethodPost, "/v1/unknown", http.StatusNotFound},
		{http.MethodGet, "/api/unknown", http.StatusNotFound},
	}
	handler := Handler()
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))

			assert.Equal(t, tt.wantStatus, rec.Code)
			if tt.wantStatus != http.StatusOK {
				assert.NotContains(t, rec.Header().Get("Content-Type"), "text/html")
			}
		})
	}
}
