# Tera Agent

Agente empresarial multiplataforma (Go) que conecta el equipo local con el ERP
para **impresión** y **hardware de punto de venta** (scanner, cajón, balanza,
display, etc.). Se comunica con el Backend (Django) mediante una conexión
**WebSocket segura** iniciada siempre por el Agent.

> Estado: **scaffold inicial**. La arquitectura y la máquina de estados están
> establecidas; los adapters de comunicación, impresión, dispositivos y UI son
> puntos de extensión pendientes, cada uno con su agente responsable.

## Principios

Clean Architecture · SOLID · KISS · DRY · YAGNI · Composition over Inheritance ·
Dependency Injection. La dirección de dependencias apunta **hacia el dominio**:
el dominio no conoce frameworks, WebSocket, impresoras ni UI.

## Estructura

```
cmd/tera-agent/            Entry point (composición + señales del SO)
internal/
  domain/                  Reglas de empresa (entidades + ports). Sin frameworks.
    agent/                 Máquina de estados del Agent
    job/                   Unidad de trabajo despachada por el Backend
    printing/  device/     Entidades y ports de impresión / hardware
    comms/                 Contratos de protocolo y port Transport
  app/                     Casos de uso / orquestación
    ports/                 Ports driven (Logger, ConfigStore, Clock, Config)
    lifecycle/             Orquesta startup/handshake/serve/shutdown
    dispatcher/            Enruta jobs a su handler por Kind
  adapters/                Implementaciones (frameworks e infraestructura)
    communication/websocket/   Transport WebSocket (stub → Communication Engineer)
    printing/  device/  ui/    Placeholders por agente responsable
    config/  logger/           ConfigStore por fichero, Logger con slog
  infra/di/                Composition root (único que importa adapters)
```

## Máquina de estados

`STARTING → CONNECTING → AUTHENTICATING → REGISTERING → CONNECTED`, con
`DISCONNECTED / RECONNECTING / OFFLINE / ERROR` y terminal `SHUTTING_DOWN`.
Las transiciones válidas están declaradas y verificadas en
[`internal/domain/agent/state.go`](internal/domain/agent/state.go). La UI se
suscribe como `Observer` y refleja siempre el estado actual.

## Autenticación

La autenticación pertenece **completamente al Backend**. El Agent solo transmite
el **Token** emitido por el Backend; nunca lo genera, renueva, ni conoce
usuarios o credenciales. Tras un Token válido, el Backend responde con UUID,
Empresa, Sucursal, Equipo, intervalo de Heartbeat y configuración inicial.

## Instalación de Go

Requiere **Go 1.23+**.

```bash
# Linux (amd64) — ejemplo
curl -sSL -o go.tgz https://go.dev/dl/go1.23.5.linux-amd64.tar.gz
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go.tgz
export PATH=$PATH:/usr/local/go/bin        # añádelo a tu ~/.bashrc
go version
```

(macOS/Windows: instaladores en <https://go.dev/dl/>.)

## Compilar y probar

```bash
make build      # o: go build ./...
make test       # o: go test ./...
make race       # go test -race ./...
```

## Instalación y ejecutables

Ver el **[manual de instalación](docs/INSTALL.md)** (Linux y Windows) y el
**[estado del MVP](docs/MVP.md)** (pruebas realizadas y problemas conocidos).

Genera los ejecutables (`tera-agent-linux`, `tera-agent.exe`):

```bash
make cross     # -> dist/
```

## Comandos

```bash
tera-agent printers                                   # listar impresoras (nombre/sistema/estado/tipo)
tera-agent print --printer XP-80 --file factura.pdf   # PDF/imagen/texto -> ESC/POS
tera-agent raw   --printer XP-80 --file ticket.bin    # bytes sin modificar
tera-agent text  --printer XP-80 --text "Hola mundo"  # texto -> ESC/POS
tera-agent run   [--config config.yaml]               # agente residente (modo local)
tera-agent ui    [--config config.yaml]               # agente + panel de escritorio (web UI)
tera-agent serve [--addr 127.0.0.1:9100]              # web service HTTP (Django manda PDFs)
```

**Panel de escritorio** (`ui`): abre una ventana web con estado en vivo, empresa/sede,
impresoras, historial, configuración y logs (temas claro/oscuro). Sirve una SPA
embebida en `127.0.0.1:9180` y refleja el estado del agente en tiempo real.

Flags de impresión: `--paper 58|80`, `--width <dots>`, `--density 1..5`,
`--cut`, `--drawer`, `--out FILE` (dry-run sin imprimir).

**Integración con Django** vía HTTP: ver [docs/HTTP_API.md](docs/HTTP_API.md) —
`POST /print` con el PDF y el agente lo rasteriza a ESC/POS 80mm e imprime.

## Motor de impresión

La impresión pasa por un **motor extensible por pipeline**
(`PrintJob → Engine → Resolver → Renderer → Encoder → Driver`) que soporta
distintos formatos de entrada e impresoras **sin tocar el Core**. El `Resolver`
elige el pipeline por **capacidades** de la impresora (sin condicionales por
formato). Diseño en [docs/PRINT_ENGINE.md](docs/PRINT_ENGINE.md); estrategia de
binarización 1-bit en [docs/BINARIZATION.md](docs/BINARIZATION.md).

**Imprimir un PDF de 80mm (FPDF) como ticket térmico** — el motor rasteriza el
PDF (`pdftoppm`), lo binariza con **Otsu** al ancho nativo y lo envía como
**ESC/POS raster** con corte:

```bash
# Requiere CUPS + poppler-utils (pdftoppm)
./tera-agent print -printer XP-80 -file recibo.pdf            # PDF -> ESC/POS raster -> imprime
./tera-agent print -printer XP-80 -file recibo.pdf -width 576 -cut
```

**Dry-run (sin gastar papel):** vuelca los bytes ESC/POS a un archivo para
inspeccionarlos:

```bash
./tera-agent print -printer XP-80 -file recibo.pdf -out ticket.bin
```

Otros formatos y destinos (mismo comando, el Resolver elige el pipeline):

```bash
echo "Hola" | ./tera-agent print -printer XP-80 -format text   # texto -> ESC/POS
./tera-agent print -printer OFICINA -file doc.pdf -device pdf   # PDF nativo (láser) sin rasterizar
./tera-agent printers                                          # listar impresoras y estado
```

> **Genera el PDF a 80mm en tu backend** (FPDF): `new FPDF('P','mm', array(80,$alto))`.
> Mandar un PDF “tal cual” a una cola térmica *raw* imprime código basura; el motor
> lo resuelve rasterizando a ESC/POS según el perfil de la impresora.

> Windows: el driver nativo (Windows Print API) está pendiente. En Windows los
> comandos `lp`/`pdftoppm` no existen y el driver reporta error en tiempo de
> ejecución (el binario compila igualmente).

## Ejecutar el agente

```bash
./tera-agent run                       # modo local (sin backend): imprime vía CLI
# conectado al ERP por WebSocket:
cp config.example.yaml config.yaml     # server.url: "ws://host:8765"
./tera-agent run --config config.yaml
tera-agent.exe run --tray              # Windows: con icono en la bandeja
```

Con `server.url` el Agent **conecta por WebSocket** al backend, se autentica con
el Token, se registra, sincroniza los perfiles de impresora, manda heartbeats y
**recibe PrintJobs** que imprime (respondiendo `job_completed`/`job_failed`), con
reconexión automática. Sin `server.url` corre en **modo local** (offline).

Protocolo en [docs/protocol/](docs/protocol/README.md). Para probar sin Django,
usa el servidor de referencia:

```bash
go run ./examples/mock-server --pdf factura.pdf --printer KL200
# en otra terminal, con server.url: "ws://127.0.0.1:8765"
./tera-agent run --config config.yaml
```

> Aún **sin seguridad** (ws://, sin validación estricta de Token). TLS/wss y la
> revisión de seguridad son la fase siguiente.

## Equipo de agentes

El desarrollo se organiza como un equipo de agentes especializados definidos en
[`.claude/agents/`](.claude/agents/), cada uno con una única responsabilidad y
límites estrictos. Ver [`CLAUDE.md`](CLAUDE.md) para las reglas y el flujo de
trabajo, y [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) para el diseño.
