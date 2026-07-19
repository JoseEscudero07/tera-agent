package agent

import "testing"

func TestMachine_ValidTransitionNotifiesObservers(t *testing.T) {
	m := NewMachine()

	var gotFrom, gotTo State
	m.Subscribe(func(from, to State) {
		gotFrom, gotTo = from, to
	})

	if got := m.Current(); got != StateStarting {
		t.Fatalf("initial state = %s, want %s", got, StateStarting)
	}
	if err := m.Transition(StateConnecting); err != nil {
		t.Fatalf("Transition(CONNECTING) returned error: %v", err)
	}
	if m.Current() != StateConnecting {
		t.Fatalf("current = %s, want %s", m.Current(), StateConnecting)
	}
	if gotFrom != StateStarting || gotTo != StateConnecting {
		t.Fatalf("observer got %s->%s, want %s->%s", gotFrom, gotTo, StateStarting, StateConnecting)
	}
}

func TestMachine_InvalidTransitionRejected(t *testing.T) {
	m := NewMachine()

	err := m.Transition(StateConnected) // STARTING -> CONNECTED is not allowed
	if err == nil {
		t.Fatal("expected ErrInvalidTransition, got nil")
	}
	if _, ok := err.(ErrInvalidTransition); !ok {
		t.Fatalf("expected ErrInvalidTransition, got %T", err)
	}
	if m.Current() != StateStarting {
		t.Fatalf("state changed to %s after invalid transition", m.Current())
	}
}

func TestMachine_ShuttingDownIsTerminal(t *testing.T) {
	m := NewMachine()
	if err := m.Transition(StateShuttingDown); err != nil {
		t.Fatalf("Transition(SHUTTING_DOWN) returned error: %v", err)
	}
	if err := m.Transition(StateConnecting); err == nil {
		t.Fatal("expected transition out of SHUTTING_DOWN to be rejected")
	}
}
