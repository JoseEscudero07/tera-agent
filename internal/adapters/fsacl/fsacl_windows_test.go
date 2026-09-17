//go:build windows

package fsacl

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

// SecureServiceDir deja una carpeta que VerifyNoUntrustedWrite acepta: solo
// SYSTEM y Administradores pueden escribir.
func TestSecureServiceDirPassesVerify(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "TeraAgent")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := SecureServiceDir(dir); err != nil {
		t.Fatalf("SecureServiceDir: %v", err)
	}
	if err := VerifyNoUntrustedWrite(dir); err != nil {
		t.Errorf("VerifyNoUntrustedWrite tras asegurar: %v", err)
	}
}

// GrantUsersRead añade lectura para el grupo Usuarios pero NO escritura, así que
// la carpeta sigue considerándose segura.
func TestGrantUsersReadStaysSafe(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := GrantUsersRead(dir); err != nil {
		t.Fatalf("GrantUsersRead: %v", err)
	}
	if err := VerifyNoUntrustedWrite(dir); err != nil {
		t.Errorf("Usuarios con solo lectura no debe considerarse inseguro: %v", err)
	}
}

// Si el grupo Usuarios tiene escritura, VerifyNoUntrustedWrite debe fallar: es el
// caso exacto del hallazgo original (Permissions: users-modify).
func TestVerifyDetectsUsersWrite(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "abierta")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	users, err := windows.CreateWellKnownSid(windows.WinBuiltinUsersSid)
	if err != nil {
		t.Fatal(err)
	}
	system, _ := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	entries := []windows.EXPLICIT_ACCESS{
		grant(system, windows.GENERIC_ALL),
		grant(users, windows.GENERIC_ALL), // Usuarios con control total: inseguro
	}
	acl, err := windows.ACLFromEntries(entries, nil)
	if err != nil {
		t.Fatal(err)
	}
	secInfo := windows.SECURITY_INFORMATION(
		windows.DACL_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION)
	if err := windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, secInfo, nil, nil, acl, nil); err != nil {
		t.Fatal(err)
	}
	if err := VerifyNoUntrustedWrite(dir); err == nil {
		t.Error("VerifyNoUntrustedWrite debe detectar la escritura del grupo Usuarios")
	}
}
