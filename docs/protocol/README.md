# Protocolo Tera Agent ↔ Django — Fase 4 (Communication)

> **Estado: propuesta de diseño, pendiente de aprobación del Software Architect.**
> No implementar código hasta aprobar. Owner: Communication Engineer.

Especificación completa del canal WebSocket persistente y seguro entre el ERP
(Django) y Tera Agent. `Angular → Django API → WebSocket Server → Tera Agent →
Print Engine → Hardware`. El Agent siempre inicia la conexión.

## Documentos

| Doc | Contenido |
|---|---|
| [websocket.md](websocket.md) | Transporte, TLS, secuencia de conexión, keepalive, reconexión, offline, versionado, close codes |
| [authentication.md](authentication.md) | Handshake `hello`/`authenticate`, identidad, registro y capabilities |
| [messages.md](messages.md) | Envelope, catálogo de mensajes, convenciones y versionado |
| [jobs.md](jobs.md) | Ciclo de vida de jobs, payload de `print`, acuses, idempotencia |
| [events.md](events.md) | Heartbeat, config, sincronización de perfiles, eventos y logs |
| [errors.md](errors.md) | Modelo de errores, catálogo de códigos, mapa a la FSM |

## Alineación con el resto del sistema

- Las fases del protocolo mapean 1:1 con la **máquina de estados** del Agent
  (`internal/domain/agent`).
- El payload de job `print` coincide con el contrato que ya consume el **motor de
  impresión** (`internal/app/print`), ver [../PRINT_ENGINE.md](../PRINT_ENGINE.md).
- Los `PrinterProfile` sincronizados alimentan el `ProfileCache` en memoria; el
  Agent nunca los crea ni persiste como lógica de negocio.

## Entregables de implementación (tras aprobación)

WebSocket Client · Authentication Handler · Registration Handler ·
Heartbeat Service · Job Receiver · Job Response Sender.

## Criterio de aceptación de la fase

1. Django acepta la conexión del Agent. 2. El Agent se autentica con Token.
3. Se registra. 4. El ERP conoce sus impresoras. 5. El ERP envía un PrintJob.
6. El Agent lo ejecuta. 7. Devuelve resultado. 8. La conexión sobrevive
desconexiones temporales.
