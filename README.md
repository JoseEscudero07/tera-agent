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

## Probar la impresión silenciosa (Linux/macOS)

Requiere **CUPS** con al menos una impresora configurada. La impresión es
silenciosa (sin diálogo) usando `lp`/`lpstat`.

```bash
# 1) Listar impresoras y su estado
./tera-agent printers

# 2) Imprimir un archivo o texto por stdin (sustituye NOMBRE por tu impresora)
echo "Ticket de prueba - Tera Agent" | ./tera-agent print -printer NOMBRE

# 3) Impresión RAW para tickets ESC/POS o etiquetas ZPL (sin driver)
./tera-agent print -printer NOMBRE -raw -file ticket.escpos
```

`print` devuelve exit 0 y registra el `request id` de CUPS al encolar el trabajo.
Si no tienes una impresora física, puedes crear una virtual PDF instalando
`cups-pdf` (`sudo apt install printer-driver-cups-pdf`).

> Windows: la impresión silenciosa (Windows Print API) está pendiente; el
> binario compila con un stub que reporta la plataforma como no soportada.

## Ejecutar el agente

```bash
cp config.example.json config.json   # edita backend_url y token
./tera-agent run -config config.json
```

> El transporte WebSocket es aún un stub; `run` avanzará por la máquina de
> estados hasta `ERROR` al no poder conectar con un backend real.

## Equipo de agentes

El desarrollo se organiza como un equipo de agentes especializados definidos en
[`.claude/agents/`](.claude/agents/), cada uno con una única responsabilidad y
límites estrictos. Ver [`CLAUDE.md`](CLAUDE.md) para las reglas y el flujo de
trabajo, y [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) para el diseño.
