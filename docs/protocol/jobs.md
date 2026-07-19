# Protocolo — Jobs

> Fase 4. Propuesta pendiente de aprobación. Owner: Communication Engineer.
> El Agent ejecuta jobs a través del **motor de impresión** ya aprobado
> (ver [../PRINT_ENGINE.md](../PRINT_ENGINE.md)). Ver [messages.md](messages.md).

## 1. Ciclo de vida

```
Server → job ──▶ Agent valida ──▶ job_received (ack)
                                       │
                                       ▼
                              ejecuta (Print Engine)
                                       │
                         ┌─────────────┴─────────────┐
                         ▼                             ▼
                  job_completed                    job_failed
```

Reglas:

1. El servidor **solo** envía jobs cuando el Agent está `CONNECTED`.
2. El Agent responde **siempre**: primero `job_received`, luego exactamente uno de
   `job_completed` / `job_failed`.
3. Los resultados no confirmados por caída de red se **reenvían** al reconectar
   (cola de salida acotada; ver [websocket.md §7](websocket.md)).

## 2. Mensaje `job`  (Server → Agent)

```json
{
  "type": "job",
  "id": "job_12345",
  "job_type": "print",
  "payload": { }
}
```

- `id`: único y estable. Es la clave de **idempotencia** (ver §5).
- `job_type`: enruta el job internamente. MVP: `print`. Futuro: `device`, etc.
- `payload`: específico del `job_type`.

### 2.1 Payload de `print`

Coincide con el contrato que ya consume el motor (`app/print`):

```json
{
  "printer_id": "XP-80",
  "format": "application/pdf",
  "content": "<base64>",
  "copies": 1,
  "cut": true,
  "open_drawer": false
}
```

- `printer_id`: la impresora destino; el Agent ya conoce su `PrinterProfile`
  (cacheado). El ERP **no** reenvía el perfil en cada job.
- `format`: MIME de origen (`application/pdf`, `image/png`, `text/plain`, …). El
  `Resolver` elige el pipeline según el perfil (p. ej. PDF → ESC/POS raster).
- `content`: documento en base64.

## 3. Acuse  `job_received`  (Agent → Server)

Inmediato tras validar el sobre (antes de ejecutar):

```json
{ "type": "job_received", "id": "job_12345" }
```

## 4. Resultado

### 4.1 Completed

```json
{
  "type": "job_completed",
  "id": "job_12345",
  "duration_ms": 1200,
  "result": { "device": "escpos", "bytes": 55133 }
}
```

### 4.2 Failed

```json
{
  "type": "job_failed",
  "id": "job_12345",
  "error": { "code": "JOB_UNKNOWN_PRINTER", "message": "no profile for XP-80" }
}
```

Catálogo de códigos de job en [errors.md](errors.md).

## 5. Idempotencia y entrega

- Entrega **at-least-once** desde el servidor (puede reenviar un `job` tras un
  timeout). El Agent **deduplica por `id`**: si ya ejecutó/está ejecutando ese
  `id`, reenvía el estado actual y **no** vuelve a imprimir. → efecto *exactly-once*.
- El Agent mantiene un registro reciente de `id` procesados (ventana acotada por
  tiempo/cantidad) para deduplicar tras reconexión.
- `job_received` no implica ejecución completada; el ERP debe esperar
  `job_completed`/`job_failed` para el estado final.

## 6. Validación (antes de ejecutar)

El Agent rechaza con `job_failed` sin ejecutar cuando:

- Falta `printer_id` o no hay `PrinterProfile` en cache → `JOB_UNKNOWN_PRINTER`.
- `format` no resoluble para ese perfil → `JOB_UNSUPPORTED_FORMAT`.
- `content` vacío o supera el límite de tamaño → `JOB_BAD_PAYLOAD`.

## 7. Concurrencia y orden

- Límite de jobs concurrentes configurable (por defecto secuencial por impresora
  para no intercalar tickets).
- Sin garantía de orden entre impresoras distintas; por impresora, FIFO.
- `context`/timeout por job; si expira → `job_failed {JOB_TIMEOUT}`.
