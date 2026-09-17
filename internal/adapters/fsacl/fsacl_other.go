//go:build !windows

package fsacl

// En POSIX no hay ACLs de Windows que aplicar. El servicio de Linux corre como
// root con la carpeta en /var/lib/tera-agent (root:root) y el config se escribe
// con modo 0600, así que la protección equivalente ya la dan los permisos de
// fichero. Estas funciones existen para que el código común compile en todas las
// plataformas sin condicionar por SO en cada llamada.

// SecureServiceDir no hace nada en POSIX.
func SecureServiceDir(string) error { return nil }

// GrantUsersRead no hace nada en POSIX.
func GrantUsersRead(string) error { return nil }

// VerifyNoUntrustedWrite considera segura cualquier ruta en POSIX (los permisos
// de fichero son responsabilidad del administrador / del empaquetado systemd).
func VerifyNoUntrustedWrite(string) error { return nil }
