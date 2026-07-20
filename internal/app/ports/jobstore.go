package ports

// JobStore persists just enough state to make job handling reliable across
// reconnects and restarts:
//
//   - Seen/MarkDone provide idempotency so a redelivered job is not reprinted.
//   - AddPending/TakePending buffer job results (job_completed/job_failed) that
//     could not be sent while disconnected, to be flushed on reconnect.
//
// Implementations must be safe for concurrent use.
type JobStore interface {
	// Seen reports whether the job id has already been processed.
	Seen(id string) bool
	// MarkDone records the job id as processed.
	MarkDone(id string)
	// AddPending stores a result frame (raw JSON) that failed to send.
	AddPending(frame []byte)
	// TakePending returns and clears all buffered result frames.
	TakePending() [][]byte
}
