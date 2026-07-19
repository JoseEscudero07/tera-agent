# Protocolo — Autenticación y Registro

> Fase 4. Propuesta pendiente de aprobación. Owner: Communication Engineer
> (revisión: Security Engineer). Ver [websocket.md](websocket.md).

## 1. Modelo

- Autenticación **exclusivamente por Token** generado por Django.
- El Token representa una **instalación/equipo**, no un usuario.
- El Agent **no crea, no renueva y no administra** Tokens ni usuarios. Solo lo
  transmite. Toda validación y expiración pertenece al Backend.
- El Token viaja **solo** en el mensaje `authenticate`, **nunca** en URL, query
  string ni logs. Tampoco el `agent_id`/`company_id` en logs.

## 2. Identificadores

| Campo | Origen | Descripción |
|---|---|---|
| `installation_id` | Backend (al registrar la instalación) | Id lógico de la instalación; puede ir vacío en la primera conexión. |
| `machine_id` | Agent | Huella estable del equipo (derivada de hardware/SO). No sensible. |
| `token` | Backend | Credencial de la instalación. Secreto. |
| `agent_id` | Backend (respuesta) | Id que el Backend asigna al Agent autenticado. |
| `company_id`, `branch_id` | Backend (respuesta) | Empresa y sucursal asociadas. |

`machine_id` permite al Backend detectar reinstalaciones/cambios de equipo.

## 3. Flujo

### 3.1 Hello  (Agent → Server)

Primer mensaje tras el upgrade WebSocket. Negocia versión.

```json
{
  "type": "hello",
  "agent_version": "1.0.0",
  "protocol_version": "1.0",
  "installation_id": "",
  "machine_id": "a1b2c3d4e5f6"
}
```

### 3.2 Authenticate  (Agent → Server)

```json
{
  "type": "authenticate",
  "token": "<opaque-token>"
}
```

### 3.3 Authenticated  (Server → Agent)

Token válido. El Agent adopta la identidad y transita a `REGISTERING`.

```json
{
  "type": "authenticated",
  "agent_id": "agt_123",
  "company_id": "co_9",
  "branch_id": "br_4"
}
```

### 3.4 Auth Error  (Server → Agent)

```json
{
  "type": "auth_error",
  "error": { "code": "AUTH_INVALID_TOKEN", "message": "token not recognized" },
  "retryable": false
}
```

Tras `auth_error` el servidor cierra el socket con el close code correspondiente
(`4001` inválido, `4003` expirado). El Agent:

- `retryable=false` (token inválido) → **no** reintenta en bucle; pasa a `ERROR`
  y espera intervención (nuevo Token). Reintento lento opcional al `cap`.
- `retryable=true` (fallo temporal del backend) → backoff normal.

## 4. Registro  (tras `authenticated`)

El Agent envía dos mensajes de onboarding: `register` (datos del equipo) y
`capabilities` (hardware detectado). Ambos **idempotentes**: se reenvían tal cual
en cada reconexión.

### 4.1 Register  (Agent → Server)

```json
{
  "type": "register",
  "hostname": "caja-01",
  "os": "linux",
  "os_version": "Ubuntu 24.04",
  "agent_version": "1.0.0",
  "local_ip": "192.168.1.50"
}
```

### 4.2 Capabilities  (Agent → Server)

Reporta dispositivos e impresoras físicas detectadas. El Agent **no** envía
perfiles (los administra el ERP). Ver el flujo de perfiles en [events.md](events.md).

```json
{
  "type": "capabilities",
  "devices": ["printer"],
  "printers": [
    { "name": "XP-80", "driver": "cups", "type": "escpos" }
  ]
}
```

## 5. Seguridad (resumen; revisión completa por Security Engineer)

- TLS 1.2+ con verificación de certificado; el Token nunca en claro fuera del
  frame TLS, ni en logs, ni en URL.
- Expiración de Token: responsabilidad del Backend; el Agent reacciona a
  `TOKEN_EXPIRED` sin intentar renovar.
- **Anti-replay**: el canal TLS ya protege el flujo; adicionalmente `hello` y
  `authenticate` incluyen `ts` (RFC3339) y el servidor puede rechazar mensajes
  con desfase temporal excesivo o reutilizados. (Opción a evaluar: challenge
  `nonce` emitido por el servidor antes de `authenticate`.)
- Validación de esquema de `hello`/`authenticate` antes de procesar; límites de
  tamaño; rate limiting de intentos de autenticación por `machine_id`/IP.
