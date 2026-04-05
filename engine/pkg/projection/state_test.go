package projection

import "testing"

func TestFromHistoryIDs(t *testing.T) {
	testCases := []struct {
		name                   string
		latestHistoryID        int64
		lastProjectedHistoryID int64
		want                   State
	}{
		{
			name:                   "caught up stays up_to_date",
			latestHistoryID:        5,
			lastProjectedHistoryID: 5,
			want:                   StateUpToDate,
		},
		{
			name:                   "new history becomes catching_up",
			latestHistoryID:        6,
			lastProjectedHistoryID: 5,
			want:                   StateCatchingUp,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := FromHistoryIDs(tc.latestHistoryID, tc.lastProjectedHistoryID)
			if got != tc.want {
				t.Fatalf("FromHistoryIDs(%d, %d) = %q, want %q", tc.latestHistoryID, tc.lastProjectedHistoryID, got, tc.want)
			}
		})
	}
}

func TestApplyRetention(t *testing.T) {
	if got := ApplyRetention(true); got != StateSummaryOnly {
		t.Fatalf("ApplyRetention(true) = %q, want %q", got, StateSummaryOnly)
	}
	if got := ApplyRetention(false); got != StateJournalExpired {
		t.Fatalf("ApplyRetention(false) = %q, want %q", got, StateJournalExpired)
	}
}
