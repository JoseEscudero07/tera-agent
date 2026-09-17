package main

import (
	"errors"
	"time"
)

// errWaitTimeout indica que la condición no se cumplió dentro del plazo.
var errWaitTimeout = errors.New("tiempo de espera agotado")

// waitFor comprueba check cada poll hasta que devuelve true, devuelve un error o
// pasa timeout. Vive fuera de service_windows.go para poder probarlo sin el SCM.
func waitFor(timeout, poll time.Duration, check func() (bool, error)) error {
	deadline := time.Now().Add(timeout)
	for {
		done, err := check()
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		if time.Now().After(deadline) {
			return errWaitTimeout
		}
		time.Sleep(poll)
	}
}
