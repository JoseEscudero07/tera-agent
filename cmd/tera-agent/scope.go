package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/teraerp/tera-agent/internal/adapters/apppath"
	"github.com/teraerp/tera-agent/internal/adapters/config"
	"github.com/teraerp/tera-agent/internal/adapters/fsacl"
)

// readLine lee una línea de stdin sin el salto final. Compartida por el registro
// interactivo (URL) y por readSecret.
func readLine() string {
	sc := bufio.NewScanner(os.Stdin)
	if sc.Scan() {
		return strings.TrimRight(sc.Text(), "\r\n")
	}
	return ""
}

// panelConnReadOnly indica si el panel debe tratar los datos de conexión (URL y
// Token) como de solo lectura. Es cierto en modo servicio: el config vive en una
// carpeta que solo Administradores pueden tocar, así que cambiarlo desde un panel
// que cualquiera puede abrir sería una vía para saltarse esa protección. El
// registro en servicio se hace con `tera-agent register` como administrador.
var panelConnReadOnly bool

// resolveConfigPath decide la ruta del config y el scope a partir de las flags.
// Precedencia: --config explícito gana (dev y compatibilidad); si no, se deriva
// del --scope; si no hay ninguno, "config.yaml" en el directorio actual (dev).
func resolveConfigPath(explicit, scope string) (path string, sc apppath.Scope, hasScope bool, err error) {
	if scope != "" {
		sc, err = apppath.ParseScope(scope)
		if err != nil {
			return "", "", false, err
		}
		hasScope = true
	}
	switch {
	case explicit != "":
		path = explicit
	case hasScope:
		path, err = sc.ConfigPath()
	default:
		path = "config.yaml"
	}
	return path, sc, hasScope, err
}

// prepareForRun resuelve la ruta del config y, según el scope, prepara la carpeta
// de datos antes de arrancar. Devuelve la ruta a pasar a di.Build.
//
//   - Sin scope (dev / --config explícito): no toca permisos ni crea nada nuevo.
//   - user:    crea la config del usuario si falta (protegida por el perfil).
//   - service: exige que la carpeta de datos sea segura (solo SYSTEM y
//     Administradores pueden escribir). Si no lo es, se NIEGA a arrancar: es la
//     señal de que la instalación no aplicó los permisos y el Token podría estar
//     expuesto. También marca el panel como de solo lectura para la conexión.
func prepareForRun(explicit, scope string) (path string, err error) {
	path, sc, hasScope, err := resolveConfigPath(explicit, scope)
	if err != nil || !hasScope {
		return path, err
	}
	dataDir, err := sc.DataDir()
	if err != nil {
		return "", err
	}
	if _, err := config.EnsureDefault(path, dataDir); err != nil {
		return "", err
	}
	if sc == apppath.ScopeService {
		if err := fsacl.VerifyNoUntrustedWrite(dataDir); err != nil {
			return "", fmt.Errorf("la carpeta de datos %q no es segura: %w.\n"+
				"Reinstala el Agent o ejecuta como administrador: tera-agent setup-data --scope service", dataDir, err)
		}
		panelConnReadOnly = true
	}
	return path, nil
}

// cmdSetupData crea la carpeta de datos con los permisos correctos y escribe la
// configuración inicial si falta. Lo invoca el instalador de Windows (elevado)
// en lugar de crear la carpeta con permisos abiertos y escribir el YAML a mano.
// Es idempotente: en una actualización reaplica los permisos y conserva el
// config existente (Token, impresoras).
func cmdSetupData(args []string) error {
	scope, rest := popFlag(args, "--scope")
	if scope == "" && len(rest) > 0 {
		scope = rest[0]
	}
	sc, err := apppath.ParseScope(scope)
	if err != nil {
		return fmt.Errorf("setup-data: %w", err)
	}
	dataDir, err := sc.DataDir()
	if err != nil {
		return err
	}
	configPath, err := sc.ConfigPath()
	if err != nil {
		return err
	}
	created, err := config.EnsureDefault(configPath, dataDir)
	if err != nil {
		return err
	}
	if sc == apppath.ScopeService {
		if err := fsacl.SecureServiceDir(dataDir); err != nil {
			return err
		}
		if err := fsacl.GrantUsersRead(filepath.Join(dataDir, config.LogSubdir)); err != nil {
			return err
		}
	}
	if created {
		fmt.Printf("configuración creada en %s\n", configPath)
	} else {
		fmt.Printf("configuración conservada en %s\n", configPath)
	}
	fmt.Printf("carpeta de datos lista: %s\n", dataDir)
	return nil
}

// cmdRegister vincula el equipo con el Backend desde la consola. Es el camino de
// registro en modo servicio, donde el panel deja los datos de conexión en solo
// lectura porque su config solo la pueden tocar los administradores.
//
//	tera-agent register --scope service                 (pide URL y Token)
//	tera-agent register --scope service --url wss://... --token-file C:\tok.txt
//
// El Token nunca se pasa como argumento (quedaría en el historial y en la lista
// de procesos): se teclea sin eco o se lee de un fichero.
func cmdRegister(args []string) error {
	scope, _ := popFlag(args, "--scope")
	if scope == "" {
		scope = string(apppath.ScopeService) // el caso que este comando resuelve
	}
	cfgPath, _ := popFlag(args, "--config")
	urlFlag, _ := popFlag(args, "--url")
	tokenFile, _ := popFlag(args, "--token-file")

	sc, err := apppath.ParseScope(scope)
	if err != nil {
		return fmt.Errorf("register: %w", err)
	}
	if err := requireElevatedForScope(sc); err != nil {
		return err
	}

	path := cfgPath
	if path == "" {
		if path, err = sc.ConfigPath(); err != nil {
			return err
		}
	}

	urlv := strings.TrimSpace(urlFlag)
	if urlv == "" {
		fmt.Print("URL del servidor (wss://...): ")
		urlv = readLine()
	}

	var token string
	if tokenFile != "" {
		b, readErr := os.ReadFile(tokenFile)
		if readErr != nil {
			return fmt.Errorf("register: no se pudo leer el token: %w", readErr)
		}
		token = strings.TrimSpace(string(b))
	} else if token, err = readSecret("Token del ERP: "); err != nil {
		return err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("register: token vacío")
	}

	dataDir, err := sc.DataDir()
	if err != nil {
		return err
	}
	if _, err := config.EnsureDefault(path, dataDir); err != nil {
		return hintForWriteError(sc, err)
	}
	store := config.New(path)
	cfg, err := store.Load()
	if err != nil {
		return err
	}
	if urlv != "" {
		if err := config.ValidateBackendURL(urlv); err != nil {
			return err
		}
		cfg.BackendURL = urlv
	}
	if cfg.BackendURL == "" {
		return fmt.Errorf("register: falta la URL del servidor (usa --url)")
	}
	cfg.Token = token
	if err := store.Save(cfg); err != nil {
		return hintForWriteError(sc, err)
	}
	fmt.Println("registro guardado en", path)

	if sc == apppath.ScopeService {
		if err := restartServiceBestEffort(); err != nil {
			fmt.Fprintf(os.Stderr, "aviso: aplica el registro reiniciando el servicio: %v\n", err)
			return nil
		}
		fmt.Println("servicio reiniciado; el equipo debería conectar en unos segundos")
	}
	return nil
}

// popFlag extrae el valor de "--name valor" de args y devuelve el resto. Un mini
// parser sirve porque setup-data/register tienen muy pocas flags y así no chocan
// con los FlagSet de los demás comandos.
func popFlag(args []string, name string) (value string, rest []string) {
	for i := 0; i < len(args); i++ {
		if args[i] == name && i+1 < len(args) {
			return args[i+1], append(append([]string{}, args[:i]...), args[i+2:]...)
		}
	}
	return "", args
}
