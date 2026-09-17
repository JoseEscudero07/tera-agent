package main

import (
	"errors"
	"testing"
	"time"
)

// El caso del registro en modo servicio: el SCM tarda unas consultas en pasar de
// StopPending a Stopped, y hasta entonces no se puede volver a arrancar.
func TestWaitForReturnsWhenConditionHolds(t *testing.T) {
	calls := 0
	err := waitFor(time.Second, time.Millisecond, func() (bool, error) {
		calls++
		return calls == 3, nil
	})
	if err != nil {
		t.Fatalf("waitFor = %v; se esperaba nil", err)
	}
	if calls != 3 {
		t.Errorf("check llamado %d veces; se esperaban 3", calls)
	}
}

func TestWaitForStopsOnError(t *testing.T) {
	boom := errors.New("sin acceso al SCM")
	calls := 0
	err := waitFor(time.Second, time.Millisecond, func() (bool, error) {
		calls++
		return false, boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("waitFor = %v; se esperaba el error de check", err)
	}
	if calls != 1 {
		t.Errorf("tras un error no debe seguir consultando: %d llamadas", calls)
	}
}

// Un servicio que no termina de pararse no debe dejar colgado `register`.
func TestWaitForTimesOut(t *testing.T) {
	start := time.Now()
	err := waitFor(20*time.Millisecond, 5*time.Millisecond, func() (bool, error) { return false, nil })
	if !errors.Is(err, errWaitTimeout) {
		t.Fatalf("waitFor = %v; se esperaba errWaitTimeout", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("tardó %s en rendirse con un plazo de 20ms", elapsed)
	}
}
