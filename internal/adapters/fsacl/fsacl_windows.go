//go:build windows

package fsacl

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Trabajamos con SIDs bien conocidos y NUNCA con nombres: "BUILTIN\Usuarios" en
// un Windows en español, "Users" en inglés, y otra cosa en cada idioma; el SID
// S-1-5-32-545 es el mismo en todos. icacls por nombre fallaría en los equipos
// que no están en inglés, que son justo los de los clientes.

// wellKnownSIDs devuelve los SIDs que usamos, creados desde su identificador
// canónico.
func wellKnownSIDs() (system, admins, users, creatorOwner *windows.SID, err error) {
	if system, err = windows.CreateWellKnownSid(windows.WinLocalSystemSid); err != nil {
		return
	}
	if admins, err = windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid); err != nil {
		return
	}
	if users, err = windows.CreateWellKnownSid(windows.WinBuiltinUsersSid); err != nil {
		return
	}
	creatorOwner, err = windows.CreateWellKnownSid(windows.WinCreatorOwnerSid)
	return
}

// fullAccess construye un EXPLICIT_ACCESS de control total heredable a
// subcarpetas y ficheros para sid.
func fullAccess(sid *windows.SID) windows.EXPLICIT_ACCESS {
	return grant(sid, windows.GENERIC_ALL)
}

// grant construye un EXPLICIT_ACCESS heredable con los permisos dados para sid.
func grant(sid *windows.SID, mask windows.ACCESS_MASK) windows.EXPLICIT_ACCESS {
	return windows.EXPLICIT_ACCESS{
		AccessPermissions: mask,
		AccessMode:        windows.GRANT_ACCESS,
		// Heredar a subcontenedores y objetos: los ficheros y subcarpetas que el
		// Agent cree (logs, jobs.json) nacen con estos mismos permisos.
		Inheritance: windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeValue: windows.TrusteeValueFromSID(sid),
		},
	}
}

// applyProtectedDACL fija en dir una DACL protegida (sin herencia del padre)
// construida con entries, y pone a Administradores como propietario. Protegerla
// es lo que corta la herencia de C:\ProgramData, que concede escritura al grupo
// Usuarios.
func applyProtectedDACL(dir string, entries []windows.EXPLICIT_ACCESS, owner *windows.SID) error {
	acl, err := windows.ACLFromEntries(entries, nil)
	if err != nil {
		return fmt.Errorf("fsacl: construir DACL: %w", err)
	}
	secInfo := windows.SECURITY_INFORMATION(
		windows.DACL_SECURITY_INFORMATION |
			windows.PROTECTED_DACL_SECURITY_INFORMATION |
			windows.OWNER_SECURITY_INFORMATION,
	)
	if err := windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, secInfo, owner, nil, acl, nil); err != nil {
		return fmt.Errorf("fsacl: aplicar DACL a %q: %w", dir, err)
	}
	return nil
}

// SecureServiceDir restringe dir a SYSTEM y Administradores (control total) y
// elimina cualquier acceso heredado, incluido el del grupo Usuarios. Es la
// carpeta de datos del servicio: contiene el Token y la configuración que
// arranca como LocalSystem.
func SecureServiceDir(dir string) error {
	system, admins, _, _, err := wellKnownSIDs()
	if err != nil {
		return fmt.Errorf("fsacl: SIDs: %w", err)
	}
	return applyProtectedDACL(dir, []windows.EXPLICIT_ACCESS{
		fullAccess(system),
		fullAccess(admins),
	}, admins)
}

// GrantUsersRead deja dir con SYSTEM y Administradores en control total y el
// grupo Usuarios en solo lectura. Se usa en la subcarpeta de logs: el soporte y
// un cajero sin privilegios pueden leer el log, pero nadie sin privilegios puede
// escribir ahí (evita que se manipule o se use para llenar el disco).
func GrantUsersRead(dir string) error {
	system, admins, users, _, err := wellKnownSIDs()
	if err != nil {
		return fmt.Errorf("fsacl: SIDs: %w", err)
	}
	return applyProtectedDACL(dir, []windows.EXPLICIT_ACCESS{
		fullAccess(system),
		fullAccess(admins),
		grant(users, windows.GENERIC_READ|windows.GENERIC_EXECUTE),
	}, admins)
}

// untrustedWriteMask son los permisos que, en manos de alguien que no sea SYSTEM,
// Administradores o el propietario, hacen insegura la carpeta: cualquier forma de
// escritura, borrado o toma de control.
const untrustedWriteMask = windows.FILE_WRITE_DATA |
	windows.FILE_APPEND_DATA |
	windows.FILE_WRITE_EA |
	windows.FILE_WRITE_ATTRIBUTES |
	windows.DELETE |
	windows.WRITE_DAC |
	windows.WRITE_OWNER |
	windows.GENERIC_WRITE |
	windows.GENERIC_ALL

// VerifyNoUntrustedWrite falla si algún SID que no sea de confianza (SYSTEM,
// Administradores o CREATOR OWNER) tiene permiso de escritura sobre dir. El
// servicio la usa al arrancar para NEGARSE a operar si su configuración vive en
// una carpeta que un usuario sin privilegios podría manipular.
func VerifyNoUntrustedWrite(dir string) error {
	sd, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("fsacl: leer DACL de %q: %w", dir, err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("fsacl: DACL de %q: %w", dir, err)
	}
	// Una DACL NULL (nil con present=true) concede todo a todos: claramente inseguro.
	if dacl == nil {
		return fmt.Errorf("fsacl: %q tiene DACL nula (acceso total para todos)", dir)
	}

	system, admins, _, creatorOwner, err := wellKnownSIDs()
	if err != nil {
		return fmt.Errorf("fsacl: SIDs: %w", err)
	}
	trusted := func(sid *windows.SID) bool {
		return sid.Equals(system) || sid.Equals(admins) || sid.Equals(creatorOwner)
	}

	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil {
			return fmt.Errorf("fsacl: leer ACE %d: %w", i, err)
		}
		// Solo los ACE de tipo ALLOW conceden acceso; los DENY solo lo recortan.
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			continue
		}
		if ace.Mask&untrustedWriteMask == 0 {
			continue // sin permisos de escritura relevantes
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if trusted(sid) {
			continue
		}
		return fmt.Errorf("fsacl: %q permite escritura a %s (debe restringirse a SYSTEM y Administradores)", dir, sid)
	}
	return nil
}
