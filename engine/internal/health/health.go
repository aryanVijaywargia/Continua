// Package health provides operational health checks for the engine runtime.
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"sync"
	"time"
)

// Pinger checks whether a dependency is reachable.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Tracker tracks worker-loop iterations for readiness checks.
type Tracker struct {
	mu      sync.RWMutex
	workers map[string]workerState
}

type workerState struct {
	staleAfter    time.Duration
	lastIteration time.Time
}

// NewTracker constructs a worker-loop readiness tracker.
func NewTracker() *Tracker {
	return &Tracker{workers: make(map[string]workerState)}
}

// Register adds a worker and its maximum readiness age.
func (t *Tracker) Register(worker string, staleAfter time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.workers[worker] = workerState{staleAfter: staleAfter}
}

// MarkIteration records a worker iteration at the current time.
func (t *Tracker) MarkIteration(worker string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	state, ok := t.workers[worker]
	if !ok {
		return
	}
	state.lastIteration = time.Now()
	t.workers[worker] = state
}

// Check reports whether every registered worker has iterated recently.
func (t *Tracker) Check(now time.Time) (ready bool, failing []string) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for worker, state := range t.workers {
		if state.lastIteration.IsZero() || now.Sub(state.lastIteration) > state.staleAfter {
			failing = append(failing, worker)
		}
	}
	sort.Strings(failing)
	return len(failing) == 0, failing
}

// LivenessHandler returns the process liveness handler.
func LivenessHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, readinessResponse{Status: "ok"})
	})
}

// ReadinessHandler returns the dependency and worker readiness handler.
func ReadinessHandler(pinger Pinger, tracker *Tracker, pingTimeout time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), pingTimeout)
		defer cancel()

		var failing []string
		if err := pinger.Ping(ctx); err != nil {
			failing = append(failing, "db")
		}
		_, failingWorkers := tracker.Check(time.Now())
		failing = append(failing, failingWorkers...)
		if len(failing) > 0 {
			writeJSON(w, http.StatusServiceUnavailable, readinessResponse{
				Status:  "unavailable",
				Failing: failing,
			})
			return
		}
		writeJSON(w, http.StatusOK, readinessResponse{Status: "ready"})
	})
}

type readinessResponse struct {
	Status  string   `json:"status"`
	Failing []string `json:"failing,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, body readinessResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
