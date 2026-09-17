package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadVerifiedOK(t *testing.T) {
	payload := []byte("binario nuevo v1.4.0")
	sum := sha256.Sum256(payload)
	hexSum := hex.EncodeToString(sum[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(payload)
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "out.bin")
	if err := downloadVerified(context.Background(), srv.Client(), srv.URL, hexSum, dst); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != string(payload) {
		t.Error("contenido descargado no coincide")
	}
}

func TestDownloadVerifiedBadHash(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("contenido manipulado"))
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "out.bin")
	err := downloadVerified(context.Background(), srv.Client(), srv.URL, "deadbeef", dst)
	if err == nil {
		t.Fatal("hash incorrecto debe fallar")
	}
	if _, statErr := os.Stat(dst); !os.IsNotExist(statErr) {
		t.Error("el fichero con hash malo debe borrarse")
	}
}

func TestDownloadVerifiedNoHash(t *testing.T) {
	if err := downloadVerified(context.Background(), http.DefaultClient, "http://x", "", "/tmp/x"); err == nil {
		t.Error("sin SHA256 debe fallar antes de descargar")
	}
}
