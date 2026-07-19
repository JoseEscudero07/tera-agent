// Package job defines the work units the Backend dispatches to the Agent.
// Domain layer — no transport, printing or device dependencies.
// Owner: Go Core Engineer (entities); handlers implemented by feature agents.
package job

import "time"

// Kind classifies a job so the dispatcher can route it to the right handler.
type Kind string

const (
	KindPrint  Kind = "PRINT"
	KindDevice Kind = "DEVICE"
)

// Job is a unit of work received from the Backend. Payload is left opaque at
// the domain level; the handler for the given Kind interprets it.
type Job struct {
	ID       string
	Kind     Kind
	Payload  []byte
	Received time.Time
}
