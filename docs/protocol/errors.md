# Protocolo — Modelo de Errores

> Fase 4. Propuesta pendiente de aprobación. Owner: Communication Engineer
> (revisión: Security Engineer). Ver [messages.md](messages.md).

## 1. Formato de error

Todo error (en `auth_error`, `error`, `job_failed`) usa la misma forma:

```json
{
  "error": {
    "code": "JOB_UNKNOWN_PRINTER",
    "message": "no profile for XP-80",
    "details": { }
  },
  "retryable": false
}
```

- `code`: enum **estable** (string en `SCREAMING_SNAKE_CASE`). Es lo que el ERP
  interpreta programáticamente.
- `message`: texto humano, no localizado; solo para diagnóstico. Sin datos
  sensibles.
- `details`: objeto opcional con contexto (p. ej. `min`/`max` de versión).
- `retryable`: pista de si reintentar tiene sentido.

## 2. Errores de conexión / protocolo

| Código | Cuándo | Close code | Reintentar |
|---|---|---|---|
| `AUTH_INVALID_TOKEN` | Token no reconocido | 4001 | No |
| `TOKEN_EXPIRED` | Token caducado | 4003 | No (acción del ERP) |
| `PROTOCOL_UNSUPPORTED` | Versión fuera de rango | 4002 | No (actualizar Agent) |
| `BAD_MESSAGE` | JSON/esquema inválido | 4009 | Sí (reset) |
| `UNKNOWN_TYPE` | `type` no reconocido | — (no cierra) | — (se descarta) |
| `RATE_LIMITED` | Demasiados mensajes | 4008 | Sí, backoff mayor |
| `INTERNAL` | Error del servidor | 1011/1012 | Sí |

- `UNKNOWN_TYPE` **no** cierra la conexión: se responde `error` y se ignora el
  mensaje (compatibilidad hacia adelante).

## 3. Errores de job

Van en `job_failed.error.code`:

| Código | Significado |
|---|---|
| `JOB_UNKNOWN_PRINTER` | No hay `PrinterProfile` en cache para `printer_id` |
| `JOB_UNSUPPORTED_FORMAT` | El `Resolver` no encuentra pipeline para `format`+perfil |
| `JOB_BAD_PAYLOAD` | `content` vacío, no decodificable o supera el límite |
| `JOB_RENDER_FAILED` | Fallo del Renderer (p. ej. `pdftoppm`) |
| `JOB_ENCODE_FAILED` | Fallo del Encoder |
| `JOB_DEVICE_ERROR` | Fallo del Driver / dispositivo (p. ej. `lp`) |
| `JOB_TIMEOUT` | Expiró el `context` del job |
| `JOB_DUPLICATE` | `id` ya procesado (se devuelve el estado previo) |

## 4. Principios de manejo

- Un mensaje inválido **nunca** debe tumbar el Agent: se registra, se responde
  `error`/`job_failed` y se continúa.
- Los errores **no reintentar** (`retryable=false`) no deben generar bucles de
  reconexión agresivos: el Agent espera (estado `ERROR`/`OFFLINE`) e informa.
- Todo `error.message` se sanea: sin Token, sin `agent_id` sensible, sin datos
  personales (coherente con la política de logs).
- Límites (tamaño de mensaje, rate) protegen memoria/CPU; superarlos produce un
  error tipado, no un fallo silencioso.

## 5. Mapa a la FSM

| Situación | Transición |
|---|---|
| `auth_error` no reintentable | `AUTHENTICATING → ERROR` |
| Cierre inesperado / pérdida de red | `CONNECTED → DISCONNECTED → RECONNECTING` |
| Reintentos agotados | `RECONNECTING → OFFLINE` |
| `reconnect` del servidor | cierre ordenado → `RECONNECTING` |
| `bye` / shutdown local | `→ SHUTTING_DOWN` |
