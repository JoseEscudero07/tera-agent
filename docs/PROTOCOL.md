# Protocolo — Tera Agent ↔ Backend

La especificación detallada del protocolo WebSocket (Fase 4) vive en
**[docs/protocol/](protocol/README.md)**:

- [websocket.md](protocol/websocket.md) — transporte, TLS, secuencia, reconexión, versionado.
- [authentication.md](protocol/authentication.md) — handshake, Token, registro, capabilities.
- [messages.md](protocol/messages.md) — envelope, catálogo de mensajes, versionado.
- [jobs.md](protocol/jobs.md) — ciclo de vida de jobs y payload de impresión.
- [events.md](protocol/events.md) — heartbeat, config, sincronización de perfiles, logs.
- [errors.md](protocol/errors.md) — modelo y catálogo de errores.

## Resumen

Toda comunicación la inicia el Agent mediante WebSocket seguro (WSS/TLS) al
Backend. `Angular → Django → WebSocket → Agent`. Nunca hay comunicación directa
Angular↔Agent. El Backend es el único que envía trabajos. La autenticación es por
**Token** emitido por el Backend; el Agent solo lo transmite (nunca lo genera ni
renueva).
