package main

import "testing"

func TestPopFlag(t *testing.T) {
	got, rest := popFlag([]string{"--scope", "service", "--url", "wss://x"}, "--scope")
	if got != "service" {
		t.Errorf("valor = %q; se esperaba service", got)
	}
	if len(rest) != 2 || rest[0] != "--url" || rest[1] != "wss://x" {
		t.Errorf("resto = %v; se esperaba [--url wss://x]", rest)
	}

	// Ausente: valor vacío y args intactos.
	got, rest = popFlag([]string{"--url", "wss://x"}, "--scope")
	if got != "" || len(rest) != 2 {
		t.Errorf("ausente: valor=%q resto=%v", got, rest)
	}

	// Flag sin valor al final: se trata como ausente (no hay valor que tomar).
	if v, _ := popFlag([]string{"--scope"}, "--scope"); v != "" {
		t.Errorf("flag sin valor: %q; se esperaba vacío", v)
	}
}

func TestResolveConfigPath(t *testing.T) {
	// --config explícito gana sobre el scope.
	path, _, hasScope, err := resolveConfigPath("/tmp/c.yaml", "service")
	if err != nil || path != "/tmp/c.yaml" || !hasScope {
		t.Errorf("explícito: path=%q hasScope=%v err=%v", path, hasScope, err)
	}

	// Sin nada: config.yaml del directorio actual (dev).
	path, _, hasScope, err = resolveConfigPath("", "")
	if err != nil || path != "config.yaml" || hasScope {
		t.Errorf("por defecto: path=%q hasScope=%v err=%v", path, hasScope, err)
	}

	// Scope inválido: error.
	if _, _, _, err := resolveConfigPath("", "root"); err == nil {
		t.Error("un scope inválido debe dar error")
	}

	// Scope válido sin --config: deriva la ruta del scope (no vacía, no cwd).
	path, _, hasScope, err = resolveConfigPath("", "user")
	if err != nil || !hasScope || path == "" || path == "config.yaml" {
		t.Errorf("scope user: path=%q hasScope=%v err=%v", path, hasScope, err)
	}
}
