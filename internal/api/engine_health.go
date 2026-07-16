package api

import (
	"net/http"
	"time"

	"github.com/continua-ai/continua/internal/store"
)

func (s *Server) GetEngineHealth(w http.ResponseWriter, r *http.Request) {
	scope, ok := scopeFromRequest(w, r, scopePolicyAllowUnbounded)
	if !ok {
		return
	}

	snapshot, err := s.store.GetEngineHealth(r.Context(), scope)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get engine health")
		return
	}

	writeJSON(w, http.StatusOK, engineHealthToAPI(&snapshot))
}

func engineHealthToAPI(snapshot *store.EngineHealthSnapshot) EngineHealthResponse {
	workers := make([]EngineWorkerHealth, len(snapshot.Workers))
	for i, worker := range snapshot.Workers {
		workers[i] = EngineWorkerHealth{
			ActiveLeases:  worker.ActiveLeases,
			ExpiredLeases: worker.ExpiredLeases,
			Id:            worker.ID,
			LastClaimAt:   worker.LastClaimAt,
			Status:        EngineWorkerHealthStatus(worker.Status),
		}
	}

	response := EngineHealthResponse{
		GeneratedAt: time.Now().UTC(),
		Workers:     workers,
	}
	response.Projector.LagRows = snapshot.ProjectorLagRows
	response.Projector.RunsCatchingUp = snapshot.RunsCatchingUp
	response.Queues.RunsReady = snapshot.RunsReady
	response.Queues.ActivityTasksPending = snapshot.ActivityTasksPending
	response.Queues.InboxPending = snapshot.InboxPending
	response.Retention.SummaryOnlyRuns = snapshot.SummaryOnlyRuns
	response.Retention.JournalExpiredRuns = snapshot.JournalExpiredRuns
	return response
}
