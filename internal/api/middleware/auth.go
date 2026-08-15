package middleware

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/continua-ai/continua/internal/config"
	"github.com/continua-ai/continua/internal/store"
)

// contextKey is a custom type for context keys to avoid collisions.
type contextKey string

// Context keys used by auth middleware.
const (
	ProjectIDKey       contextKey = "project_id"
	AuthModeKey        contextKey = "auth_mode"
	OperatorEmailKey   contextKey = "operator_email"
	OperatorSubjectKey contextKey = "operator_subject"
	// TransportPeerKey carries the connection's true peer address, captured
	// before any middleware that rewrites RemoteAddr from proxy headers.
	TransportPeerKey contextKey = "transport_peer"
)

// CaptureTransportPeer records the real transport peer address so later
// middleware can make trust decisions about it. It MUST be mounted before
// chi's RealIP middleware, which overwrites RemoteAddr with the value of the
// attacker-controlled X-Forwarded-For / X-Real-IP headers.
func CaptureTransportPeer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), TransportPeerKey, r.RemoteAddr)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// AuthMode identifies how a request was authenticated.
type AuthMode string

const (
	AuthModeAPIKey     AuthMode = "api_key"
	AuthModeOperator   AuthMode = "operator"
	AuthModePublicDemo AuthMode = "public_demo"
	AuthModeBootstrap  AuthMode = "bootstrap"
	// AuthModeLocalSingleUser marks a credential-free loopback request accepted
	// because the deployment opted into local single-user mode. Requests in this
	// mode carry no bound project; the project_id query parameter selects scope.
	AuthModeLocalSingleUser AuthMode = "local_single_user"
)

type routeProtection int

const (
	routeProtectionPublic routeProtection = iota
	routeProtectionAPIKeyOnly
	routeProtectionComposite
)

// Authenticator validates API keys and Auth0 bearer tokens for incoming routes.
type Authenticator struct {
	store      *store.Store
	auth0      *auth0Authenticator
	publicDemo *publicDemoAccess
	// localSingleUserMode mirrors config.Config.LocalSingleUserMode.
	localSingleUserMode bool
}

type publicDemoAccess struct {
	projectID uuid.UUID
}

// NewAuthenticator creates the route-aware authenticator for API handlers.
func NewAuthenticator(s *store.Store, cfg *config.Config) (*Authenticator, error) {
	authenticator := &Authenticator{
		store: s,
	}
	if cfg != nil && cfg.Auth0.Enabled {
		auth0Authenticator, err := newAuth0Authenticator(&cfg.Auth0)
		if err != nil {
			return nil, err
		}
		authenticator.auth0 = auth0Authenticator
	}
	if cfg != nil && cfg.PublicDemo.Enabled {
		authenticator.publicDemo = &publicDemoAccess{
			projectID: cfg.PublicDemo.ProjectID,
		}
	}
	if cfg != nil {
		authenticator.localSingleUserMode = cfg.LocalSingleUserMode
	}
	return authenticator, nil
}

// APIKeyAuth preserves the legacy API-key-only middleware for handlers and tests
// that only need project-scoped authentication.
func APIKeyAuth(s *store.Store) func(http.Handler) http.Handler {
	authenticator := &Authenticator{store: s}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authenticator.serveAPIKeyOnly(next, w, r)
		})
	}
}

// Middleware enforces public, API-key-only, or composite auth behavior by route.
func (a *Authenticator) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch classifyRouteProtection(r.URL.Path) {
			case routeProtectionPublic:
				next.ServeHTTP(w, r)
			case routeProtectionAPIKeyOnly:
				a.serveAPIKeyOnly(next, w, r)
			case routeProtectionComposite:
				if a.publicDemo != nil && isPublicDemoAllowedRequest(r.Method, r.URL.Path) {
					a.servePublicDemoRead(next, w, r)
					return
				}
				a.serveComposite(next, w, r)
			default:
				a.serveAPIKeyOnly(next, w, r)
			}
		})
	}
}

// GetProjectID extracts the project ID from the request context.
// Returns the project ID and true if present, or uuid.Nil and false if not.
func GetProjectID(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(ProjectIDKey).(uuid.UUID)
	return id, ok
}

// GetAuthMode extracts the authentication mode from the request context.
func GetAuthMode(ctx context.Context) (AuthMode, bool) {
	mode, ok := ctx.Value(AuthModeKey).(AuthMode)
	if ok {
		return mode, true
	}
	if _, ok := GetProjectID(ctx); ok {
		return AuthModeAPIKey, true
	}
	return "", false
}

// GetOperatorEmail extracts the authenticated operator email from the request context.
func GetOperatorEmail(ctx context.Context) (string, bool) {
	email, ok := ctx.Value(OperatorEmailKey).(string)
	return email, ok
}

// GetOperatorSubject extracts the authenticated operator subject from the request context.
func GetOperatorSubject(ctx context.Context) (string, bool) {
	subject, ok := ctx.Value(OperatorSubjectKey).(string)
	return subject, ok
}

func (a *Authenticator) serveAPIKeyOnly(next http.Handler, w http.ResponseWriter, r *http.Request) {
	apiKey := extractAPIKey(r)
	if apiKey == "" {
		writeAuthError(w, http.StatusUnauthorized, "missing_api_key", "API key required")
		return
	}

	ctx, ok := a.apiKeyContext(r.Context(), apiKey, w)
	if !ok {
		return
	}

	next.ServeHTTP(w, r.WithContext(ctx))
}

func (a *Authenticator) serveComposite(next http.Handler, w http.ResponseWriter, r *http.Request) {
	if apiKey := strings.TrimSpace(r.Header.Get("X-API-Key")); apiKey != "" {
		ctx, ok := a.apiKeyContext(r.Context(), apiKey, w)
		if !ok {
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
		return
	}

	bearerToken := extractBearerToken(r)
	if bearerToken == "" {
		if a.serveProjectBootstrap(next, w, r) {
			return
		}
		if a.serveLocalSingleUser(next, w, r) {
			return
		}
		writeAuthError(w, http.StatusUnauthorized, "missing_credentials", "Authentication required")
		return
	}

	if !looksLikeJWT(bearerToken) {
		ctx, ok := a.apiKeyContext(r.Context(), bearerToken, w)
		if !ok {
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
		return
	}

	if a.auth0 == nil {
		writeAuthError(
			w,
			http.StatusServiceUnavailable,
			"operator_auth_unavailable",
			"Operator authentication is not configured",
		)
		return
	}

	identity, authErr := a.auth0.Authenticate(r.Context(), bearerToken)
	if authErr != nil {
		writeAuthError(w, authErr.Status, authErr.Code, authErr.Message)
		return
	}

	ctx := context.WithValue(r.Context(), AuthModeKey, AuthModeOperator)
	ctx = context.WithValue(ctx, OperatorEmailKey, identity.Email)
	ctx = context.WithValue(ctx, OperatorSubjectKey, identity.Subject)
	next.ServeHTTP(w, r.WithContext(ctx))
}

func (a *Authenticator) servePublicDemoRead(next http.Handler, w http.ResponseWriter, r *http.Request) {
	if a.publicDemo == nil {
		a.serveComposite(next, w, r)
		return
	}

	ctx := context.WithValue(r.Context(), ProjectIDKey, a.publicDemo.projectID)
	ctx = context.WithValue(ctx, AuthModeKey, AuthModePublicDemo)
	next.ServeHTTP(w, r.WithContext(ctx))
}

func (a *Authenticator) serveProjectBootstrap(next http.Handler, w http.ResponseWriter, r *http.Request) bool {
	// Local-mode bootstrap: when Auth0 and the public demo are both disabled, the
	// deployment is single-tenant and the operator owns the box. We let
	// unauthenticated callers list and create projects on /api/projects so a fresh
	// install (or an operator who has lost their API key) can always self-recover
	// without wiping the database. Deployments that need cross-tenant isolation
	// must enable Auth0, which closes this path entirely.
	if a.auth0 != nil || a.publicDemo != nil || !isProjectBootstrapRoute(r.Method, r.URL.Path) {
		return false
	}

	ctx := context.WithValue(r.Context(), AuthModeKey, AuthModeBootstrap)
	next.ServeHTTP(w, r.WithContext(ctx))
	return true
}

// serveLocalSingleUser admits a credential-free request when the deployment opted
// into local single-user mode and the transport peer is the local machine. No
// project is bound: the project_id query parameter selects the read scope, exactly
// as it does for an operator. Returns true when it handled the request.
//
// Reads, the project bootstrap and project management all run credential-free
// here: the mode is an explicit opt-in on a machine its operator owns. Engine
// writes are the one exception. They are execution control rather than
// single-user console convenience, so POST /v1/engine/runs and
// POST /v1/engine/projections/backfill keep requiring an API key.
func (a *Authenticator) serveLocalSingleUser(next http.Handler, w http.ResponseWriter, r *http.Request) bool {
	if !a.localSingleUserMode || isEngineWriteRoute(r.Method, r.URL.Path) || !IsLoopbackRequest(r) {
		return false
	}

	ctx := context.WithValue(r.Context(), AuthModeKey, AuthModeLocalSingleUser)
	next.ServeHTTP(w, r.WithContext(ctx))
	return true
}

// IsLoopbackRequest reports whether the request's transport peer is the local
// machine. The peer address comes from the connection, never from proxy headers
// such as X-Forwarded-For, which are attacker-controlled and must never
// influence an authentication decision. CaptureTransportPeer stashes the real
// peer before chi's RealIP middleware overwrites RemoteAddr with those headers;
// without that middleware in the stack, RemoteAddr is still the raw peer.
func IsLoopbackRequest(r *http.Request) bool {
	remoteAddr, ok := r.Context().Value(TransportPeerKey).(string)
	if !ok {
		remoteAddr = r.RemoteAddr
	}

	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		// The peer address is not host:port (e.g. a Unix socket peer); treat it
		// whole. It must stay the captured peer, never r.RemoteAddr, which RealIP
		// may already have replaced with a header value.
		host = remoteAddr
	}

	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

func (a *Authenticator) apiKeyContext(
	ctx context.Context,
	apiKey string,
	w http.ResponseWriter,
) (context.Context, bool) {
	project, err := a.store.GetProjectByAPIKey(ctx, hashAPIKey(apiKey))
	if err != nil {
		if store.IsNotFound(err) {
			writeAuthError(w, http.StatusUnauthorized, "invalid_api_key", "Invalid API key")
			return nil, false
		}
		writeAuthError(w, http.StatusInternalServerError, "internal_error", "Failed to validate API key")
		return nil, false
	}

	ctx = context.WithValue(ctx, ProjectIDKey, project.ID)
	ctx = context.WithValue(ctx, AuthModeKey, AuthModeAPIKey)
	return ctx, true
}

func classifyRouteProtection(path string) routeProtection {
	switch {
	case path == "/api/auth/config":
		return routeProtectionPublic
	case strings.HasPrefix(path, "/v1/ingest"):
		return routeProtectionAPIKeyOnly
	case isEngineAPIKeyOnlyRoute(path):
		return routeProtectionAPIKeyOnly
	case isEngineConsoleRoute(path):
		return routeProtectionComposite
	case strings.HasPrefix(path, "/api/"), isEngineRunScopedRoute(path):
		return routeProtectionComposite
	default:
		return routeProtectionAPIKeyOnly
	}
}

func isEngineAPIKeyOnlyRoute(path string) bool {
	switch {
	case path == "/v1/engine/activities/claim":
		return true
	case strings.HasPrefix(path, "/v1/engine/activities/") &&
		(strings.HasSuffix(path, "/heartbeat") ||
			strings.HasSuffix(path, "/complete") ||
			strings.HasSuffix(path, "/fail")):
		return true
	default:
		return false
	}
}

func isEngineConsoleRoute(path string) bool {
	return path == "/v1/engine/runs" ||
		strings.HasPrefix(path, "/v1/engine/instances/") ||
		path == "/v1/engine/projections/backfill"
}

func isEngineRunScopedRoute(path string) bool {
	if !strings.HasPrefix(path, "/v1/engine/runs/") {
		return false
	}

	return strings.TrimPrefix(path, "/v1/engine/runs/") != ""
}

func isPublicDemoReadRequest(method, path string) bool {
	if method != http.MethodGet {
		return false
	}

	switch {
	case path == "/api/traces":
		return true
	case matchesPathPattern(path, "/api/traces/{id}"):
		return true
	case matchesPathPattern(path, "/api/traces/{id}/spans"):
		return true
	case matchesPathPattern(path, "/api/traces/{id}/events"):
		return true
	case path == "/api/sessions":
		return true
	case matchesPathPattern(path, "/api/sessions/{id}"):
		return true
	case matchesPathPattern(path, "/api/sessions/{id}/narrative"):
		return true
	case matchesPathPattern(path, "/api/sessions/{id}/compare"):
		return true
	case matchesPathPattern(path, "/v1/engine/instances/{instance_key}"):
		return true
	case matchesPathPattern(path, "/v1/engine/runs/{run_id}"):
		return true
	case matchesPathPattern(path, "/v1/engine/runs/{run_id}/pending-work"):
		return true
	case matchesPathPattern(path, "/v1/engine/runs/{run_id}/history"):
		return true
	case matchesPathPattern(path, "/v1/engine/runs/{run_id}/result"):
		return true
	default:
		return false
	}
}

func isPublicDemoAllowedRequest(method, path string) bool {
	if isPublicDemoReadRequest(method, path) {
		return true
	}
	return method == http.MethodPost && path == "/v1/engine/projections/backfill"
}

func isProjectBootstrapRoute(method, path string) bool {
	return path == "/api/projects" && (method == http.MethodGet || method == http.MethodPost)
}

// isEngineWriteRoute reports whether the request drives engine execution rather
// than reading it. The composite route class is wider than the console's read
// surface — isEngineConsoleRoute also covers POST /v1/engine/runs and
// POST /v1/engine/projections/backfill — so these are named explicitly and kept
// API-key-only even in local single-user mode.
func isEngineWriteRoute(method, path string) bool {
	if isSafeMethod(method) || !strings.HasPrefix(path, "/v1/engine") {
		return false
	}

	return true
}

// isSafeMethod reports whether the method is read-only per RFC 9110 and so
// cannot change server state.
func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func matchesPathPattern(path, pattern string) bool {
	pathParts := strings.Split(strings.Trim(path, "/"), "/")
	patternParts := strings.Split(strings.Trim(pattern, "/"), "/")
	if len(pathParts) != len(patternParts) {
		return false
	}

	for i, patternPart := range patternParts {
		if strings.HasPrefix(patternPart, "{") && strings.HasSuffix(patternPart, "}") {
			if pathParts[i] == "" {
				return false
			}
			continue
		}
		if pathParts[i] != patternPart {
			return false
		}
	}
	return true
}

// extractAPIKey extracts the API key from the request headers.
// Checks X-API-Key header first, then Authorization: Bearer.
func extractAPIKey(r *http.Request) string {
	if key := strings.TrimSpace(r.Header.Get("X-API-Key")); key != "" {
		return key
	}
	return extractBearerToken(r)
}

func extractBearerToken(r *http.Request) string {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	return ""
}

func looksLikeJWT(token string) bool {
	return strings.Count(token, ".") == 2
}

// hashAPIKey hashes an API key using SHA-256.
func hashAPIKey(apiKey string) string {
	return hashKeyMaterial(apiKey)
}

// HashAPIKey is the exported form used by handlers that create or rotate keys.
func HashAPIKey(apiKey string) string {
	return hashAPIKey(apiKey)
}

func hashKeyMaterial(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}

// APIKeyPrefix prefixes generated keys so they're recognizable on sight.
const APIKeyPrefix = "pk_"

// apiKeyRandomBytes is the entropy size for generated keys (32 bytes = 256 bits).
const apiKeyRandomBytes = 32

// GenerateAPIKey returns a fresh API key with the form `pk_<base32>`.
// The plaintext key is returned only once to the caller; the server stores
// just its SHA-256 hash via HashAPIKey.
func GenerateAPIKey() (string, error) {
	buf := make([]byte, apiKeyRandomBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	encoded := strings.ToLower(strings.TrimRight(base32.StdEncoding.EncodeToString(buf), "="))
	return APIKeyPrefix + encoded, nil
}

// writeAuthError writes a JSON error response for authentication failures.
func writeAuthError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"code":    code,
		"message": message,
	})
}
