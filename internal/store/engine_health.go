package store

import (
	"context"
	"fmt"
	"time"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
)

type EngineHealthSnapshot struct {
	ProjectorLagRows     int64
	RunsCatchingUp       int64
	RunsReady            int64
	ActivityTasksPending int64
	InboxPending         int64
	Workers              []EngineWorkerHealth
	SummaryOnlyRuns      int64
	JournalExpiredRuns   int64
}

type EngineWorkerHealth struct {
	ID            string
	LastClaimAt   time.Time
	ActiveLeases  int
	ExpiredLeases int
	Status        string
}

func (s *Store) GetEngineHealth(ctx context.Context, scope Scope) (EngineHealthSnapshot, error) {
	projectFilter := scope.nullableProjectFilter()
	queries := enginedb.New(s.pool)

	metrics, err := queries.GetEngineHealthMetrics(ctx, projectFilter)
	if err != nil {
		return EngineHealthSnapshot{}, fmt.Errorf("get engine health metrics: %w", err)
	}

	workerRows, err := queries.ListEngineWorkerHealth(ctx, projectFilter)
	if err != nil {
		return EngineHealthSnapshot{}, fmt.Errorf("list engine worker health: %w", err)
	}

	workers := make([]EngineWorkerHealth, 0, len(workerRows))
	for _, row := range workerRows {
		workers = append(workers, EngineWorkerHealth{
			ID:            row.ID,
			LastClaimAt:   row.LastClaimAt,
			ActiveLeases:  int(row.ActiveLeases),
			ExpiredLeases: int(row.ExpiredLeases),
			Status:        row.Status,
		})
	}

	return EngineHealthSnapshot{
		ProjectorLagRows:     metrics.ProjectorLagRows,
		RunsCatchingUp:       metrics.RunsCatchingUp,
		RunsReady:            metrics.RunsReady,
		ActivityTasksPending: metrics.ActivityTasksPending,
		InboxPending:         metrics.InboxPending,
		Workers:              workers,
		SummaryOnlyRuns:      metrics.SummaryOnlyRuns,
		JournalExpiredRuns:   metrics.JournalExpiredRuns,
	}, nil
}
