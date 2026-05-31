package store

import (
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Scope is the project-read scope resolved for one request.
//
// Bound scopes may read exactly one project. Unbounded scopes are operator/admin
// only; on operator routes, project_id is a selection filter, not an
// authorization boundary.
type Scope struct {
	projectID uuid.UUID
	bound     bool
}

// BoundScope returns a scope limited to one project.
func BoundScope(projectID uuid.UUID) Scope {
	return Scope{projectID: projectID, bound: true}
}

// UnboundedScope returns an operator/admin-only scope with no project filter.
func UnboundedScope() Scope {
	return Scope{}
}

// ProjectID returns the bound project id and whether this scope is bound.
func (s Scope) ProjectID() (uuid.UUID, bool) {
	return s.projectID, s.bound
}

func (s Scope) nullableProjectFilter() pgtype.UUID {
	if !s.bound {
		return pgtype.UUID{Valid: false}
	}
	return pgtype.UUID{Bytes: s.projectID, Valid: true}
}
