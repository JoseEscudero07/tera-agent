package apppath

import (
	"path/filepath"
	"testing"
)

func TestParseScope(t *testing.T) {
	for _, s := range []string{"service", "user"} {
		if _, err := ParseScope(s); err != nil {
			t.Errorf("ParseScope(%q) = %v; se esperaba válido", s, err)
		}
	}
	for _, s := range []string{"", "System", "admin", "servicio"} {
		if _, err := ParseScope(s); err == nil {
			t.Errorf("ParseScope(%q) = nil; se esperaba error", s)
		}
	}
}

// ConfigPath debe colgar de DataDir del mismo scope: config y datos viven juntos,
// y la protección de la carpeta cubre a ambos.
func TestConfigPathUnderDataDir(t *testing.T) {
	for _, sc := range []Scope{ScopeService, ScopeUser} {
		dd, err := sc.DataDir()
		if err != nil {
			t.Fatalf("DataDir(%s): %v", sc, err)
		}
		cp, err := sc.ConfigPath()
		if err != nil {
			t.Fatalf("ConfigPath(%s): %v", sc, err)
		}
		if filepath.Dir(cp) != dd {
			t.Errorf("scope %s: ConfigPath %q no cuelga de DataDir %q", sc, cp, dd)
		}
	}
}
