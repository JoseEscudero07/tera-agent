# CLAUDE.md — Tera Agent

Guía para agentes de Claude Code trabajando en este repositorio.

## Qué es

Agente empresarial en **Go**, multiplataforma, que conecta el equipo local con un
ERP (Backend Django) mediante **WebSocket seguro** para **impresión** y
**hardware de POS**. La comunicación la inicia siempre el Agent.

## Filosofía (obligatoria)

Pensar primero como arquitecto, después como desarrollador. Antes de escribir
código, preguntarse: ¿escala? ¿respeta Clean Architecture y SOLID? ¿es
mantenible y extensible sin romper lo existente? ¿añade complejidad innecesaria?
Si la respuesta es negativa, detenerse y proponer algo mejor. **Nunca** soluciones
rápidas solo para cumplir una tarea.

## Principios

Clean Architecture · SOLID · KISS · DRY · YAGNI · Composition over Inheritance ·
Dependency Injection. Sin dependencias circulares. Sin acoplamiento entre
módulos. Una única responsabilidad por módulo. Interfaces pequeñas.
`context.Context` para cancelación/timeouts. Sin variables globales. Archivos y
funciones pequeños. Preferir la stdlib de Go; dependencias externas solo si están
activas, con licencia MIT/BSD/Apache y revisadas por el Security Engineer.

## Regla de dependencia

`adapters → app → domain`. Las dependencias apuntan **hacia el dominio**. El
dominio no importa frameworks, WebSocket, impresoras ni UI. Solo `infra/di`
importa `adapters`. Ver [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Autenticación

Pertenece al Backend. El Agent solo transmite el **Token** emitido por el
Backend; nunca lo genera, renueva, ni conoce usuarios/credenciales. Token y UUID
nunca en logs ni en URLs.

## Equipo de agentes y límites

Definidos en [.claude/agents/](.claude/agents/). Cada agente tiene una única
responsabilidad y **no modifica módulos de otro** sin documentar el motivo.

- **software-architect** — diseño, interfaces, revisión. No implementa.
- **go-core-engineer** — lifecycle, config, DI, logger, dispatcher, jobs, FSM.
- **communication-engineer** — WebSocket, heartbeat, reconexión, Token handshake.
- **printing-engineer** — CUPS, Windows, ESC/POS, Zebra, PDF, descubrimiento.
- **device-engineer** — scanner, cajón, balanza, display, serial, USB HID, RFID.
- **ui-engineer** — tray, estado, config, logs, lista de impresoras.
- **qa-engineer** — tests, benchmarks, race, cobertura, performance. No implementa producto.
- **security-engineer** — TLS, certificados, Token, secretos, revisión.
- **devops-engineer** — build, cross-compile, CI/CD, releases, instaladores, auto-update.
- **technical-writer** — README, ARCHITECTURE, API, PROTOCOL, CHANGELOG, etc.

## Flujo de trabajo

Ningún agente programa de inmediato. Para todo cambio: **1) analizar la tarea,
2) analizar el impacto, 3) revisar la arquitectura, 4) proponer la solución,
5) esperar aprobación, 6) implementar.**

Orden: Architect → Go Core → Communication → Printing → Device (si aplica) → UI →
QA → Security → Architect → Merge.

## Comandos

```bash
go build ./...
go test ./...
go test -race -cover ./...
go vet ./...
go run ./cmd/tera-agent --config config.json
```
