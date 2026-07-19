// Package agent contains the enterprise rules for the Agent lifecycle,
// including the finite state machine that governs its runtime status.
//
// This package belongs to the domain layer: it has no dependencies on
// frameworks, transports, printers, devices or UI. Owner: Go Core Engineer.
package agent

import (
	"fmt"
	"sync"
)

// State is a valid runtime status of the Agent.
type State string

const (
	StateStarting      State = "STARTING"
	StateConnecting    State = "CONNECTING"
	StateAuthenticating State = "AUTHENTICATING"
	StateRegistering   State = "REGISTERING"
	StateConnected     State = "CONNECTED"
	StateDisconnected  State = "DISCONNECTED"
	StateReconnecting  State = "RECONNECTING"
	StateOffline       State = "OFFLINE"
	StateError         State = "ERROR"
	StateShuttingDown  State = "SHUTTING_DOWN"
)

// transitions declares, for each state, the set of states it may move to.
// Any transition not listed here is rejected by the Machine.
var transitions = map[State]map[State]struct{}{
	StateStarting:       set(StateConnecting, StateError, StateShuttingDown),
	StateConnecting:     set(StateAuthenticating, StateDisconnected, StateError, StateShuttingDown),
	StateAuthenticating: set(StateRegistering, StateConnected, StateDisconnected, StateError, StateShuttingDown),
	StateRegistering:    set(StateConnected, StateDisconnected, StateError, StateShuttingDown),
	StateConnected:      set(StateDisconnected, StateError, StateShuttingDown),
	StateDisconnected:   set(StateReconnecting, StateOffline, StateShuttingDown),
	StateReconnecting:   set(StateConnecting, StateOffline, StateError, StateShuttingDown),
	StateOffline:        set(StateReconnecting, StateShuttingDown),
	StateError:          set(StateReconnecting, StateOffline, StateShuttingDown),
	StateShuttingDown:   set(), // terminal
}

func set(states ...State) map[State]struct{} {
	m := make(map[State]struct{}, len(states))
	for _, s := range states {
		m[s] = struct{}{}
	}
	return m
}

// CanTransition reports whether moving from -> to is allowed.
func CanTransition(from, to State) bool {
	allowed, ok := transitions[from]
	if !ok {
		return false
	}
	_, ok = allowed[to]
	return ok
}

// ErrInvalidTransition is returned when a disallowed transition is attempted.
type ErrInvalidTransition struct {
	From State
	To   State
}

func (e ErrInvalidTransition) Error() string {
	return fmt.Sprintf("invalid state transition: %s -> %s", e.From, e.To)
}

// Observer is notified after every successful state change. The UI Engineer's
// tray/status view subscribes here; it never mutates state itself.
type Observer func(from, to State)

// Machine is a thread-safe finite state machine for the Agent lifecycle.
// The zero value is not usable; construct it with NewMachine.
type Machine struct {
	mu        sync.RWMutex
	current   State
	observers []Observer
}

// NewMachine returns a Machine starting in STARTING.
func NewMachine() *Machine {
	return &Machine{current: StateStarting}
}

// Current returns the current state.
func (m *Machine) Current() State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

// Subscribe registers an observer for state changes.
func (m *Machine) Subscribe(o Observer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observers = append(m.observers, o)
}

// Transition moves the machine to next if the transition is valid, notifying
// observers outside the lock. It returns ErrInvalidTransition otherwise.
func (m *Machine) Transition(next State) error {
	m.mu.Lock()
	from := m.current
	if !CanTransition(from, next) {
		m.mu.Unlock()
		return ErrInvalidTransition{From: from, To: next}
	}
	m.current = next
	observers := make([]Observer, len(m.observers))
	copy(observers, m.observers)
	m.mu.Unlock()

	for _, o := range observers {
		o(from, next)
	}
	return nil
}
