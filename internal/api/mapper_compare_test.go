package api

import (
	"testing"

	"github.com/continua-ai/continua/internal/store"
	"github.com/stretchr/testify/assert"
)

func TestSessionCompareSpanDiffToAPI_PreservesEmptyChangedFieldsAsArray(t *testing.T) {
	row := sessionCompareSpanDiffToAPI(&store.SessionCompareSpanDiffRow{
		ChangedFields: nil,
		SemanticGroups: []store.SessionCompareSemanticDiffGroup{
			{ChangedFields: nil},
		},
	})

	assert.NotNil(t, row.ChangedFields)
	assert.Empty(t, row.ChangedFields)
	assert.Len(t, row.SemanticGroups, 1)
	assert.NotNil(t, row.SemanticGroups[0].ChangedFields)
	assert.Empty(t, row.SemanticGroups[0].ChangedFields)
}
