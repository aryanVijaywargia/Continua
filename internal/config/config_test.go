package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/continua-ai/continua/internal/config"
)

func TestLoad_RejectsInvalidTrueAsyncDefault(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("INGEST_TRUE_ASYNC_DEFAULT", "definitely")

	cfg, err := config.Load()
	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "INGEST_TRUE_ASYNC_DEFAULT")
}

func TestLoad_RejectsInvalidWorkerCount(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("RIVER_QUEUE_INGEST_WORKERS", "-1")

	cfg, err := config.Load()
	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "RIVER_QUEUE_INGEST_WORKERS")
}
