# Protocolo — Mensajes, Envelope y Versionado

> Fase 4. Propuesta pendiente de aprobación. Owner: Communication Engineer.
> Ver [websocket.md](websocket.md).

## 1. Envelope

Todo mensaje es un objeto JSON con un discriminador `type`. Los campos
específicos van al nivel superior (planos); solo `job` anida su contenido en
`payload`.

Campos comunes:

| Campo | Tipo | Presencia | Descripción |
|---|---|---|---|
| `type` | string | siempre | Discriminador del mensaje. |
| `id` | string | en jobs y sus respuestas | Correlación. En respuestas de job es **el id del job**. |
| `ts` | string (RFC3339 UTC) | recomendado | Reloj del emisor. |
| `v` | string (semver) | en `hello`/`authenticated` | Versión de protocolo. |

Reglas:

- Codificación **UTF-8**, un mensaje por frame de texto.
- **Ignorar campos desconocidos** (compatibilidad hacia adelante).
- `type` desconocido → responder `error {code: "UNKNOWN_TYPE"}` y descartar, sin
  cerrar la conexión.

## 2. Catálogo de mensajes

### Agent → Server

| `type` | Fase | Propósito | Doc |
|---|---|---|---|
| `hello` | AUTHENTICATING | Saludo + negociación de versión | [authentication](authentication.md) |
| `authenticate` | AUTHENTICATING | Presenta el Token | [authentication](authentication.md) |
| `register` | REGISTERING | Datos del equipo | [authentication](authentication.md) |
| `capabilities` | REGISTERING | Dispositivos e impresoras físicas | [authentication](authentication.md) |
| `profiles_ack` | REGISTERING | Confirma recepción de perfiles | [events](events.md) |
| `heartbeat` | CONNECTED | Estado periódico | [events](events.md) |
| `job_received` | CONNECTED | Acuse de recibo de un job | [jobs](jobs.md) |
| `job_completed` | CONNECTED | Resultado OK de un job | [jobs](jobs.md) |
| `job_failed` | CONNECTED | Resultado con error de un job | [jobs](jobs.md) |
| `event` | CONNECTED | Notificación (cambio de estado de impresora, etc.) | [events](events.md) |
| `log` | CONNECTED | Log estructurado reenviado al ERP | [events](events.md) |
| `bye` | SHUTTING_DOWN | Cierre ordenado | [websocket](websocket.md) |

### Server → Agent

| `type` | Fase | Propósito | Doc |
|---|---|---|---|
| `authenticated` | AUTHENTICATING | Token válido + identidad | [authentication](authentication.md) |
| `auth_error` | AUTHENTICATING | Token inválido/expirado | [authentication](authentication.md) · [errors](errors.md) |
| `profiles_sync` | REGISTERING | Sincronización completa de perfiles | [events](events.md) |
| `profile_update` | CONNECTED | Alta/cambio de un perfil | [events](events.md) |
| `config` | REGISTERING/CONNECTED | Config (intervalo heartbeat, log level, …) | [events](events.md) |
| `job` | CONNECTED | Trabajo a ejecutar | [jobs](jobs.md) |
| `error` | cualquiera | Error de protocolo/servidor | [errors](errors.md) |
| `reconnect` | CONNECTED | Pide reconexión (mantenimiento) | [websocket](websocket.md) |
| `bye` | cualquiera | Cierre ordenado del servidor | [websocket](websocket.md) |

## 3. Convenciones

- **Nombres**: `snake_case` en claves; `type` en `snake_case`.
- **Tiempos**: duraciones en `_ms` (int); marcas de tiempo en RFC3339 UTC.
- **Bytes**: contenido binario en base64 (`content`), con su `format` declarado.
- **Correlación de jobs**: la respuesta reutiliza el `id` del job (no genera uno
  nuevo). Ver [jobs.md](jobs.md).

## 4. Versionado

- `protocol_version` **semver**; negociada en `hello`.
- El servidor declara el **rango soportado**; si el Agent está fuera → cierre
  `4002 PROTOCOL_UNSUPPORTED` con `min`/`max` en el `error.details`.
- **MINOR** añade mensajes/campos opcionales sin romper: un extremo antiguo los
  ignora. **MAJOR** puede eliminar/renombrar y exige actualización.
- El catálogo de `type` es estable dentro de una MAJOR: no se reutiliza ni
  cambia el significado de un `type` existente.

## 5. Nota de implementación (no vinculante para este diseño)

El puerto de dominio actual `comms.Envelope{Type, Data}` evolucionará para portar
el objeto JSON completo (o `Type` + `json.RawMessage`) durante la implementación,
sin cambiar el dominio de impresión ni de negocio. La forma exacta se decide en
la fase de código, ya con este protocolo aprobado.
