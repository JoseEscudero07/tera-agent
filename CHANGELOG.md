# Changelog

Formato basado en [Keep a Changelog](https://keepachangelog.com/es-ES/1.0.0/).

## [Unreleased]

### Security
- **Permisos de la carpeta de datos en Windows (endurecimiento).** El instalador
  creaba `C:\ProgramData\TeraAgent` con `Permissions: users-modify`, así que
  cualquier usuario local podía leer el **Token**, apuntar `server.url` a otro
  servidor (y recibir el Token en el siguiente arranque) o fijar `log.file` a una
  ruta arbitraria que el servicio (LocalSystem) crearía/abriría —una primitiva de
  escalada de privilegios y de DoS—. Ahora:
  - **Modo servicio:** la carpeta se restringe a **SYSTEM y Administradores**
    (nuevo `adapters/fsacl`, aplicado por `tera-agent setup-data --scope service`).
    Al arrancar, el servicio **verifica** que sigue siendo segura y se niega a
    arrancar si no lo es. El panel del servicio deja los datos de conexión (URL y
    Token) en **solo lectura**; el registro se hace con `tera-agent register`
    desde una consola de administrador (Token por consola sin eco o `--token-file`,
    nunca como argumento).
  - **Modo app de usuario:** la configuración pasa al perfil del usuario
    (`%LOCALAPPDATA%\TeraAgent`, `--scope user`), protegida por los permisos del
    perfil; cada cuenta tiene su propio registro. No se usa Roaming (el Token no
    debe viajar entre equipos).
  - **`log.file` debe estar dentro de `data_dir`** (validado al arrancar) para que
    la protección de la carpeta cubra también el destino del log; los logs viven
    en `data_dir\logs\` (en servicio, legibles por Usuarios pero no escribibles).
  - `config.yaml` inicial lo escribe ahora el propio Agent (`setup-data`), no el
    script Pascal del instalador: desaparece el bug de las comillas dobles en
    rutas de Windows.
- **El panel local (`127.0.0.1:9180`) aceptaba peticiones de cualquier web** abierta
  en el POS. Un `fetch` no-cors a `POST /api/config` cambiaba la URL del Backend y
  el agente mandaba el Token a ese servidor al reconectar; un
  `<img src=".../api/printers/drawer">` abría el cajón (y `/api/test-print`
  imprimía). Además, el WebSocket `/ws/ui` aceptaba cualquier origen y el panel
  era vulnerable a DNS rebinding. Ahora:
  - solo se atiende con cabecera Host `127.0.0.1:<puerto>` o `localhost:<puerto>`
    (403), también si se arranca con `--addr 0.0.0.0`;
  - todo lo que cambia estado exige Origin igual al del panel (403) y
    `Content-Type: application/json` (415);
  - cada ruta acepta solo su método (`POST /api/test-print`,
    `DELETE /api/printers`…); los GET a acciones ya no hacen nada;
  - el WebSocket del panel solo acepta el Origin del propio panel;
  - `X-Frame-Options: DENY` y `frame-ancestors 'none'` impiden meter el panel en
    un iframe para provocar clics (clickjacking).
- Desde el panel, `ws://` solo se admite contra el propio equipo (`localhost`,
  `127.0.0.0/8`, `::1`): sin TLS el Token viajaría en claro por la red. Para
  producción, `wss://`. (Editando `config.yaml` a mano `ws://` se sigue aceptando.)
- El panel construía los controles de cada impresora con handlers inline
  (`onchange="setKind('${nombre}',…)"`) y `esc()` solo escapaba comillas dobles:
  un nombre de impresora con `'` o `<` rompía el JS o inyectaba HTML. Ahora las
  filas usan `data-*` con listeners delegados, todo dato que va a `innerHTML` se
  escapa y los toasts usan `textContent`. No queda ningún handler inline.
- `windows-verify.ps1`: comprobaciones nuevas de permisos (carpeta sin herencia y
  sin escritura de usuarios sin privilegios, config no legible por Usuarios en
  servicio), de que `log.file` está dentro de `data_dir`, de que el panel rechaza
  un POST de otro origen y de que el panel del servicio rechaza el registro.
- Poppler 26.02 tenía un desbordamiento de memoria en `pdftoppm`
  (CVE-2026-10118, corregido en 26.06): un PDF manipulado podía ejecutar código,
  como SYSTEM en modo servicio. Actualizado a 26.09.
- `release.yml` verifica el SHA256 del zip de poppler (repositorio de terceros)
  antes de usarlo.
- La búsqueda de `pdftoppm`/`pdftocairo` solo acepta rutas absolutas con el nombre
  exacto: ya no resuelve por el directorio de trabajo (`exec.ErrDot`) ni variantes
  de PATHEXT como `pdftocairo.exe.bat`, que permitían ejecutar un binario plantado.
- Los procesos de poppler usan `WaitDelay`: un proceso hijo del driver de
  impresora que retenga stderr ya no deja el trabajo colgado más allá del timeout.

### Added
- **Compilar el instalador desde Linux**: `scripts/build-release.sh` (Go + Docker),
  mismo resultado que `build-release.ps1`, con Inno Setup bajo Wine
  (`installer/wine/Dockerfile`, instalador de Inno Setup verificado por SHA256).
  Versión y hash de poppler en `installer/poppler.env`, compartido con
  `release.yml`.
- **`tools/windows-vm`**: VM Windows 11 limpia en Docker para validar un instalador
  desde Linux. `test.sh` vuelve al estado limpio, instala (usuario o servicio),
  reinicia, pasa `windows-verify.ps1`, imprime en impresoras virtuales (térmica,
  láser y una con tildes) y comprueba lo que recibieron; `make-erp-pdfs.sh` genera
  facturas reales con el generador del ERP.
- `examples/mock-server`: `--format pdf` (perfil de impresora "normal" como lo
  envía el ERP), varios PDF separados por comas, `--exit` y una línea `RESULT`
  por trabajo para scripts.
- **Impresión vectorial en láser / inyección (Windows)** con `pdftocairo -print`
  (`driver/pdfvector`), modo por defecto. El texto llega al driver como texto y
  la impresora lo imprime a su resolución nativa; carta sale en carta y A4 en A4.
  Medido en Windows 11 limpio con facturas del ERP (fpdf2): 29 páginas en 25 s y
  24 MB, donde el camino raster no terminaba (8,6 GB comprometidos) y las páginas
  con color llegaban a la impresora a ~170 ppp.
- **Modo por impresora "Vectorial / Imagen"** en el panel (`page_mode` en
  `config.yaml`). Imagen es el camino GDI raster anterior, como opción de
  compatibilidad. El cambio aplica al siguiente trabajo sin reiniciar. Cae solo a
  imagen si falta `pdftocairo.exe`, para PNG/JPEG y para impresoras con tildes o
  ñ en Windows anteriores a 10 1903.
- `tools/popplerbundle`: empaqueta `pdftoppm.exe` y `pdftocairo.exe` con **solo**
  las DLLs que cargan (imports y carga diferida) y pone a `pdftocairo.exe` un
  manifiesto UTF-8 para que encuentre impresoras con tildes o ñ. Lo usan
  `build-release.ps1` y la compilación desde Linux.
- `windows-verify.ps1` comprueba que `pdftocairo.exe` está, arranca, soporta
  `-print` y lleva el manifiesto UTF-8 (20 comprobaciones).
- **Instalador de Windows** (`installer/tera-agent.iss`, Inno Setup 6): un
  `TeraAgent-Setup-<versión>.exe` que empaqueta el Agent, el binario de bandeja y
  **poppler completo** (`pdftoppm.exe` + sus 26 DLLs), configura el arranque
  automático, crea los accesos al panel y registra un desinstalador en
  "Aplicaciones". Actualiza in-place conservando la configuración y el Token.
  Soporta instalación silenciosa (`/VERYSILENT /MODE=service|user`).
- Asistente con **modo de ejecución excluyente** — app de usuario (bandeja +
  panel al iniciar sesión) o servicio de Windows. Son excluyentes porque dos
  agentes con el mismo Token duplicarían cada impresión.
- `scripts/build-release.ps1`: compila los dos binarios con la versión inyectada
  por `-ldflags`, **verifica que el binario reporte esa versión**, prepara el
  bundle de poppler, compila el instalador y emite `SHA256SUMS.txt` para el
  manifiesto `/agent/version` del ERP.
- `.github/workflows/release.yml`: release por tag semver, compilado en
  `windows-latest`, con release en borrador.
- Job de `windows-latest` en CI: los adapters de Windows (driver GDI, spooler,
  servicio) están detrás de build tags y hasta ahora **nunca se probaban** — el
  job de Ubuntu solo los cross-compilaba.
- `scripts/windows-verify.ps1`: verificación de aceptación de una instalación
  (19 comprobaciones). Cubre explícitamente los tres fallos que llegaron a
  clientes: runtime de Visual C++ ausente del bundle, `pdftoppm` que no arranca, y
  `config.yaml` con rutas entre comillas dobles. Distingue *omitida* de *correcta*
  para no dar por verificado lo que no se pudo comprobar.
- `docs/ACCEPTANCE.md`: cómo validar el instalador en una VM limpia (Windows
  Sandbox o VM), pasos manuales, prueba de actualización in-place y dónde mirar
  cuando falla. Documenta por qué el equipo de desarrollo ocultaba estos fallos.
- El instalador despliega `testpage.pdf` (600 bytes) para que la verificación y el
  soporte puedan ejercitar el pipeline PDF→raster→ESC/POS sin gastar papel.
- `docs/RELEASE.md`: versionamiento SemVer, flujo de release y el contrato del
  manifiesto de actualización para implementar el lado Django.
- Módulo de auto-actualización (`domain/update`, `app/update`,
  `adapters/update/{manifest,selfupdate}`): manifiesto del Backend, comparación
  semver, cliente HTTPS y descarga con verificación SHA256 obligatoria.
  **Parcial**: falta el `Applier` que reemplaza el binario y el cableado en `di`.

### Changed
- **Poppler 26.02 → 26.09** en el instalador y en `release.yml`, con el SHA256 del
  zip fijado y verificado antes de descomprimir (ver Security).
- El instalador ya no copia `*.dll` del bundle de poppler: `tools/popplerbundle`
  elige 30 ficheros de los 143 de 26.09. El runtime de Visual C++ sale del propio
  bundle (26.07+); solo se toma de System32 si el bundle no lo trae.
- `platform.NormalizingProfileCache` normaliza al **leer** el perfil, no al
  guardarlo, para que el modo de impresión cambie en caliente sin olvidar el
  perfil que envió el ERP.
- El perfil local de una láser (`ui.pdfProfile`) declara `pdf`, como el del ERP.
  En Linux, "Probar" en una láser ahora sale por CUPS en vez de fallar por falta
  de driver `gdi-raster`.
- `rasterizer/poppler` usa el nuevo paquete compartido `printing/popplerbin`
  (búsqueda del ejecutable, consola oculta y mensajes de error).
- Los errores de poppler ya no arrastran los avisos `No display font for …`.
- **Panel · Configuración: el Token ya se puede cambiar.** Antes el único campo de
  Token estaba en la pantalla de registro, que solo aparece si el equipo NO está
  registrado: una vez vinculado, cambiar la credencial exigía editar `config.yaml`
  a mano (en modo servicio, dentro de `ProgramData`). Ahora está en Configuración,
  como campo de contraseña. El Token **nunca se envía al navegador**: `GET
  /api/config` devuelve solo `tokenSet` y los últimos 4 caracteres, y dejar el
  campo vacío al guardar significa "no lo cambies".
- Nuevo `POST /api/unregister` y botón **"Desvincular este equipo"**: borra Token y
  URL y vuelve a la pantalla de registro. Antes eso solo se conseguía por
  accidente, vaciando el campo URL y guardando. No toca las impresoras.
- Los cambios de conexión (URL o Token) desde Configuración reconectan el agente
  al momento, reutilizando el mismo mecanismo que el registro. El resto de ajustes
  se aplican en vivo sin reiniciar.
- `AgentVersion` pasa de `const "1.0.0"` a `var "0.0.0-dev"`, inyectable con
  `-ldflags`. El valor por defecto delata los binarios compilados fuera del
  build de release en vez de fingir una versión válida.
- El servicio de Windows se crea con **arranque automático retrasado**,
  dependencia del **spooler** y **acciones de recuperación** (reintento a los 5s,
  15s y luego cada minuto, incluidas las salidas no-crash). Antes, un fallo
  dejaba el POS sin imprimir hasta un reinicio manual.
- `scripts/windows-install.ps1` deja de copiar ficheros a mano y pasa a ser el
  desplegador desatendido sobre el instalador. La versión anterior copiaba solo
  `tera-agent.exe`, lo que **dejaba la impresión de PDF rota** en cualquier
  equipo sin poppler en el `PATH`.

### Removed
- **Ajuste de calidad (DPI) de las impresoras de hoja**, en el panel (por
  impresora y por defecto) y en `config.yaml` (`render_dpi`). Nunca tuvo efecto:
  el modo imagen sube siempre el raster a 600 DPI / 4960 dots y el vectorial
  imprime a la resolución de la impresora. Los `config.yaml` con `render_dpi`
  siguen cargando y el campo desaparece al guardar. `ports.Config.PageTuning`
  pasa a `PageMargin`, que resuelve solo el margen.
- Tres interruptores del panel que no hacían nada: "Reconectar automáticamente",
  "Iniciar con Windows" y "Minimizar a la bandeja". Solo cambiaban un color — no
  los leía `saveConfig()`, no existían en `/api/config` ni en `ports.Config`. En su
  lugar, Configuración muestra el **modo de arranque** como texto de solo lectura,
  porque quién arranca el Agent lo decide el instalador, no la configuración.
- El handler global `.toggle → classList.toggle('on')`, que además de sobrar era
  una trampa: sobrescribía el `onclick` de cualquier `.toggle` del DOM, así que si
  hubiera llegado a ejecutarse tras renderizar la lista de impresoras habría
  dejado el interruptor de "activar impresora" siendo solo un cambio de color.
- `ProfileCache.Forget`: se quedó sin uso cuando la caché pasó a guardar solo
  perfiles del Backend. El único que lo llamaba era el panel al cambiar el tipo de
  impresora, y eso tiraba el perfil del ERP.

### Fixed
- **`tera-agent register --scope service` no llegaba a reiniciar el servicio.**
  `service stop` volvía en cuanto el SCM aceptaba la orden, con el servicio aún
  parándose, y el arranque siguiente fallaba con "ya se está ejecutando una
  instancia de este servicio": el Token quedaba guardado pero el servicio seguía
  con la configuración anterior hasta que alguien lo reiniciaba a mano. Ahora
  `stop` espera a que esté parado de verdad (30 s como máximo) y parar un
  servicio ya parado no es un error. Comprobado en la VM Windows 11.
- **"Probar" (bandeja y panel) y `POST /print` ya no pisan el perfil del ERP.**
  Instalaban un perfil ESC/POS en la caché de perfiles compartida: una láser con
  perfil `pdf` del ERP pasaba a recibir ESC/POS crudo en los trabajos del ERP hasta
  la siguiente sincronización, y una térmica de 58 mm quedaba a 576 dots. Ahora
  estas acciones leen el perfil con `ProfileOr` y, si no hay, usan uno por defecto
  solo para ese trabajo (`Engine.PrintWithProfile`), normalizado igual que uno del
  ERP. La caché solo guarda perfiles del Backend.
- "Probar" en la bandeja se comporta como el del panel (`ui.PrintTestPage`): en
  una láser imprime la página PDF de prueba en vez de un ticket ESC/POS.
- Cambiar el tipo de una impresora en el panel ya no olvida su perfil: solo podía
  tirar el del ERP y dejar sus trabajos sin perfil. El tipo nuevo aplica al
  instante a las impresoras sin perfil del ERP.
- Carrera de datos entre el panel y los trabajos en curso: el panel modificaba
  `Config.Printers` en su sitio mientras los resolutores del motor lo leían
  (detectado con `go test -race`).
- `windows-verify.ps1` daba un falso fallo de "reintentos del servicio" en
  Windows en español (buscaba `RESTART` en la salida traducida de `sc.exe`); ahora
  lee `FailureActions` del registro.
- El instalador empaquetaba como licencia de Poppler el aviso de poppler-data;
  ahora lleva la GPL v2 (`COPYING.gpl2`).
- **Una ventana de consola negra parpadeaba en cada impresión de PDF.**
  `pdftoppm.exe` es una aplicación de consola; cuando la lanza el binario de la
  bandeja (`-H=windowsgui`, sin consola propia), Windows le crea una ventana
  nueva. Ahora se lanza con `CREATE_NO_WINDOW`. No afecta a la captura de
  stderr, así que los errores de poppler se siguen recogiendo igual.
- **La impresión de PDF fallaba con `0xc0000135` en equipos de cliente.** El bundle
  llevaba las 26 DLLs de poppler pero **no el runtime de Visual C++**
  (`msvcp140.dll`, `vcruntime140.dll`, `vcruntime140_1.dll`), que importan
  `pdftoppm.exe` y varias DLLs del propio poppler (`cairo`, `Lerc`, `expat`…). En
  una máquina de desarrollo existen en System32 porque el VC++ Redistributable está
  instalado; en un cliente limpio no, y el proceso muere con
  `STATUS_DLL_NOT_FOUND` sin ejecutar nada. Ahora se despliegan **app-local** junto
  a `pdftoppm.exe` (el cargador de Windows busca primero ahí), así que no hay que
  instalar el Redistributable ni pedir reinicio. El build falla si no las encuentra.
- El error de `pdftoppm` para `0xC0000135` ahora explica la causa y la acción, en vez
  de propagar `exit status 0xc0000135` con stderr vacío — el proceso no llega a
  arrancar, así que no hay nada que poppler pueda decir por su cuenta.
- **El instalador generaba una configuración que impedía arrancar.** La plantilla
  escribía las rutas entre comillas **dobles**, y en YAML las comillas dobles
  interpretan escapes: `"C:\ProgramData\TeraAgent"` contiene `\P` y `\T`, que no son
  escapes válidos. El agente moría con `found unknown escape character` antes de
  levantar nada. Solo se veía en instalaciones **nuevas** — donde ya existía un
  `config.yaml` previo, el instalador lo respeta y el fallo quedaba oculto. Ahora
  se usan comillas simples (contenido literal) mediante una función `YamlStr`.
- **Un fallo de arranque era completamente invisible.** `main` escribía el error en
  `os.Stderr` y hacía `os.Exit(1)`; en el binario de la bandeja (`-H=windowsgui`) no
  hay consola, así que el síntoma para el cliente era "se instala pero nunca abre,
  no sale nada", imposible de diagnosticar en remoto. Ahora `fatalStartup` deja el
  error en `%ProgramData%\TeraAgent\startup-error.log` **y** muestra un diálogo
  nativo (omitido bajo el SCM, donde la sesión 0 no puede mostrar UI).
- **El Agent nunca conseguía conectar: HTTP 403 en el upgrade WebSocket.** El dial
  se hacía sin cabeceras, y Django Channels —envuelto en
  `AllowedHostsOriginValidator`— rechaza con 403 cualquier upgrade sin `Origin`,
  antes incluso de resolver la ruta. El Agent quedaba en bucle
  `CONNECTING → DISCONNECTED → RECONNECTING` sin llegar a autenticarse nunca, así
  que el token no tenía nada que ver. Ahora se envía `Origin` derivado del propio
  host del Backend (`wss://host/…` → `https://host`). El contrato no cambia: el
  Token sigue viajando solo en el mensaje `authenticate`.
- **El log en fichero no escribía nada en el binario de la bandeja.** El logger usa
  `io.MultiWriter(os.Stderr, fichero)`, y en un ejecutable compilado con
  `-H=windowsgui` no hay consola: `os.Stderr` es un handle inválido y cada
  escritura falla. `io.MultiWriter` aborta en el primer error, así que **el fichero
  quedaba a 0 bytes** — justo en el binario que corre en los equipos de los
  clientes, dejando el soporte a ciegas. Ahora stderr va envuelto en un writer
  best-effort cuyos fallos no cortan la cadena.
- **`POST /api/config` borraba la URL del Backend.** Los campos se asignaban como
  valores planos, así que guardar el formulario sin `url` (o con el campo vacío)
  dejaba `BackendURL: ""` y desregistraba el equipo en silencio. Ahora todos los
  campos son opcionales: lo que el panel no envía, no se toca. Con test de
  regresión.
- La URL del Backend no se validaba: un valor con una errata se guardaba tal cual y
  el agente entraba en un bucle de reconexión sin explicar por qué. Ahora se exige
  esquema `wss://` (o `ws://` en desarrollo) y host, tanto al registrar como al
  guardar.
- **El panel decía "Los cambios se aplicaron" siempre**, incluso cuando el POST
  fallaba: `saveConfig()` no miraba la respuesta. Ahora muestra el mensaje real del
  agente, y distingue entre aplicado en vivo y reconectando.
- `registrar()` usaba el helper `api()`, que se traga los errores y activa el modo
  demo: un registro rechazado mostraba "Registrado (demo)". Ahora comprueba la
  respuesta y muestra el error del servidor.
- `restartSelf` no detectaba el SCM de Windows: tras registrar el equipo desde el
  panel, un Agent corriendo como servicio lanzaba un **proceso duplicado** en vez
  de dejar que el gestor lo reiniciase. Ahora sale con código distinto de 0, que
  es lo que el SCM interpreta como fallo para aplicar la recuperación.

### Added (histórico)
- Scaffold inicial con Clean Architecture (domain / app / adapters / infra).
- Máquina de estados del Agent con transiciones validadas y observadores.
- Ports de `printing`, `device` y `comms`; dispatcher y lifecycle con handshake
  de autenticación por Token.
- 10 subagentes de desarrollo en `.claude/agents/`.
- **Impresión silenciosa (Linux/macOS)** mediante CUPS (`lp`/`lpstat`), sin
  dependencias externas. Salida de CUPS forzada a `LC_ALL=C` para parseo de
  estado independiente del idioma.
- CLI con subcomandos `run`, `printers` y `print` para probar la impresión
  silenciosa sin backend.
- Stub de impresión para plataformas sin backend (mantiene el cross-compile).
- `Makefile` con build, test, race, cover y cross-compile.
- **Motor de impresión extensible por pipeline**
  (`PrintJob → Engine → Resolver → Renderer → Encoder → Driver`) con resolución
  por capacidades (sin condicionales por formato). Diseño en
  `docs/PRINT_ENGINE.md`.
- Puertos del dominio: `Renderer`, `Encoder`, `Driver`, `Rasterizer`,
  `Binarizer`, `Resolver`, `ProfileProvider`/`ProfileCache`.
- `Rasterizer` con implementación `PopplerRasterizer` (reemplazable).
- Renderers PDF/imagen/texto; encoders ESC/POS raster y texto; drivers CUPS
  raw/nativo y `file` (dry-run).
- **Binarización** con estrategia enchufable: `Otsu` (por defecto), `Atkinson`,
  `Threshold`. Recomendación técnica en `docs/BINARIZATION.md`.
- `PrinterProfile` propiedad del ERP, cacheado en memoria (`ProfileCache`).
- CLI `print` sobre el motor: PDF 80mm → ESC/POS raster con corte, y `-out` para
  dry-run a archivo.
- Tests del Resolver (composición y pass-through), binarizers (Otsu/Threshold/
  Atkinson) y encoder ESC/POS.

- **CLI multiplataforma** con comandos `printers`, `print`, `raw`, `text`, `run`.
- **Windows**: driver e impresión RAW vía spooler (`winspool`) y enumeración con
  `EnumPrinters`, sin depender del driver gráfico. Fachada `platform` que
  selecciona CUPS (Linux/macOS) o spooler (Windows) por build tags.
- ESC/POS: anchos **58mm/80mm**, corte, cajón y **densidad** configurable
  (sesgo de umbral Otsu).
- **config.yaml** (`gopkg.in/yaml.v3`) con `server`/`agent`/`printer`/`heartbeat`/`log`.
- `Transport` + **LocalTransport**: modo local sin backend; seam listo para el
  futuro `WebSocketTransport`.
- **Web service HTTP local** (`tera-agent serve`) para integración con Django:
  `GET /health`, `GET /printers`, `POST /print` (multipart o JSON base64). Reusa
  el motor de impresión; opcional bearer token; `run` lo expone si `http.addr`
  está configurado. Guía en `docs/HTTP_API.md`.
- **Servicio de Windows** (`service install|uninstall|start|stop`) con
  `x/sys/windows/svc` e instalador `scripts/windows-install.ps1`.
- **Transporte WebSocket funcional (Fase 4)** con `gorilla/websocket`: el Agent
  conecta, hace el handshake (hello/authenticate/authenticated), se registra,
  reporta capabilities, sincroniza perfiles (`profiles_sync`/`profiles_ack`),
  envía heartbeats y **recibe/ejecuta PrintJobs** (`job` → `job_received` →
  `job_completed`/`job_failed`) con idempotencia y **reconexión con backoff**.
  Sin TLS ni validación de Token todavía (fase Security).
- `examples/mock-server`: servidor WebSocket de referencia (lado Django) para
  probar el agente end-to-end.
- Test de integración del ciclo completo de sesión WebSocket.
- Diseño del protocolo WebSocket (Fase 4) en `docs/protocol/`.
- Manual de instalación (`docs/INSTALL.md`) y estado del MVP con pruebas y
  problemas conocidos (`docs/MVP.md`).

### Changed
- La impresión ya no usa un `Document`/`Port` plano: pasa por el motor. Los
  paquetes de `adapters/printing` se reorganizan en subpaquetes
  (rasterizer/renderer/encoder/binarizer/driver/discovery/profile).
- Configuración migrada de JSON a **YAML** (`config.yaml`).

### Added (panel de escritorio)
- **UI web de escritorio** (`tera-agent ui`): SPA moderna embebida (Go `embed`)
  servida en local, con estado en vivo, empresa/sede, impresoras, historial,
  configuración y logs; temas claro/oscuro; actualización en tiempo real por
  WebSocket local (`/ws/ui`). API JSON en el agente
  (`/api/status|printers|test-print|config|register|open-data`). Reusa el motor
  de impresión y la máquina de estados; abre el navegador automáticamente.

### Added (robustez y UI)
- **Dedupe persistente de trabajos** y **buffer offline de resultados**
  (`adapters/store`, fichero JSON): no reimprime un job reenviado tras reinicio y
  reenvía `job_completed`/`job_failed` al reconectar.
- **Log rotativo a fichero** (`adapters/logger`, size-based, stdlib) además de stderr.
- **wss/TLS** en el transporte WebSocket (+ `insecure_skip_verify` para dev).
- **Icono de bandeja (tray) en Windows** (`run --tray`, `fyne.io/systray`, Go puro):
  estado, abrir carpeta de datos, salir.
- config.yaml: `server.insecure_skip_verify`, `data_dir`, `log.file/max_size_mb/max_backups`.

### Fixed
- `.gitignore` usaba el patrón `tera-agent` sin anclar, que ignoraba el
  directorio `cmd/tera-agent/`: el entrypoint `main.go` no estaba versionado.
  Patrones anclados a la raíz; `main.go` ahora en el repositorio.
- **Impresión térmica: corte tardío / papel en blanco**. El encoder ESC/POS ahora
  **recorta las filas en blanco finales** del raster y añade una pequeña
  alimentación (`ESC J`) antes del corte, para cortar justo tras el contenido y no
  desperdiciar papel (reportado en Windows con la POS-80C).
- Añadido el comando `version`.
- `examples/mock-server`: mantiene la conexión abierta tras el resultado (no
  reenvía el mismo job en cada reconexión).
