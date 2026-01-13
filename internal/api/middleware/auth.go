package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/continua-ai/continua/internal/store"
)

// contextKey is a custom type for context keys to avoid collisions.
type contextKey string

// ProjectIDKey is the context key for the project ID.
const ProjectIDKey contextKey = "project_id"

// APIKeyAuth creates middleware that validates API keys and injects project ID into context.
// It extracts the API key from either:
// - X-API-Key header
// - Authorization: Bearer <key> header
func APIKeyAuth(s *store.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey := extractAPIKey(r)
			if apiKey == "" {
				writeAuthError(w, http.StatusUnauthorized, "missing_api_key", "API key required")
				return
			}

			// Hash the API key for lookup
			keyHash := hashAPIKey(apiKey)

			// Look up project by API key hash
			project, err := s.GetProjectByAPIKey(r.Context(), keyHash)
			if err != nil {
				if store.IsNotFound(err) {
					writeAuthError(w, http.StatusUnauthorized, "invalid_api_key", "Invalid API key")
					return
				}
				writeAuthError(w, http.StatusInternalServerError, "internal_error", "Failed to validate API key")
				return
			}

			// Inject project ID into context
			ctx := context.WithValue(r.Context(), ProjectIDKey, project.ID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetProjectID extracts the project ID from the request context.
// Returns the project ID and true if present, or uuid.Nil and false if not.
func GetProjectID(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(ProjectIDKey).(uuid.UUID)
	return id, ok
}

// extractAPIKey extracts the API key from the request headers.
// Checks X-API-Key header first, then Authorization: Bearer.
func extractAPIKey(r *http.Request) string {
	// Check X-API-Key header first
	if key := r.Header.Get("X-API-Key"); key != "" {
		return key
	}

	// Check Authorization: Bearer header
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}

	return ""
}

// hashAPIKey hashes an API key using SHA-256.
func hashAPIKey(apiKey string) string {
	hash := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(hash[:])
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
