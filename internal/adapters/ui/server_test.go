package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/teraerp/tera-agent/internal/app/ports"
	"github.com/teraerp/tera-agent/internal/domain/agent"
)

// newTestServer builds a Server with just the collaborators the config endpoints
// touch, capturing what gets persisted and whether a restart was requested.
func newTestServer(cfg ports.Config) (*Server, *ports.Config, *bool) {
	saved := &ports.Config{}
	restarted := new(bool)
	s := &Server{d: Deps{
		Machine:      agent.NewMachine(),
		Info:         agent.NewInfo(),
		Cfg:          cfg,
		RunMode:      "Aplicación de usuario",
		SaveConfig:   func(c ports.Config) error { *saved = c; return nil },
		OnRegistered: func() { *restarted = true },
	}}
	return s, saved, restarted
}

func postConfig(t *testing.T, s *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	s.config(rec, httptest.NewRequest(http.MethodPost, "/api/config", strings.NewReader(body)))
	return rec
}

// Regresión: guardar el formulario sin el campo `url` NO debe borrar la URL del
// Backend. Antes los campos eran valores planos y un POST sin `url` (o con la
// cadena vacía) desregistraba el equipo silenciosamente.
func TestConfigPOSTOmittedFieldsArePreserved(t *testing.T) {
	cfg := ports.Config{
		BackendURL: "wss://erp.example.com/ws/agent/",
		Token:      "token-secreto-1a2b",
		AgentID:    "Caja 1",
		LogLevel:   "info",
	}
	s, saved, _ := newTestServer(cfg)

	rec := postConfig(t, s, `{"log":"debug"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if saved.BackendURL != cfg.BackendURL {
		t.Errorf("BackendURL = %q; se esperaba conservar %q", saved.BackendURL, cfg.BackendURL)
	}
	if saved.Token != cfg.Token {
		t.Errorf("Token = %q; se esperaba conservar %q", saved.Token, cfg.Token)
	}
	if saved.AgentID != cfg.AgentID {
		t.Errorf("AgentID = %q; se esperaba conservar %q", saved.AgentID, cfg.AgentID)
	}
	if saved.LogLevel != "debug" {
		t.Errorf("LogLevel = %q; se esperaba debug", saved.LogLevel)
	}
}

// Un token vacío significa "no lo cambies", no "bórralo": es lo que permite
// guardar el formulario sin reescribir la credencial en cada guardado.
func TestConfigPOSTEmptyTokenKeepsCurrent(t *testing.T) {
	s, saved, restarted := newTestServer(ports.Config{
		BackendURL: "wss://erp.example.com/ws/agent/",
		Token:      "token-secreto-1a2b",
	})

	if rec := postConfig(t, s, `{"token":"   ","nombre":"Caja 2"}`); rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if saved.Token != "token-secreto-1a2b" {
		t.Errorf("Token = %q; un token vacío no debe cambiarlo", saved.Token)
	}
	if *restarted {
		t.Error("no debería reiniciar: no hubo cambio de conexión")
	}
}

// Cambiar el token sí es un cambio de conexión: debe persistirse y disparar el
// reinicio, porque la sesión actual con el Backend queda invalidada.
func TestConfigPOSTTokenChangeTriggersRestart(t *testing.T) {
	s, saved, _ := newTestServer(ports.Config{
		BackendURL: "wss://erp.example.com/ws/agent/",
		Token:      "viejo",
	})

	rec := postConfig(t, s, `{"token":"nuevo-token"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if saved.Token != "nuevo-token" {
		t.Errorf("Token = %q; se esperaba nuevo-token", saved.Token)
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["restarting"] != true {
		t.Errorf("la respuesta debe avisar restarting=true, got %v", resp)
	}
}

// Una URL con esquema equivocado se rechaza antes de persistirla. Antes se
// guardaba tal cual y el agente entraba en un bucle de reconexión sin explicar
// por qué.
func TestConfigPOSTRejectsBadURL(t *testing.T) {
	original := "wss://erp.example.com/ws/agent/"
	s, saved, _ := newTestServer(ports.Config{BackendURL: original, Token: "t"})

	rec := postConfig(t, s, `{"url":"http://erp.example.com/ws/agent/"}`)
	if rec.Code == http.StatusOK {
		t.Fatalf("una URL http:// debe rechazarse, status = %d", rec.Code)
	}
	if saved.BackendURL != "" {
		t.Errorf("no debe persistir nada al fallar la validación, guardó %q", saved.BackendURL)
	}
	if s.d.Cfg.BackendURL != original {
		t.Errorf("la config en memoria quedó modificada: %q", s.d.Cfg.BackendURL)
	}
}

// El GET no debe exponer el Token: sólo si existe y sus últimos 4 caracteres.
func TestConfigGETNeverLeaksToken(t *testing.T) {
	const token = "token-muy-secreto-9f8e"
	s, _, _ := newTestServer(ports.Config{BackendURL: "wss://x/ws/", Token: token})

	rec := httptest.NewRecorder()
	s.config(rec, httptest.NewRequest(http.MethodGet, "/api/config", nil))

	if strings.Contains(rec.Body.String(), token) {
		t.Fatalf("la respuesta filtra el token completo: %s", rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["tokenSet"] != true {
		t.Errorf("tokenSet = %v; se esperaba true", resp["tokenSet"])
	}
	if resp["tokenHint"] != "9f8e" {
		t.Errorf("tokenHint = %v; se esperaba 9f8e", resp["tokenHint"])
	}
}

// Desvincular borra ambas credenciales y reinicia, de forma explícita.
func TestUnregisterClearsCredentials(t *testing.T) {
	s, saved, _ := newTestServer(ports.Config{
		BackendURL:     "wss://erp.example.com/ws/agent/",
		Token:          "token-secreto",
		DefaultPrinter: "XP-80",
	})

	rec := httptest.NewRecorder()
	s.unregister(rec, httptest.NewRequest(http.MethodPost, "/api/unregister", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if saved.Token != "" || saved.BackendURL != "" {
		t.Errorf("credenciales no borradas: token=%q url=%q", saved.Token, saved.BackendURL)
	}
	// Desvincular no debe tocar la configuración de impresión del equipo.
	if saved.DefaultPrinter != "XP-80" {
		t.Errorf("DefaultPrinter = %q; desvincular no debe borrar impresoras", saved.DefaultPrinter)
	}
}

func TestValidateBackendURL(t *testing.T) {
	ok := []string{
		"wss://api.grupotera.cloud/ws/agent/",
		"ws://localhost:8000/ws/agent/",
		"wss://erp.example.com:8443/ws/agent/",
	}
	for _, u := range ok {
		if err := validateBackendURL(u); err != nil {
			t.Errorf("validateBackendURL(%q) = %v; se esperaba válida", u, err)
		}
	}
	bad := []string{
		"https://erp.example.com/ws/agent/", // esquema HTTP, no WebSocket
		"erp.example.com/ws/agent/",         // sin esquema
		"wss://",                            // sin host
		"",
	}
	for _, u := range bad {
		if err := validateBackendURL(u); err == nil {
			t.Errorf("validateBackendURL(%q) = nil; se esperaba error", u)
		}
	}
}

// Con tokens cortos no se devuelve pista: mejor ninguna que filtrar el secreto.
func TestTokenHint(t *testing.T) {
	cases := map[string]string{
		"":                   "",
		"corto":              "",
		"1234567":            "",
		"12345678":           "5678",
		"token-secreto-1a2b": "1a2b",
	}
	for in, want := range cases {
		if got := tokenHint(in); got != want {
			t.Errorf("tokenHint(%q) = %q; se esperaba %q", in, got, want)
		}
	}
}
