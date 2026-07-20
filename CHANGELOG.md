# Changelog

Formato basado en [Keep a Changelog](https://keepachangelog.com/es-ES/1.0.0/).

## [Unreleased]

### Added
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
