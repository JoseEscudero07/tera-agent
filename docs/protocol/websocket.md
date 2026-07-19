# Protocolo — Transporte WebSocket

> Fase 4 (Communication). **Propuesta de diseño, pendiente de aprobación.**
> No implementar hasta aprobar. Owner: Communication Engineer.
> Índice: [authentication](authentication.md) · [messages](messages.md) ·
> [jobs](jobs.md) · [events](events.md) · [errors](errors.md).

## 1. Principios

- El **Agent siempre inicia** la conexión. `Angular → Django API → WebSocket
  Server → Tera Agent → Print Engine → Hardware`. Angular nunca habla con el Agent.
- Canal **persistente** y **seguro (WSS/TLS)**. Un único socket por instalación.
- Mensajería **JSON** (frames de texto UTF-8). Un mensaje = un objeto JSON con un
  campo discriminador `type` (ver [messages.md](messages.md)).
- Contenido binario grande (p. ej. PDF de un job) viaja **base64** dentro del
  payload. Optimización futura (frames binarios/chunking) reservada a una versión
  mayor del protocolo.

## 2. Endpoint y TLS

- URL: `wss://<host>/ws/agent/` (configurada en el Agent; `wss://` obligatorio).
- TLS 1.2+ con **verificación de certificado del servidor** (nunca `InsecureSkipVerify`).
- El **Token nunca** viaja en la URL ni en query string; solo en el mensaje
  `authenticate` (ver [authentication.md](authentication.md)).
- Detalle de seguridad revisado por Security Engineer antes de producción.

## 3. Secuencia de conexión (mapeada a la máquina de estados)

La FSM del Agent (`STARTING → CONNECTING → AUTHENTICATING → REGISTERING →
CONNECTED → …`) se corresponde 1:1 con las fases del protocolo:

```
FSM                Protocolo
─────────────────  ─────────────────────────────────────────────
CONNECTING         TCP + TLS + WebSocket upgrade
AUTHENTICATING     → hello        → authenticate    ← authenticated | auth_error
REGISTERING        → register     → capabilities    ← profiles_sync  → profiles_ack  ← config
CONNECTED          ↺ heartbeat    ← job  → job_received → job_completed | job_failed
SHUTTING_DOWN      → bye          (close 1000)
DISCONNECTED/RECONNECTING/OFFLINE   ver §6
```

Secuencia feliz:

```
Agent                                   Server
  │  (TLS + WS upgrade)                    │
  │  hello {versions, machine_id}          │
  │───────────────────────────────────────▶│
  │  authenticate {token}                  │
  │───────────────────────────────────────▶│
  │            authenticated {agent_id,...} │
  │◀────────────────────────────────────────│
  │  register {hostname, os, ip}           │
  │───────────────────────────────────────▶│
  │  capabilities {printers[]}             │
  │───────────────────────────────────────▶│
  │            profiles_sync {profiles[]}   │   (ERP asigna perfiles)
  │◀────────────────────────────────────────│
  │  profiles_ack {version}                │
  │───────────────────────────────────────▶│
  │            config {heartbeat_interval}  │
  │◀────────────────────────────────────────│
  │  heartbeat {state, uptime, ...}  ↺      │
  │            job {id, payload}            │
  │◀────────────────────────────────────────│
  │  job_received {id}                     │
  │───────────────────────────────────────▶│
  │  job_completed {id, duration}          │
  │───────────────────────────────────────▶│
```

## 4. Keepalive

- **Nivel transporte**: WebSocket ping/pong (control frames) para detectar
  sockets muertos; intervalo por defecto 20s, timeout de pong 10s.
- **Nivel aplicación**: `heartbeat` con estado del Agent en el intervalo que
  indique `config` (ver [events.md](events.md)). El heartbeat de aplicación es el
  que el ERP usa para “vivo/ocupado/pendientes”, no el ping de transporte.

## 5. Límites y robustez

- Tamaño máximo de mensaje entrante: configurable (por defecto 8 MiB). Mensajes
  mayores → cierre `4009 BAD_MESSAGE`.
- Rate limiting por dirección (servidor) → `4008 RATE_LIMITED`.
- Timeouts: handshake completo debe cerrarse en ≤ 30s o el Agent aborta y reintenta.
- Todo mensaje entrante se valida contra su esquema antes de procesar
  (ver [errors.md](errors.md)); un mensaje inválido no debe tumbar el Agent.

## 6. Reconexión

Disparadores: pérdida de internet, reinicio del servidor, suspensión del equipo,
cambio de red, cierre inesperado del socket.

Estrategia:

- **Backoff exponencial con jitter**: `delay = min(cap, base * 2^n) ± jitter`.
  Por defecto `base=1s`, factor 2, `cap=60s`, jitter ±20%.
- Tras `N` intentos (por defecto sin límite duro) el Agent pasa a **OFFLINE** y
  sigue reintentando al intervalo `cap`.
- El backoff se **reinicia** al alcanzar `CONNECTED`.
- Transiciones FSM: `CONNECTED → DISCONNECTED → RECONNECTING → CONNECTING → …`;
  si agota reintentos razonables → `OFFLINE` (reintento lento).
- Al reconectar se repite **todo** el handshake (hello/auth/register/printer-sync);
  todas esas operaciones son **idempotentes** en el servidor.

### Close codes (rango privado 4000–4999)

| Código | Significado | ¿Reintentar? |
|---|---|---|
| 1000 | Cierre normal (`bye`) | — |
| 1001 | Going away | Sí |
| 1012 | Reinicio del servicio | Sí |
| 4001 | AUTH_FAILED (token inválido) | **No** |
| 4002 | PROTOCOL_UNSUPPORTED | **No** (hasta actualizar) |
| 4003 | TOKEN_EXPIRED | No automático (requiere acción del ERP) |
| 4008 | RATE_LIMITED | Sí, con backoff mayor |
| 4009 | BAD_MESSAGE | Sí (reset de estado) |

## 7. Offline

Sin conexión, el Agent:

- **Sigue operando localmente** (impresión vía CLI y perfiles cacheados).
- Mantiene en memoria los **PrinterProfile** ya sincronizados.
- **Encola** eventos salientes pendientes (resultados de jobs no confirmados,
  logs, cambios de estado) en una cola **acotada** (descarta los más antiguos al
  llenarse) y los **reenvía** al reconectar.
- No recibe jobs nuevos (el ERP los mantiene en su cola mientras el Agent está
  offline; entrega al reconectar).

## 8. Versionado

- `protocol_version` **semver** negociada en `hello`. El servidor responde
  `authenticated` (compatible) o cierra `4002 PROTOCOL_UNSUPPORTED` indicando el
  rango soportado.
- **MAJOR**: cambios incompatibles. **MINOR**: campos/mensajes aditivos
  (compatibles). **PATCH**: correcciones de documentación.
- Regla de compatibilidad: **ignorar campos desconocidos**; un `type` desconocido
  se responde con `error {UNKNOWN_TYPE}` y se descarta, sin romper la conexión.
- Detalle en [messages.md §Versionado](messages.md).
