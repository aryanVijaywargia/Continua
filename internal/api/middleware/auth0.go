package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/auth0/go-jwt-middleware/v3/jwks"
	"github.com/auth0/go-jwt-middleware/v3/validator"

	"github.com/continua-ai/continua/internal/config"
)

const auth0JWKSCacheTTL = 5 * time.Minute

type auth0CustomClaims struct {
	Email string `json:"email,omitempty"`
}

func (c *auth0CustomClaims) Validate(context.Context) error {
	return nil
}

type auth0Authenticator struct {
	validator     *validator.Validator
	validateToken func(context.Context, string) (any, error)
	httpClient    *http.Client
	userInfoURL   string
	allowedEmails map[string]struct{}
	operatorCache sync.Map
}

type cachedOperatorIdentity struct {
	subject   string
	email     string
	expiresAt time.Time
}

// OperatorIdentity captures the Auth0 operator identity resolved for a request.
type OperatorIdentity struct {
	Subject string
	Email   string
}

type operatorAuthError struct {
	Status  int
	Code    string
	Message string
	Err     error
}

func (e *operatorAuthError) Error() string {
	return e.Message
}

func (e *operatorAuthError) Unwrap() error {
	return e.Err
}

func newAuth0Authenticator(cfg config.Auth0Config) (*auth0Authenticator, error) {
	issuerURL, err := url.Parse("https://" + cfg.Domain + "/")
	if err != nil {
		return nil, fmt.Errorf("failed to parse Auth0 issuer URL: %w", err)
	}

	provider, err := jwks.NewCachingProvider(
		jwks.WithIssuerURL(issuerURL),
		jwks.WithCacheTTL(auth0JWKSCacheTTL),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Auth0 JWKS provider: %w", err)
	}

	jwtValidator, err := validator.New(
		validator.WithKeyFunc(provider.KeyFunc),
		validator.WithAlgorithm(validator.RS256),
		validator.WithIssuer(issuerURL.String()),
		validator.WithAudience(cfg.Audience),
		validator.WithCustomClaims(func() *auth0CustomClaims {
			return &auth0CustomClaims{}
		}),
		validator.WithAllowedClockSkew(30*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Auth0 JWT validator: %w", err)
	}

	allowedEmails := make(map[string]struct{}, len(cfg.AllowedEmails))
	for _, email := range cfg.AllowedEmails {
		allowedEmails[strings.ToLower(email)] = struct{}{}
	}

	return &auth0Authenticator{
		validator:     jwtValidator,
		validateToken: jwtValidator.ValidateToken,
		httpClient:    &http.Client{Timeout: 5 * time.Second},
		userInfoURL:   "https://" + cfg.Domain + "/userinfo",
		allowedEmails: allowedEmails,
	}, nil
}

func (a *auth0Authenticator) Authenticate(
	ctx context.Context,
	accessToken string,
) (*OperatorIdentity, *operatorAuthError) {
	validateToken := a.validateToken
	if validateToken == nil && a.validator != nil {
		validateToken = a.validator.ValidateToken
	}
	if validateToken == nil {
		return nil, &operatorAuthError{
			Status:  http.StatusInternalServerError,
			Code:    "internal_error",
			Message: "Operator token validator is not configured",
		}
	}

	validatedToken, err := validateToken(ctx, accessToken)
	if err != nil {
		return nil, &operatorAuthError{
			Status:  http.StatusUnauthorized,
			Code:    "invalid_token",
			Message: "Failed to validate operator token",
			Err:     err,
		}
	}

	claims, ok := validatedToken.(*validator.ValidatedClaims)
	if !ok {
		return nil, &operatorAuthError{
			Status:  http.StatusInternalServerError,
			Code:    "internal_error",
			Message: "Failed to read operator token claims",
		}
	}

	cacheKey := hashKeyMaterial(accessToken)
	if cachedIdentity, ok := a.cachedIdentity(cacheKey); ok {
		return cachedIdentity, nil
	}

	subject := strings.TrimSpace(claims.RegisteredClaims.Subject)
	email := ""
	if customClaims, ok := claims.CustomClaims.(*auth0CustomClaims); ok {
		email = normalizeOperatorEmail(customClaims.Email)
	}
	if email == "" {
		userInfo, authErr := a.fetchUserInfo(ctx, accessToken)
		if authErr != nil {
			return nil, authErr
		}
		if subject != "" && userInfo.Subject != "" && subject != userInfo.Subject {
			return nil, &operatorAuthError{
				Status:  http.StatusUnauthorized,
				Code:    "invalid_token",
				Message: "Operator profile did not match the token subject",
			}
		}
		if subject == "" {
			subject = userInfo.Subject
		}
		email = normalizeOperatorEmail(userInfo.Email)
	}

	if email == "" {
		return nil, &operatorAuthError{
			Status:  http.StatusForbidden,
			Code:    "missing_operator_email",
			Message: "Operator email is required",
		}
	}
	if _, ok := a.allowedEmails[email]; !ok {
		return nil, &operatorAuthError{
			Status:  http.StatusForbidden,
			Code:    "forbidden_operator",
			Message: "Operator email is not allowed",
		}
	}

	identity := &OperatorIdentity{
		Subject: subject,
		Email:   email,
	}

	if claims.RegisteredClaims.Expiry > 0 {
		a.operatorCache.Store(cacheKey, cachedOperatorIdentity{
			subject:   identity.Subject,
			email:     identity.Email,
			expiresAt: time.Unix(claims.RegisteredClaims.Expiry, 0),
		})
	}

	return identity, nil
}

func (a *auth0Authenticator) cachedIdentity(cacheKey string) (*OperatorIdentity, bool) {
	cachedValue, ok := a.operatorCache.Load(cacheKey)
	if !ok {
		return nil, false
	}

	cachedIdentity, ok := cachedValue.(cachedOperatorIdentity)
	if !ok {
		a.operatorCache.Delete(cacheKey)
		return nil, false
	}
	if time.Now().After(cachedIdentity.expiresAt) {
		a.operatorCache.Delete(cacheKey)
		return nil, false
	}

	return &OperatorIdentity{
		Subject: cachedIdentity.subject,
		Email:   cachedIdentity.email,
	}, true
}

type auth0UserInfo struct {
	Subject string `json:"sub"`
	Email   string `json:"email"`
}

func (a *auth0Authenticator) fetchUserInfo(
	ctx context.Context,
	accessToken string,
) (*auth0UserInfo, *operatorAuthError) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, a.userInfoURL, nil)
	if err != nil {
		return nil, &operatorAuthError{
			Status:  http.StatusInternalServerError,
			Code:    "internal_error",
			Message: "Failed to create the Auth0 userinfo request",
			Err:     err,
		}
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)

	response, err := a.httpClient.Do(request)
	if err != nil {
		return nil, &operatorAuthError{
			Status:  http.StatusBadGateway,
			Code:    "userinfo_lookup_failed",
			Message: "Failed to resolve the operator profile",
			Err:     err,
		}
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return nil, &operatorAuthError{
			Status:  http.StatusUnauthorized,
			Code:    "invalid_token",
			Message: "Failed to validate operator token",
		}
	}
	if response.StatusCode != http.StatusOK {
		return nil, &operatorAuthError{
			Status:  http.StatusBadGateway,
			Code:    "userinfo_lookup_failed",
			Message: "Failed to resolve the operator profile",
		}
	}

	var userInfo auth0UserInfo
	if err := json.NewDecoder(response.Body).Decode(&userInfo); err != nil {
		return nil, &operatorAuthError{
			Status:  http.StatusBadGateway,
			Code:    "userinfo_lookup_failed",
			Message: "Failed to decode the operator profile",
			Err:     err,
		}
	}

	return &userInfo, nil
}

func normalizeOperatorEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
