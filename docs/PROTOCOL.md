# Protocolo — Tera Agent ↔ Backend

> Borrador inicial. Propiedad: Communication Engineer (transporte) y Software
> Architect (contratos). Los tipos viven en
> [`internal/domain/comms/comms.go`](../internal/domain/comms/comms.go).

## Transporte

- WebSocket sobre **TLS**. La conexión la inicia **siempre** el Agent.
- Framing: cada mensaje es un `Envelope { type, data }`, donde `data` es el
  cuerpo específico del tipo (serializado, p. ej. JSON).
- Heartbeat con el intervalo entregado por el Backend en el handshake.
- Reconexión con backoff; compresión y timeouts gestionados por el adapter.

## Handshake de registro / autenticación

```
Agent                                  Backend
  │  AUTH { token }                       │
  │──────────────────────────────────────▶│   valida el Token
  │                                        │
  │            AUTH_OK { uuid, empresa,    │   (Token válido)
  │            sucursal, equipo, ... }     │
  │◀───────────────────────────────────────│
  │                                        │
  │            AUTH_ERR                     │   (Token inválido → cerrar)
  │◀───────────────────────────────────────│
```

- El Agent envía **solo** el Token emitido por el Backend. Nunca genera ni
  renueva Tokens; no conoce usuarios ni credenciales.
- Con `AUTH_OK`, el Agent adopta la identidad recibida (UUID, Empresa, Sucursal,
  Equipo) y pasa a `CONNECTED`.
- Con `AUTH_ERR` (o cierre), el Agent transita a `ERROR` y reporta.

## Mensajes

| Tipo | Dirección | Cuerpo | Descripción |
|------|-----------|--------|-------------|
| `AUTH` | Agent → Backend | `{ token }` | Presenta el Token |
| `AUTH_OK` | Backend → Agent | `Identity` + config inicial | Registro válido |
| `AUTH_ERR` | Backend → Agent | — | Token inválido |
| `HEARTBEAT` | Agent → Backend | — | Liveness |
| `JOB` | Backend → Agent | `Job { id, kind, payload }` | Trabajo a ejecutar |
| `RESULT` | Agent → Backend | resultado del job | Outcome |

## Jobs

El Backend es el único que despacha `JOB`. El `Dispatcher` del núcleo enruta cada
job al `Handler` registrado según su `Kind` (`PRINT`, `DEVICE`). El `payload` es
opaco a nivel de transporte; lo interpreta el handler del tipo correspondiente.

## Seguridad (revisión del Security Engineer)

- El Token nunca aparece en logs ni en query strings.
- Verificación de certificados del servidor en TLS.
- Validación de todos los mensajes entrantes antes de procesarlos.
