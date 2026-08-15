package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/continua-ai/continua/internal/store"
	"github.com/continua-ai/continua/internal/testutil"
)

// localModeRequest builds a request with an explicit RemoteAddr. Loopback
// detection must be derived from the transport peer address only, never from
// caller-supplied proxy headers.
func localModeRequest(method, target, remoteAddr string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	req.RemoteAddr = remoteAddr
	return req
}

func TestLocalModeAllowsUnauthenticatedLoopbackRequest(t *testing.T) {
	authenticator := &Authenticator{localSingleUserMode: true}

	var handlerInvoked bool
	var receivedMode AuthMode
	var modePresent bool
	var projectBound bool
	protectedHandler := authenticator.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerInvoked = true
		receivedMode, modePresent = GetAuthMode(r.Context())
		_, projectBound = GetProjectID(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))

	req := localModeRequest(http.MethodGet, "/api/traces", "127.0.0.1:54321")
	rec := httptest.NewRecorder()

	protectedHandler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.True(t, handlerInvoked, "loopback request should reach the handler in local single-user mode")
	require.True(t, modePresent)
	assert.Equal(t, AuthModeLocalSingleUser, receivedMode)
	assert.False(
		t,
		projectBound,
		"local single-user mode must not bind a project; project_id selects the read scope",
	)
}

func TestLocalModeRejectsNonLoopbackRequest(t *testing.T) {
	authenticator := &Authenticator{localSingleUserMode: true}

	var handlerInvoked bool
	protectedHandler := authenticator.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handlerInvoked = true
		w.WriteHeader(http.StatusNoContent)
	}))

	req := localModeRequest(http.MethodGet, "/api/traces", "203.0.113.5:44444")
	rec := httptest.NewRecorder()

	protectedHandler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.False(t, handlerInvoked, "a remote caller must never reach the handler unauthenticated")

	resp := decodeAuthErrorBody(t, rec)
	assert.Equal(t, "missing_credentials", resp["code"])
}

func TestLocalModeIgnoresForwardedHeaders(t *testing.T) {
	authenticator := &Authenticator{localSingleUserMode: true}

	var handlerInvoked bool
	protectedHandler := authenticator.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handlerInvoked = true
		w.WriteHeader(http.StatusNoContent)
	}))

	req := localModeRequest(http.MethodGet, "/api/traces", "203.0.113.5:44444")
	// Attacker-controlled headers claiming a loopback origin must be ignored.
	req.Header.Set("X-Forwarded-For", "127.0.0.1")
	req.Header.Set("X-Real-IP", "127.0.0.1")
	rec := httptest.NewRecorder()

	protectedHandler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.False(t, handlerInvoked, "spoofed forwarded headers must not unlock local single-user mode")

	resp := decodeAuthErrorBody(t, rec)
	assert.Equal(t, "missing_credentials", resp["code"])
}

func TestLocalModeIPv6LoopbackAllowed(t *testing.T) {
	authenticator := &Authenticator{localSingleUserMode: true}

	var handlerInvoked bool
	var receivedMode AuthMode
	protectedHandler := authenticator.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerInvoked = true
		receivedMode, _ = GetAuthMode(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))

	req := localModeRequest(http.MethodGet, "/api/traces", "[::1]:54321")
	rec := httptest.NewRecorder()

	protectedHandler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.True(t, handlerInvoked, "IPv6 loopback is still loopback")
	assert.Equal(t, AuthModeLocalSingleUser, receivedMode)
}

func TestLocalModeDisabledStillRequiresCredentials(t *testing.T) {
	authenticator := &Authenticator{}

	var handlerInvoked bool
	protectedHandler := authenticator.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handlerInvoked = true
		w.WriteHeader(http.StatusNoContent)
	}))

	req := localModeRequest(http.MethodGet, "/api/traces", "127.0.0.1:54321")
	rec := httptest.NewRecorder()

	protectedHandler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.False(t, handlerInvoked, "the bypass must be opt-in, not implied by a loopback peer")

	resp := decodeAuthErrorBody(t, rec)
	assert.Equal(t, "missing_credentials", resp["code"])
}

func TestLocalModeStillHonorsAPIKey(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	platformStore := store.New(pool)

	apiKey := "local-mode-api-key-" + uuid.NewString()
	project := createCompositeAuthProject(ctx, t, platformStore, apiKey)
	authenticator := &Authenticator{store: platformStore, localSingleUserMode: true}

	var receivedMode AuthMode
	var receivedProjectID uuid.UUID
	protectedHandler := authenticator.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ok bool
		receivedMode, ok = GetAuthMode(r.Context())
		require.True(t, ok)
		receivedProjectID, ok = GetProjectID(r.Context())
		require.True(t, ok)
		w.WriteHeader(http.StatusNoContent)
	}))

	req := localModeRequest(http.MethodGet, "/api/traces", "127.0.0.1:54321")
	req.Header.Set("X-API-Key", apiKey)
	rec := httptest.NewRecorder()

	protectedHandler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, AuthModeAPIKey, receivedMode)
	assert.Equal(t, project.ID, receivedProjectID)
}

func TestLocalModeDoesNotApplyToIngest(t *testing.T) {
	authenticator := &Authenticator{localSingleUserMode: true}

	var handlerInvoked bool
	protectedHandler := authenticator.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handlerInvoked = true
		w.WriteHeader(http.StatusNoContent)
	}))

	req := localModeRequest(http.MethodPost, "/v1/ingest", "127.0.0.1:54321")
	rec := httptest.NewRecorder()

	protectedHandler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.False(t, handlerInvoked, "ingest must never become unauthenticated")

	resp := decodeAuthErrorBody(t, rec)
	assert.Equal(t, "missing_api_key", resp["code"])
}
