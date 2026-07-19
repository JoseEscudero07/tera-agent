# Protocolo — Eventos, Heartbeat, Config y Sincronización de Perfiles

> Fase 4. Propuesta pendiente de aprobación. Owner: Communication Engineer.
> Ver [messages.md](messages.md).

## 1. Heartbeat  (Agent → Server)

Periódico, en el intervalo indicado por `config.heartbeat_interval_s`. Es la señal
que el ERP usa para “vivo / ocupado / pendientes” (distinto del ping de transporte).

```json
{
  "type": "heartbeat",
  "state": "CONNECTED",
  "uptime_s": 3600,
  "mem_bytes": 25165824,
  "version": "1.0.0",
  "pending_jobs": 0
}
```

- `state`: estado de la FSM del Agent.
- Si el servidor no recibe heartbeats durante `2×intervalo`, considera el Agent
  caído y lo marca offline en el ERP.

## 2. Config  (Server → Agent)

Enviado durante el onboarding (tras `profiles_ack`) y cuando cambie. Ajusta el
comportamiento sin reiniciar.

```json
{
  "type": "config",
  "heartbeat_interval_s": 30,
  "log_level": "info",
  "max_concurrent_jobs": 1
}
```

- Campos desconocidos se ignoran (compatibilidad). Valores fuera de rango se
  acotan a límites seguros del Agent.

## 3. Sincronización de perfiles de impresora

El **perfil pertenece al ERP**. El Agent solo reporta impresoras físicas y cachea
en memoria lo que el Backend le envía.

```
Agent detecta impresora física
        │  (capabilities: ver authentication.md)
        ▼
ERP asigna PrinterProfile (catálogo)
        │
        ▼
Server → profiles_sync  ──▶  Agent guarda en cache (ProfileCache)
        ▲                         │
        │                         ▼
        └────────  Agent → profiles_ack {version}
```

### 3.1 Profiles Sync  (Server → Agent) — sincronización completa

```json
{
  "type": "profiles_sync",
  "version": 7,
  "profiles": [
    {
      "printer_id": "XP-80",
      "native_formats": ["escpos"],
      "width_dots": 576,
      "dpi": 203,
      "supports_cut": true,
      "supports_drawer": true,
      "meta": { "technology": "thermal", "logo": "..." }
    }
  ]
}
```

- `version`: número monótono del catálogo. El Agent lo devuelve en `profiles_ack`
  y lo usa para detectar desincronización.
- Reemplaza por completo la cache (`ProfileCache.SetAll`).

### 3.2 Profile Update  (Server → Agent) — cambio puntual

```json
{
  "type": "profile_update",
  "version": 8,
  "profile": { "printer_id": "XP-80", "native_formats": ["escpos"], "width_dots": 576, "dpi": 203, "supports_cut": true, "supports_drawer": false, "meta": {} }
}
```

- Upsert de un único perfil (`ProfileCache.Set`). El Agent actualiza su `version`.

### 3.3 Profiles Ack  (Agent → Server)

```json
{ "type": "profiles_ack", "version": 7 }
```

- Si el Agent detecta que su `version` cacheada es menor que la anunciada por el
  servidor (p. ej. en un heartbeat futuro), solicita un `profiles_sync` completo.

## 4. Eventos del Agent  (Agent → Server)

Notificaciones asíncronas iniciadas por el Agent.

```json
{
  "type": "event",
  "event": "printer_status_changed",
  "ts": "2026-07-19T19:00:00Z",
  "data": { "printer_id": "XP-80", "status": "OFFLINE" }
}
```

Eventos previstos (extensible): `printer_status_changed`,
`device_connected`, `device_disconnected`, `agent_error`.

## 5. Logs  (Agent → Server)

Reenvío de logs estructurados al ERP (nivel según `config.log_level`).

```json
{ "type": "log", "level": "warn", "message": "reconnecting", "ts": "2026-07-19T19:00:01Z" }
```

- **Nunca** incluir Token, `agent_id` sensible ni datos personales en `message`.

## 6. Buffer offline

`event`, `log` y resultados de job pendientes se **encolan** cuando no hay
conexión y se **vacían** al reconectar (cola acotada; descarta lo más antiguo al
llenarse). Ver [websocket.md §7](websocket.md).
