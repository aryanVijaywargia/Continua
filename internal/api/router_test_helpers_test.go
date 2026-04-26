package api

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/continua-ai/continua/internal/api/middleware"
	"github.com/continua-ai/continua/internal/store"
)

func newAuthenticatedRouter(t *testing.T, server *Server, platformStore *store.Store) http.Handler {
	t.Helper()

	authenticator, err := middleware.NewAuthenticator(platformStore, nil)
	require.NoError(t, err)

	return NewRouter(server, authenticator)
}
