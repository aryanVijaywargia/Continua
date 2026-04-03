package store

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
)

func TestTransactionCommitAndRollback(t *testing.T) {
	ts := newTestStore(t)
	projectID := uuidOrFatal(t)

	tx, err := ts.store.BeginTx(ts.ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}

	rolledBackKey := "instance-rollback"
	if _, err := tx.CreateInstance(ts.ctx, enginedb.CreateInstanceParams{
		ProjectID:      projectID,
		InstanceKey:    rolledBackKey,
		DefinitionName: "workflow.demo",
		Metadata:       []byte(`{"state":"rollback"}`),
	}); err != nil {
		t.Fatalf("tx.CreateInstance() error = %v", err)
	}
	if err := tx.Rollback(ts.ctx); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}

	_, err = ts.store.GetInstanceByProjectAndKey(ts.ctx, enginedb.GetInstanceByProjectAndKeyParams{
		ProjectID:   projectID,
		InstanceKey: rolledBackKey,
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected rollback to hide instance, got %v", err)
	}

	tx, err = ts.store.BeginTx(ts.ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}

	committedKey := "instance-commit"
	if _, err := tx.CreateInstance(ts.ctx, enginedb.CreateInstanceParams{
		ProjectID:      projectID,
		InstanceKey:    committedKey,
		DefinitionName: "workflow.demo",
		Metadata:       []byte(`{"state":"commit"}`),
	}); err != nil {
		t.Fatalf("tx.CreateInstance() error = %v", err)
	}
	if err := tx.Commit(ts.ctx); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	instance, err := ts.store.GetInstanceByProjectAndKey(ts.ctx, enginedb.GetInstanceByProjectAndKeyParams{
		ProjectID:   projectID,
		InstanceKey: committedKey,
	})
	if err != nil {
		t.Fatalf("GetInstanceByProjectAndKey() error = %v", err)
	}
	if instance.InstanceKey != committedKey {
		t.Fatalf("expected committed instance key %q, got %q", committedKey, instance.InstanceKey)
	}
}
