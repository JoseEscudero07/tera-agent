package update

import "testing"

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.2.3", "1.2.3", 0},
		{"1.2.3", "1.2.4", -1},
		{"1.3.0", "1.2.9", 1},
		{"2.0.0", "1.9.9", 1},
		{"1.2", "1.2.0", 0},       // partes ausentes = 0
		{"v1.3.0", "1.3.0", 0},    // prefijo v
		{"1.3.0-rc1", "1.3.0", -1}, // pre-release < release
		{"1.3.0", "1.3.0-rc1", 1},
		{"1.3.0-rc1", "1.3.0-rc2", -1},
		{"1.10.0", "1.9.0", 1}, // comparación numérica, no lexicográfica
	}
	for _, c := range cases {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%q,%q)=%d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestNewer(t *testing.T) {
	if !Newer("1.3.0", "1.2.0") {
		t.Error("1.3.0 debe ser más nueva que 1.2.0")
	}
	if Newer("1.2.0", "1.2.0") {
		t.Error("igual no es más nueva")
	}
	if Newer("0.0.0-dev", "1.0.0") {
		t.Error("dev no debe considerarse más nueva que release")
	}
}
