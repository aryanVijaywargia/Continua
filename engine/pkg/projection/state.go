package projection

type State string

const (
	StateUpToDate      State = "up_to_date"
	StateCatchingUp    State = "catching_up"
	StateSummaryOnly   State = "summary_only"
	StateJournalExpired State = "journal_expired"
)

func (s State) String() string {
	return string(s)
}

func FromHistoryIDs(latestHistoryID, lastProjectedHistoryID int64) State {
	if latestHistoryID > lastProjectedHistoryID {
		return StateCatchingUp
	}
	return StateUpToDate
}

func ApplyRetention(historyRetained bool) State {
	if historyRetained {
		return StateSummaryOnly
	}
	return StateJournalExpired
}
