package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealth_OK(t *testing.T) {
	s := New(nil, nil, nil, nil, "")
	// /health does not touch engine/profiles/disc, so nils are fine.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	s.handleHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type = %q", ct)
	}
}

func TestAuth_RejectsMissingToken(t *testing.T) {
	s := New(nil, nil, nil, nil, "secret")
	called := false
	h := s.auth(func(http.ResponseWriter, *http.Request) { called = true })

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/printers", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if called {
		t.Fatal("handler should not run without a valid token")
	}
}

func TestAuth_AllowsValidToken(t *testing.T) {
	s := New(nil, nil, nil, nil, "secret")
	called := false
	h := s.auth(func(w http.ResponseWriter, _ *http.Request) { called = true; w.WriteHeader(200) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/printers", nil)
	req.Header.Set("Authorization", "Bearer secret")
	h(rec, req)

	if !called {
		t.Fatal("handler should run with a valid token")
	}
}
