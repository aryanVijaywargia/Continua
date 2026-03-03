package api

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/continua-ai/continua/db/gen/go/platform"
)

func TestSessionToAPI_IncludesExternalID(t *testing.T) {
	now := time.Now().UTC()
	name := "checkout session"
	userID := "user-123"

	session := platform.Session{
		ID:         uuid.New(),
		ProjectID:  uuid.New(),
		ExternalID: "checkout-flow-42",
		Name:       &name,
		UserID:     &userID,
		Metadata:   []byte(`{"key":"value"}`),
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	apiSession := sessionToAPI(&session)

	assert.Equal(t, session.ID, apiSession.Id)
	assert.Equal(t, session.ExternalID, apiSession.ExternalId)
	require.NotNil(t, apiSession.Name)
	assert.Equal(t, name, *apiSession.Name)
	require.NotNil(t, apiSession.UserId)
	assert.Equal(t, userID, *apiSession.UserId)
	require.NotNil(t, apiSession.Metadata)
	assert.Equal(t, "value", (*apiSession.Metadata)["key"])
}

func TestSessionWithCountToAPI_IncludesTraceCountAndExternalID(t *testing.T) {
	session := platform.Session{
		ID:         uuid.New(),
		ProjectID:  uuid.New(),
		ExternalID: "checkout-flow-42",
		CreatedAt:  time.Now().UTC(),
	}

	apiSession := sessionWithCountToAPI(&session, 7)

	assert.Equal(t, session.ExternalID, apiSession.ExternalId)
	require.NotNil(t, apiSession.TraceCount)
	assert.Equal(t, 7, *apiSession.TraceCount)
}
