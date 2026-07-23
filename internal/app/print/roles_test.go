package print

import (
	"testing"

	"github.com/teraerp/tera-agent/internal/app/ports"
)

func TestRoleResolver(t *testing.T) {
	cases := []struct {
		name     string
		printers []ports.ManagedPrinter
		def      string
		role     string
		want     string
	}{
		{
			name:     "single match by role",
			printers: []ports.ManagedPrinter{{Name: "POS80", Role: ports.RoleFacturacion, Enabled: true}},
			def:      "POS80",
			role:     "facturacion",
			want:     "POS80",
		},
		{
			name: "multiple matches prefer default",
			printers: []ports.ManagedPrinter{
				{Name: "POS80-Caja1", Role: ports.RoleFacturacion, Enabled: true},
				{Name: "POS80-Caja2", Role: ports.RoleFacturacion, Enabled: true},
			},
			def:  "POS80-Caja2",
			role: "facturacion",
			want: "POS80-Caja2",
		},
		{
			name: "multiple matches without default → first",
			printers: []ports.ManagedPrinter{
				{Name: "A", Role: ports.RoleCocina, Enabled: true},
				{Name: "B", Role: ports.RoleCocina, Enabled: true},
			},
			role: "cocina",
			want: "A",
		},
		{
			name:     "no match → default printer",
			printers: []ports.ManagedPrinter{{Name: "POS80", Role: ports.RoleFacturacion, Enabled: true}},
			def:      "POS80",
			role:     "cocina",
			want:     "POS80",
		},
		{
			name:     "disabled printer excluded",
			printers: []ports.ManagedPrinter{{Name: "POS80", Role: ports.RoleFacturacion, Enabled: false}},
			def:      "HP",
			role:     "facturacion",
			want:     "HP",
		},
		{
			name: "empty role → default",
			def:  "POS80",
			want: "POS80",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRoleResolver(tc.printers, tc.def)
			if got := r.Resolve(tc.role); got != tc.want {
				t.Errorf("Resolve(%q) = %q, want %q", tc.role, got, tc.want)
			}
		})
	}
}

func TestRoleResolverUpdate(t *testing.T) {
	r := NewRoleResolver(nil, "")
	if got := r.Resolve("cocina"); got != "" {
		t.Fatalf("initial: got %q, want empty", got)
	}
	r.Update([]ports.ManagedPrinter{
		{Name: "Cocina1", Role: ports.RoleCocina, Enabled: true},
	}, "")
	if got := r.Resolve("cocina"); got != "Cocina1" {
		t.Fatalf("after update: got %q, want Cocina1", got)
	}
}
