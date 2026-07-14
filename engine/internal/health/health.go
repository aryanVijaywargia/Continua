// Package health provides operational health checks for the engine runtime.
package health

import (
	"context"
	"net/http"
	"time"
)

// Pinger checks whether a dependency is reachable.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Tracker tracks worker-loop iterations for readiness checks.
type Tracker struct{}

// NewTracker constructs a worker-loop readiness tracker.
func NewTracker() *Tracker {
	return &Tracker{}
}

// Register adds a worker and its maximum readiness age.
func (t *Tracker) Register(worker string, staleAfter time.Duration) {}

// MarkIteration records a worker iteration at the current time.
func (t *Tracker) MarkIteration(worker string) {}

// Check reports whether every registered worker has iterated recently.
func (t *Tracker) Check(now time.Time) (ready bool, failing []string) {
	return false, []string{"health tracker not implemented"}
}

// LivenessHandler returns the process liveness handler.
func LivenessHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
}

// ReadinessHandler returns the dependency and worker readiness handler.
func ReadinessHandler(pinger Pinger, tracker *Tracker, pingTimeout time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
}
