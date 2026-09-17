// Package fsacl restringe los permisos de la carpeta de datos del Agent en
// Windows. Es la barrera que impide que un usuario local sin privilegios lea el
// Token o manipule la configuración que el servicio arranca como LocalSystem.
//
// Toda la lógica de Windows vive en fsacl_windows.go. En el resto de plataformas
// las funciones son no-op / verificación trivial: en POSIX los permisos del
// fichero (0600) y la pertenencia de la carpeta a root ya cumplen ese papel.
// Owner: Security Engineer + Go Core Engineer.
package fsacl
