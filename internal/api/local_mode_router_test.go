package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/continua-ai/continua/internal/api/middleware"
	"github.com/continua-ai/continua/internal/config"
)

// These tests exercise the REAL router, not a bare Authenticator, because the
// vulnerability they guard lives in the middleware ordering rather than in the
// auth logic. chi's RealIP middleware rewrites r.RemoteAddr from caller-supplied
// proxy headers, so without middleware.CaptureTransportPeer mounted ahead of it a
// remote client can claim to be loopback and unlock local single-user mode. Only
// a test that runs the whole stack can see that.

// remoteNonLoopbackAddr is a transport peer that must never be served
// unauthenticated, whatever its headers claim.
const remoteNonLoopbackAddr = "192.168.68.61:44444"

// spoofedLoopbackHeaders are the headers chi's RealIP consults, plus RFC 7239
// Forwarded, which no middleware honors today but might tomorrow.
func spoofedLoopbackHeaders() map[string]string {
	return map[string]string{
		"X-Forwarded-For": "127.0.0.1",
		"X-Real-IP":       "127.0.0.1",
		"True-Client-IP":  "127.0.0.1",
		"Forwarded":       "for=127.0.0.1",
	}
}

func serveLocalModeRouterRequest(
	t *testing.T,
	router http.Handler,
	target string,
	remoteAddr string,
	headers map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.RemoteAddr = remoteAddr
	for name, value := range headers {
		req.Header.Set(name, value)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestLocalModeRouterRejectsSpoofedForwardedForHeader(t *testing.T) {
	fixture := newLocalModeScopeFixture(t)
	target := "/api/traces?project_id=" + fixture.projectA.ID.String()

	for header, value := range spoofedLoopbackHeaders() {
		t.Run(header, func(t *testing.T) {
			rec := serveLocalModeRouterRequest(
				t,
				fixture.router,
				target,
				remoteNonLoopbackAddr,
				map[string]string{header: value},
			)

			require.Equal(
				t,
				http.StatusUnauthorized,
				rec.Code,
				"a remote peer must not unlock local single-user mode via %s", header,
			)

			body := rec.Body.String()
			assert.NotContains(t, body, fixture.traceA.ID.String(), "leaked project A data")
			assert.NotContains(t, body, fixture.traceB.ID.String(), "leaked project B data")
		})
	}
}

func TestLocalModeRouterRejectsRemotePeerWithoutHeaders(t *testing.T) {
	fixture := newLocalModeScopeFixture(t)

	rec := serveLocalModeRouterRequest(
		t,
		fixture.router,
		"/api/traces?project_id="+fixture.projectA.ID.String(),
		remoteNonLoopbackAddr,
		nil,
	)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.NotContains(t, rec.Body.String(), fixture.traceA.ID.String())
}

func TestLocalModeRouterAllowsGenuineLoopbackPeer(t *testing.T) {
	fixture := newLocalModeScopeFixture(t)

	rec := serveLocalModeRouterRequest(
		t,
		fixture.router,
		"/api/traces?project_id="+fixture.projectA.ID.String(),
		"127.0.0.1:54321",
		nil,
	)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	resp := decodeJSONBody[TraceList](t, rec)
	assert.Equal(t, 1, resp.Total)
	require.Len(t, resp.Traces, 1)
	assert.Equal(t, fixture.traceA.ID, resp.Traces[0].Id)
	assert.NotContains(t, body, fixture.traceB.ID.String())
}

func TestAuthConfigRouterHidesLocalModeFromSpoofedPeer(t *testing.T) {
	router := newLocalModeAuthConfigRouter(t)

	rec := serveLocalModeRouterRequest(
		t,
		router,
		"/api/auth/config",
		remoteNonLoopbackAddr,
		map[string]string{"X-Forwarded-For": "127.0.0.1"},
	)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), `"local_mode_enabled":true`)

	var resp AuthConfig
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	if resp.LocalModeEnabled != nil {
		assert.False(t, *resp.LocalModeEnabled)
	}
}

func TestAuthConfigRouterAdvertisesLocalModeToLoopbackPeer(t *testing.T) {
	router := newLocalModeAuthConfigRouter(t)

	rec := serveLocalModeRouterRequest(t, router, "/api/auth/config", "127.0.0.1:54321", nil)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp AuthConfig
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotNil(t, resp.LocalModeEnabled)
	assert.True(t, *resp.LocalModeEnabled)
}

// newLocalModeAuthConfigRouter builds the production router around a server with
// local single-user mode on. /api/auth/config is a public route, so no store is
// needed to reach the handler.
func newLocalModeAuthConfigRouter(t *testing.T) http.Handler {
	t.Helper()

	server := NewServer(nil, nil)
	server.localSingleUserMode = true
	authenticator, err := middleware.NewAuthenticator(nil, &config.Config{
		LocalSingleUserMode: true,
	})
	require.NoError(t, err)

	return NewRouter(server, authenticator)
}

// Local single-user mode is credential-free across the whole composite surface
// on loopback, engine writes included. Execution control is deliberately part of
// that: the mode is an explicit opt-in on a machine its operator owns. Loopback
// admission is therefore the entire security boundary, so these drive the
// production router — where chi's RealIP actually sits — and pin both halves:
// a genuine loopback peer gets through, and a remote peer does not, however its
// headers are dressed up.
func TestLocalModeRouterAdmitsCredentialFreeEngineWritesFromLoopback(t *testing.T) {
	router := newLocalModeEngineRouter(t)

	for _, target := range engineWriteRoutes() {
		t.Run(target, func(t *testing.T) {
			rec := serveEngineWrite(t, router, target, "127.0.0.1:54321", nil)

			// The handler rejects the empty body, which is exactly the point:
			// reaching validation proves auth admitted the request.
			require.NotEqual(
				t,
				http.StatusUnauthorized,
				rec.Code,
				"local mode must admit a credential-free loopback write to %s", target,
			)
			assert.Equal(t, http.StatusBadRequest, rec.Code, "expected to reach handler validation")
		})
	}
}

func TestLocalModeRouterRejectsEngineWritesFromRemotePeer(t *testing.T) {
	router := newLocalModeEngineRouter(t)

	peers := map[string]map[string]string{"no headers": nil}
	for header, value := range spoofedLoopbackHeaders() {
		peers["spoofed "+header] = map[string]string{header: value}
	}

	for _, target := range engineWriteRoutes() {
		for peer, headers := range peers {
			t.Run(target+"/"+peer, func(t *testing.T) {
				rec := serveEngineWrite(t, router, target, remoteNonLoopbackAddr, headers)

				require.Equal(
					t,
					http.StatusUnauthorized,
					rec.Code,
					"a remote peer must not reach %s", target,
				)
				assert.Equal(t, "missing_credentials", decodeJSONBody[Error](t, rec).Code)
			})
		}
	}
}

// TestLocalModeRouterKeepsIngestAPIKeyOnly pins the one route local mode must
// never open: ingest is routeProtectionAPIKeyOnly, so even a loopback caller
// needs a key.
func TestLocalModeRouterKeepsIngestAPIKeyOnly(t *testing.T) {
	router := newLocalModeEngineRouter(t)

	rec := serveEngineWrite(t, router, "/v1/ingest", "127.0.0.1:54321", nil)

	require.Equal(t, http.StatusUnauthorized, rec.Code, "ingest must still require an API key")
	assert.Equal(t, "missing_api_key", decodeJSONBody[Error](t, rec).Code)
}

func TestLocalModeRouterStillAllowsEngineReads(t *testing.T) {
	router := newLocalModeEngineRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/engine/runs", http.NoBody)
	req.RemoteAddr = "127.0.0.1:54321"
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.NotEqual(t, http.StatusUnauthorized, rec.Code, "engine reads must stay open in local mode")
}

// engineWriteRoutes are the composite engine mutations: starting a run and
// triggering a projection backfill.
func engineWriteRoutes() []string {
	return []string{"/v1/engine/runs", "/v1/engine/projections/backfill"}
}

func serveEngineWrite(
	t *testing.T,
	router http.Handler,
	target, remoteAddr string,
	headers map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader("{}"))
	req.RemoteAddr = remoteAddr
	req.Header.Set("Content-Type", "application/json")
	for name, value := range headers {
		req.Header.Set(name, value)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// newLocalModeEngineRouter builds the production router with local single-user
// mode on and the engine public API mounted.
func newLocalModeEngineRouter(t *testing.T) http.Handler {
	t.Helper()

	server := NewServer(nil, nil)
	server.localSingleUserMode = true
	server.enginePublicAPIEnabled = true
	authenticator, err := middleware.NewAuthenticator(nil, &config.Config{
		LocalSingleUserMode: true,
	})
	require.NoError(t, err)

	return NewRouter(server, authenticator)
}
