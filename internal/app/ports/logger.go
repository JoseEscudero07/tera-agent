// Package ports declares the driven-side interfaces (secondary ports) that the
// application layer depends on. Adapters implement them. Keeping these small
// keeps the core decoupled from concrete infrastructure. Owner: Go Core Engineer.
package ports

// Logger is the minimal structured-logging contract used across the core.
// The concrete implementation lives in internal/adapters/logger.
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}
