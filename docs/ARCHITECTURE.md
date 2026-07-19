# Arquitectura — Tera Agent

## Objetivo

Producto empresarial preparado para evolucionar durante años. Prioridad
absoluta: **arquitectura, mantenibilidad, escalabilidad, seguridad y
rendimiento** — nunca sacrificados por velocidad de desarrollo.

## Capas (Clean Architecture)

La regla de dependencia es estricta: **las dependencias apuntan hacia adentro**.
Una capa interior nunca importa una exterior.

```
        ┌──────────────────────────────────────────┐
        │  adapters (frameworks e infraestructura)  │  websocket, cups, escpos,
        │                                            │  zebra, ui, config, logger
        ├──────────────────────────────────────────┤
        │  app (casos de uso / orquestación)        │  lifecycle, dispatcher, ports
        ├──────────────────────────────────────────┤
        │  domain (reglas de empresa)               │  agent(FSM), job, printing,
        │                                            │  device, comms  ← ports
        └──────────────────────────────────────────┘
                 ▲ implementan               ▲ dependen de interfaces
                 └── adapters ───────────────┘

     infra/di  = composition root: ÚNICO lugar que importa adapters y los
                 inyecta como implementaciones de los ports.
```

### domain
Entidades y **ports** (interfaces). Sin dependencias externas. Contiene:
la máquina de estados (`agent`), la unidad de trabajo (`job`), y los contratos
de `printing`, `device` y `comms` (incluido el port `Transport`).

### app
Orquestación sobre los ports del dominio:
- `lifecycle` — secuencia STARTING → CONNECTING → AUTHENTICATING → REGISTERING →
  CONNECTED → serve → SHUTTING_DOWN.
- `dispatcher` — enruta `job.Job` al `Handler` registrado según `Kind`.
- `ports` — interfaces driven que la app necesita (Logger, ConfigStore, Clock).

### adapters
Implementaciones concretas e intercambiables de los ports. Cada backend de
impresión y cada driver de dispositivo es un adapter independiente. El código
específico de plataforma va detrás de **build tags**.

### infra/di
Composition root. Construye los adapters concretos y los inyecta. Es el único
paquete autorizado a importar `adapters`.

## Propiedad por agente

| Área | Paquetes | Agente responsable |
|------|----------|--------------------|
| Núcleo / lifecycle / DI | `domain/agent`, `domain/job`, `app/*`, `infra/di`, `adapters/logger`, `adapters/config` | Go Core Engineer |
| Comunicación | `domain/comms`, `adapters/communication/websocket` | Communication Engineer |
| Impresión | `domain/printing`, `adapters/printing` | Printing Engineer |
| Hardware | `domain/device`, `adapters/device` | Device Engineer |
| UI | `adapters/ui` | UI Engineer |
| Ports / diseño | interfaces del dominio | Software Architect |

Ningún agente modifica módulos de otro sin documentar el motivo.

## Reglas invariantes

- Sin dependencias circulares. Sin acoplamiento entre adapters.
- Interfaces pequeñas; `context.Context` para cancelación y timeouts.
- Sin variables globales. Archivos y funciones pequeños.
- Preferir la biblioteca estándar de Go; dependencias externas solo con
  mantenimiento activo, licencia permisiva (MIT/BSD/Apache) y revisión del
  Security Engineer.

## Comunicación con el ERP

`Angular → Django → WebSocket → Agent`. Nunca hay comunicación directa
Angular ↔ Agent. El Agent **siempre** inicia la conexión (TLS). El Backend es el
único que envía trabajos. Ver [`PROTOCOL.md`](PROTOCOL.md).
