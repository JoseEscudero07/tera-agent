package ports

import "time"

// Clock abstracts time so components (heartbeat, reconnection backoff) stay
// testable. The production adapter delegates to the standard library; tests
// can supply a fake.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}
