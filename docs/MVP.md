# Tera Agent — Estado del MVP funcional

Resumen de lo entregado, pruebas realizadas, decisiones técnicas y problemas
conocidos. (Prioridad del MVP: **funcionar > documentar**.)

## 1. Qué hace hoy

| Comando | Función | Linux/macOS | Windows |
|---|---|---|---|
| `printers` | Lista impresoras (nombre/sistema/estado/tipo) | ✅ CUPS | ✅ spooler |
| `print` | PDF/imagen/texto → **ESC/POS raster** (Otsu) | ✅ | ✅ (PDF requiere poppler) |
| `raw` | Envía **bytes sin modificar** | ✅ `lp -o raw` | ✅ winspool RAW |
| `text` | Texto → ESC/POS con corte | ✅ | ✅ |
| `run` | Agente residente en **modo local** | ✅ | ✅ |

Capacidades de impresión térmica: **GS v 0 raster**, anchos **58mm (384 dots)** y
**80mm (576 dots)**, **corte** (`GS V 0`), **cajón** (`ESC p`), **densidad**
configurable (1..5) y **binarización Otsu** por defecto (sin interpolación ni
antialiasing; ver [BINARIZATION.md](BINARIZATION.md)).

## 2. Pruebas realizadas

Verificado con Go 1.23.5 (build + `go test -race`):

- `go build ./...` en **Linux, Windows (amd64) y macOS (arm64)** → OK.
- `go vet ./...` y `go test -race ./...` → OK (dominio FSM, resolver, binarizers,
  encoder ESC/POS, loader YAML).
- **Pipeline PDF→ESC/POS** (dry-run `--out`, sin imprimir):
  - 80mm → ancho **576 px**, 55.133 bytes; 58mm → **384 px**, 24.517 bytes.
  - Cabecera `ESC @` + `GS v 0` (banda 128 filas) + corte `GS V 0`.
  - Bitmap reconstruido ≈ **3% de negro** (texto real, no basura).
  - Densidad 1 vs 5 → 2,97 % vs 3,33 % de negro (el sesgo de umbral funciona).
- `text --out` → 18 bytes: `ESC @` + "Hola mundo" + corte.
- `raw` → enruta al driver; con impresora inexistente devuelve el error de `lp`
  (plumbing correcto), sin imprimir.
- Impresión física real: verificada en sesión anterior con una **XP-80** (80mm),
  encolando el trabajo en CUPS con exit 0.

> El entorno de CI actual **no tiene impresoras** configuradas, por lo que
> `printers` responde "No printers found" (comportamiento correcto). El
> descubrimiento se validó previamente con la XP-80.

## 3. Decisiones técnicas (tomadas como arquitecto)

- **YAML de config**: `gopkg.in/yaml.v3` (MIT, maduro) en vez de parser propio,
  para no retrasar el MVP.
- **Windows sin driver gráfico**: impresión **RAW** directa al spooler
  (`winspool.drv`: OpenPrinter/StartDocPrinter/WritePrinter). El PDF en Windows
  se rasteriza igual que en Linux (poppler) → ESC/POS.
- **Densidad** = sesgo del umbral Otsu (software), independiente del modelo; los
  comandos de calor/heat propios de cada impresora quedan fuera del MVP.
- **Selección de plataforma** mediante build tags (`platform_unix.go` /
  `platform_windows.go`), sin `if runtime.GOOS` disperso.
- **Modo local**: `run` sin `server.url` funciona offline; `LocalTransport`
  mantiene el seam para el futuro `WebSocketTransport` (Fase 4 de código).

## 4. Problemas conocidos / limitaciones

1. **Windows: sin runtime test**. El driver winspool **compila** (cross-compile
   amd64/386/arm64 OK) pero **no se ha probado en una máquina Windows real** en
   este entorno (no hay Windows ni `wine`). Guía de validación lista:
   [WINDOWS_TEST.md](WINDOWS_TEST.md) (script `scripts/windows-test.ps1`).
2. **Estado de impresora en Windows**: implementado vía `GetPrinter` nivel 6
   (READY/BUSY/OFFLINE/ERROR); **pendiente de confirmar** en Windows real.
3. **PDF en Windows** requiere `pdftoppm.exe` en el `PATH` (poppler). Sin él,
   `print` de PDF falla; `raw`/`text` funcionan igual.
4. **Escalado de imágenes** (`image/png`, `image/jpeg`): el renderer de imagen no
   reescala al ancho del papel (usa el tamaño original). El PDF sí se escala al
   ancho nativo. Reescalado de imágenes: pendiente.
5. **`run` es headless**: sin icono de bandeja (tray) todavía; eso pertenece a la
   fase de UI. El binario corre como proceso residente / servicio.
6. **Servicio de Windows**: implementado (`service install|uninstall|start|stop`
   con `x/sys/windows/svc`) + instalador `scripts/windows-install.ps1`; **pendiente
   de probar en Windows real**. En Linux/macOS no hay servicio (usar systemd/launchd).
7. **Backend WebSocket**: **implementado** (Fase 4). `run` con `server.url`
   conecta, autentica (Token, **sin TLS aún**), registra, sincroniza perfiles,
   heartbeat, recibe PrintJobs → imprime → responde, y reconecta con backoff.
   Verificado end-to-end con `examples/mock-server`. **Falta** TLS/wss y la
   revisión de seguridad (expiración de Token, replay, rate limit) — Fase Security.
   Django debe implementar el lado servidor ([protocol/](protocol/README.md)).

## 5. Siguientes pasos sugeridos

- Probar en una máquina Windows real (RAW + enumeración) y ajustar estado.
- Implementar el lado servidor del protocolo en Django (base: `examples/mock-server`).
- Seguridad: TLS/wss, validación de Token y límites (Fase Security).
- Reescalado de imágenes y, más adelante, tray (UI) e instaladores (DevOps).
