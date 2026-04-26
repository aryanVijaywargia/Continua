package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/auth0/go-jwt-middleware/v3/validator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuth0AuthenticateRejectsInvalidToken(t *testing.T) {
	authenticator := &auth0Authenticator{
		validateToken: func(context.Context, string) (any, error) {
			return nil, errors.New("invalid token")
		},
		allowedEmails: map[string]struct{}{},
	}

	identity, authErr := authenticator.Authenticate(context.Background(), "invalid.jwt.token")

	require.Nil(t, identity)
	require.NotNil(t, authErr)
	assert.Equal(t, http.StatusUnauthorized, authErr.Status)
	assert.Equal(t, "invalid_token", authErr.Code)
}

func TestAuth0AuthenticateRejectsMissingEmail(t *testing.T) {
	userInfoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sub":"google-oauth2|operator"}`))
	}))
	t.Cleanup(userInfoServer.Close)

	authenticator := &auth0Authenticator{
		validateToken: func(context.Context, string) (any, error) {
			return validatedAuth0Claims("", time.Now().Add(time.Hour)), nil
		},
		httpClient:    userInfoServer.Client(),
		userInfoURL:   userInfoServer.URL,
		allowedEmails: map[string]struct{}{"operator@example.com": {}},
	}

	identity, authErr := authenticator.Authenticate(context.Background(), "missing.email.token")

	require.Nil(t, identity)
	require.NotNil(t, authErr)
	assert.Equal(t, http.StatusForbidden, authErr.Status)
	assert.Equal(t, "missing_operator_email", authErr.Code)
}

func TestAuth0AuthenticateRejectsNonAllowlistedEmail(t *testing.T) {
	authenticator := &auth0Authenticator{
		validateToken: func(context.Context, string) (any, error) {
			return validatedAuth0Claims("outside@example.com", time.Now().Add(time.Hour)), nil
		},
		allowedEmails: map[string]struct{}{"operator@example.com": {}},
	}

	identity, authErr := authenticator.Authenticate(context.Background(), "outside.email.token")

	require.Nil(t, identity)
	require.NotNil(t, authErr)
	assert.Equal(t, http.StatusForbidden, authErr.Status)
	assert.Equal(t, "forbidden_operator", authErr.Code)
}

func TestAuth0AuthenticateAcceptsAllowlistedEmail(t *testing.T) {
	authenticator := &auth0Authenticator{
		validateToken: func(context.Context, string) (any, error) {
			return validatedAuth0Claims("Operator@Example.com", time.Now().Add(time.Hour)), nil
		},
		allowedEmails: map[string]struct{}{"operator@example.com": {}},
	}

	identity, authErr := authenticator.Authenticate(context.Background(), "allowed.email.token")

	require.Nil(t, authErr)
	require.NotNil(t, identity)
	assert.Equal(t, "google-oauth2|operator", identity.Subject)
	assert.Equal(t, "operator@example.com", identity.Email)
}

func TestAuth0AuthenticateCachesResolvedUserInfoEmail(t *testing.T) {
	userInfoCalls := 0
	userInfoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		userInfoCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sub":"google-oauth2|operator","email":"operator@example.com"}`))
	}))
	t.Cleanup(userInfoServer.Close)

	validateCalls := 0
	authenticator := &auth0Authenticator{
		validateToken: func(context.Context, string) (any, error) {
			validateCalls++
			return validatedAuth0Claims("", time.Now().Add(time.Hour)), nil
		},
		httpClient:    userInfoServer.Client(),
		userInfoURL:   userInfoServer.URL,
		allowedEmails: map[string]struct{}{"operator@example.com": {}},
	}

	firstIdentity, firstErr := authenticator.Authenticate(context.Background(), "userinfo.email.token")
	secondIdentity, secondErr := authenticator.Authenticate(context.Background(), "userinfo.email.token")

	require.Nil(t, firstErr)
	require.Nil(t, secondErr)
	assert.Equal(t, firstIdentity, secondIdentity)
	assert.Equal(t, 2, validateCalls)
	assert.Equal(t, 1, userInfoCalls)
}

func validatedAuth0Claims(email string, expiry time.Time) *validator.ValidatedClaims {
	return &validator.ValidatedClaims{
		CustomClaims: &auth0CustomClaims{
			Email: email,
		},
		RegisteredClaims: validator.RegisteredClaims{
			Subject: "google-oauth2|operator",
			Expiry:  expiry.Unix(),
		},
	}
}
