package update

import (
	"strconv"
	"strings"
)

// Compare compara dos versiones semver "X.Y.Z" (con posible sufijo -pre y
// prefijo 'v' opcional). Devuelve -1 si a<b, 0 si iguales, 1 si a>b.
//
// Reglas (subconjunto pragmático de semver): se comparan MAJOR.MINOR.PATCH
// numéricamente; una versión CON sufijo de pre-release (ej. 1.3.0-rc1) es MENOR
// que la misma SIN sufijo (1.3.0). Partes ausentes valen 0. Trozos no numéricos
// en X.Y.Z se tratan como 0 (defensivo ante entradas raras del Backend).
func Compare(a, b string) int {
	an, apre := parse(a)
	bn, bpre := parse(b)
	for i := 0; i < 3; i++ {
		if an[i] != bn[i] {
			if an[i] < bn[i] {
				return -1
			}
			return 1
		}
	}
	// Igual núcleo: la que tiene pre-release es menor.
	switch {
	case apre == "" && bpre == "":
		return 0
	case apre == "" && bpre != "":
		return 1 // a es release, b es pre → a > b
	case apre != "" && bpre == "":
		return -1
	default:
		return strings.Compare(apre, bpre)
	}
}

// Newer reporta si latest es estrictamente más nueva que current.
func Newer(latest, current string) bool { return Compare(latest, current) > 0 }

// parse devuelve [major,minor,patch] y el sufijo de pre-release (sin el '-').
func parse(v string) ([3]int, string) {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	pre := ""
	if i := strings.IndexByte(v, '-'); i >= 0 {
		pre = v[i+1:]
		v = v[:i]
	}
	// Descarta metadata de build (+...), no participa en el orden.
	if i := strings.IndexByte(pre, '+'); i >= 0 {
		pre = pre[:i]
	}
	var out [3]int
	for i, part := range strings.SplitN(v, ".", 3) {
		if i > 2 {
			break
		}
		n, _ := strconv.Atoi(strings.TrimSpace(part))
		out[i] = n
	}
	return out, pre
}
