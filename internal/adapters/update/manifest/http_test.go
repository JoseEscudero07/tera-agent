package manifest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPBase(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"wss://api.grupotera.cloud/ws/agent/", "https://api.grupotera.cloud", true},
		{"ws://localhost:8000/ws/agent/", "http://localhost:8000", true},
		{"https://api.grupotera.cloud", "https://api.grupotera.cloud", true},
		{"", "", false},
		{"ftp://x/y", "", false},
	}
	for _, c := range cases {
		got, err := HTTPBase(c.in)
		if c.ok && (err != nil || got != c.want) {
			t.Errorf("HTTPBase(%q)=%q,%v want %q", c.in, got, err, c.want)
		}
		if !c.ok && err == nil {
			t.Errorf("HTTPBase(%q) debía fallar", c.in)
		}
	}
}

func TestFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != endpointPath {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("os") == "" || r.URL.Query().Get("current") == "" {
			http.Error(w, "faltan params", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"latest":"1.4.0","url":"https://x/bin","sha256":"abc","notes":"hola"}`))
	}))
	defer srv.Close()

	c := &Client{versionURL: srv.URL + endpointPath, http: srv.Client(), log: nopLog{}}
	m, err := c.Fetch(context.Background(), "1.3.0", "windows", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if m.Latest != "1.4.0" || m.URL != "https://x/bin" || m.SHA256 != "abc" {
		t.Fatalf("manifiesto inesperado: %+v", m)
	}
}

func TestFetchBadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", 500)
	}))
	defer srv.Close()
	c := &Client{versionURL: srv.URL + endpointPath, http: srv.Client(), log: nopLog{}}
	if _, err := c.Fetch(context.Background(), "1.3.0", "windows", "amd64"); err == nil {
		t.Error("HTTP 500 debe ser error")
	}
}

type nopLog struct{}

func (nopLog) Debug(string, ...any) {}
func (nopLog) Info(string, ...any)  {}
func (nopLog) Warn(string, ...any)  {}
func (nopLog) Error(string, ...any) {}
