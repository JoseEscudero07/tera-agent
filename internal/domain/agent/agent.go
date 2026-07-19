package agent

// Identity holds the immutable data the Backend assigns to this Agent during
// the initial registration handshake. The Agent never generates these values;
// it receives them from the Backend after a valid Token is presented.
type Identity struct {
	UUID     string // unique Agent id assigned by the Backend
	Empresa  string // company
	Sucursal string // branch
	Equipo   string // device/workstation
}

// Valid reports whether the identity has the minimum fields to operate.
func (i Identity) Valid() bool {
	return i.UUID != "" && i.Empresa != ""
}
