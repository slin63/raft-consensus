package responses

type Result struct {
	// CurrentTerm, for leader to update itself
	Term int

	// True if follower contained entry matching prevLogIndex and prevLogTerm
	Success bool

	// Response body for client requests
	Data string

	// Original entry for client requests
	Entry string

	// Index of entry processed
	Index int

	// Error code for testing
	Error int

	// If the sending candidate received the vote
	VoteGranted bool
}
