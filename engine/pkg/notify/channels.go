package notify

const (
	// ChannelRuns wakes workflow workers when run work may be available.
	ChannelRuns = "engine_runs"
	// ChannelActivity wakes activity workers when activity work may be available.
	ChannelActivity = "engine_activity"
	// ChannelInbox wakes workflow workers when inbox work may be available.
	ChannelInbox = "engine_inbox"
)
