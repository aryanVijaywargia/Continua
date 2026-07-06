package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/continua-ai/continua/internal/api/middleware"
	"github.com/continua-ai/continua/internal/store"
)

func TestNormalizePagination_Defaults(t *testing.T) {
	limit, offset := normalizePagination(nil, nil)

	if limit != defaultPageLimit {
		t.Fatalf("expected default limit %d, got %d", defaultPageLimit, limit)
	}
	if offset != 0 {
		t.Fatalf("expected default offset 0, got %d", offset)
	}
}

func TestNormalizePagination_ClampsBounds(t *testing.T) {
	tooLarge := 1_000_000
	negative := -42

	limit, offset := normalizePagination(&tooLarge, &negative)

	if limit != maxPageLimit {
		t.Fatalf("expected capped limit %d, got %d", maxPageLimit, limit)
	}
	if offset != 0 {
		t.Fatalf("expected clamped offset 0, got %d", offset)
	}
}

func TestNormalizePagination_ClampsNonPositiveLimit(t *testing.T) {
	zero := 0
	neg := -1

	limitZero, _ := normalizePagination(&zero, nil)
	if limitZero != 1 {
		t.Fatalf("expected limit 1 for zero input, got %d", limitZero)
	}

	limitNeg, _ := normalizePagination(&neg, nil)
	if limitNeg != 1 {
		t.Fatalf("expected limit 1 for negative input, got %d", limitNeg)
	}
}

func TestTraceFilterFromParams_EngineOnlyUsesDynamicQuery(t *testing.T) {
	engineOnly := true

	filter := traceFilterFromParams(store.BoundScope(uuid.New()), &ListTracesParams{
		EngineOnly: &engineOnly,
	}, 50, 0)

	if !filter.EngineOnly {
		t.Fatal("expected engine_only to map into the store filter")
	}
	if !traceNeedsDynamicQuery(&filter) {
		t.Fatal("expected engine_only=true to use the dynamic trace query")
	}
}

func TestTraceFilterFromParams_EngineOnlyFalseIsDefaultPath(t *testing.T) {
	engineOnly := false

	filter := traceFilterFromParams(store.BoundScope(uuid.New()), &ListTracesParams{
		EngineOnly: &engineOnly,
	}, 50, 0)

	if filter.EngineOnly {
		t.Fatal("expected engine_only=false to leave the store filter disabled")
	}
	if traceNeedsDynamicQuery(&filter) {
		t.Fatal("expected engine_only=false alone to preserve the default trace query path")
	}
}

func TestScopeFromRequest_APIKeyResolvesBoundScope(t *testing.T) {
	projectID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/api/traces", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.ProjectIDKey, projectID))
	rec := httptest.NewRecorder()

	scope, ok := scopeFromRequest(rec, req, scopePolicyAllowUnbounded)
	if !ok {
		t.Fatal("expected scope resolution to succeed")
	}

	gotProjectID, bound := scope.ProjectID()
	if !bound {
		t.Fatal("expected API key request to resolve to a bound scope")
	}
	if gotProjectID != projectID {
		t.Fatalf("expected project %s, got %s", projectID, gotProjectID)
	}
}

func TestScopeFromRequest_PublicDemoResolvesBoundScope(t *testing.T) {
	projectID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/api/traces?project_id="+uuid.New().String(), nil)
	reqCtx := context.WithValue(req.Context(), middleware.ProjectIDKey, projectID)
	reqCtx = context.WithValue(reqCtx, middleware.AuthModeKey, middleware.AuthModePublicDemo)
	rec := httptest.NewRecorder()

	scope, ok := scopeFromRequest(rec, req.WithContext(reqCtx), scopePolicyRequireProject)
	if !ok {
		t.Fatal("expected scope resolution to succeed")
	}

	gotProjectID, bound := scope.ProjectID()
	if !bound {
		t.Fatal("expected public demo request to resolve to a bound scope")
	}
	if gotProjectID != projectID {
		t.Fatalf("expected project %s, got %s", projectID, gotProjectID)
	}
}

func TestScopeFromRequest_OperatorListRequiresProjectID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/traces", nil)
	reqCtx := context.WithValue(req.Context(), middleware.AuthModeKey, middleware.AuthModeOperator)
	rec := httptest.NewRecorder()

	_, ok := scopeFromRequest(rec, req.WithContext(reqCtx), scopePolicyRequireProject)
	if ok {
		t.Fatal("expected scope resolution to fail without project_id")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestScopeFromRequest_OperatorListResolvesSelectedProject(t *testing.T) {
	projectID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/api/traces?project_id="+projectID.String(), nil)
	reqCtx := context.WithValue(req.Context(), middleware.AuthModeKey, middleware.AuthModeOperator)
	rec := httptest.NewRecorder()

	scope, ok := scopeFromRequest(rec, req.WithContext(reqCtx), scopePolicyRequireProject)
	if !ok {
		t.Fatal("expected scope resolution to succeed")
	}

	gotProjectID, bound := scope.ProjectID()
	if !bound {
		t.Fatal("expected operator list request to resolve to a bound scope")
	}
	if gotProjectID != projectID {
		t.Fatalf("expected project %s, got %s", projectID, gotProjectID)
	}
}

func TestScopeFromRequest_OperatorDetailResolvesUnbounded(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/traces/"+uuid.New().String(), nil)
	reqCtx := context.WithValue(req.Context(), middleware.AuthModeKey, middleware.AuthModeOperator)
	rec := httptest.NewRecorder()

	scope, ok := scopeFromRequest(rec, req.WithContext(reqCtx), scopePolicyAllowUnbounded)
	if !ok {
		t.Fatal("expected scope resolution to succeed")
	}

	if _, bound := scope.ProjectID(); bound {
		t.Fatal("expected operator detail request to resolve to unbounded scope")
	}
}

func TestScopeFromRequest_OperatorDetailResolvesSelectedProject(t *testing.T) {
	projectID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/api/traces/"+uuid.New().String()+"?project_id="+projectID.String(), nil)
	reqCtx := context.WithValue(req.Context(), middleware.AuthModeKey, middleware.AuthModeOperator)
	rec := httptest.NewRecorder()

	scope, ok := scopeFromRequest(rec, req.WithContext(reqCtx), scopePolicyAllowUnbounded)
	if !ok {
		t.Fatal("expected scope resolution to succeed")
	}

	gotProjectID, bound := scope.ProjectID()
	if !bound {
		t.Fatal("expected operator detail project_id to resolve to a bound scope")
	}
	if gotProjectID != projectID {
		t.Fatalf("expected project %s, got %s", projectID, gotProjectID)
	}
}

func TestScopeFromRequest_OperatorDetailRejectsInvalidProjectID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/traces/"+uuid.New().String()+"?project_id=not-a-uuid", nil)
	reqCtx := context.WithValue(req.Context(), middleware.AuthModeKey, middleware.AuthModeOperator)
	rec := httptest.NewRecorder()

	_, ok := scopeFromRequest(rec, req.WithContext(reqCtx), scopePolicyAllowUnbounded)
	if ok {
		t.Fatal("expected scope resolution to fail for invalid project_id")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}
