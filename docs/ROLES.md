# Roles de impresión y ruteo multi-agente

Documento fijado del diseño de impresión automática por rol. Owner: Software
Architect. Última revisión: 2026-07-22.

## Problema

Una sede puede tener varios equipos con Tera Agent instalado (caja, cocina,
mostrador…). Dos impresoras físicas distintas pueden llamarse igual en el SO
(dos "POS80"). El backend Django que genera facturas, tickets, comandas y
remisiones **no debe conocer el nombre físico de cada impresora**: ese
conocimiento vive en el equipo donde está la impresora.

## Dos caminos de impresión

Coexisten y son complementarios. Nunca compiten.

### 1. HTTP (operador → POS → backend)

El operador elige una impresora en el POS/tablet. El frontend Angular manda
`impresora_id` (UUID de catálogo) al backend, que lo reenvía al Agent dueño de
esa impresora. Es un camino **explícito**: el humano decide.

- Usa `impresora_id` — nunca ambiguo aunque haya nombres repetidos.
- Endpoint: `POST /tera-agent/imprimir/` con `impresora_id` en el multipart.
- No usa el sistema de roles.

### 2. Backend automático (código de negocio → Agent)

Una función interna del backend (al guardar factura, al confirmar pedido)
imprime sin intervención humana. El backend NO sabe qué impresoras hay en la
sede — sólo declara un **rol funcional**:

```python
from apps.tera_agent.roles import Rol
from apps.tera_agent.services import imprimir_pdf

imprimir_pdf(ticket_pdf, factura.empresa, factura.sede, rol=Rol.FACTURACION)
imprimir_pdf(comanda_pdf, pedido.empresa, pedido.sede, rol=Rol.COCINA)
imprimir_pdf(cierre_z_pdf, caja.empresa, caja.sede,     rol=Rol.CAJA)
```

Este documento cubre este segundo camino.

## Roles canónicos

Enum cerrado. Ampliar la lista requiere migración de una línea.

| Constante          | Valor          | Uso típico                                            |
|--------------------|----------------|-------------------------------------------------------|
| `Rol.CAJA`         | `caja`         | Recibos de caja, cierre Z, arqueos, movimientos       |
| `Rol.FACTURACION`  | `facturacion`  | Tickets 80mm y facturas A4 de venta                   |
| `Rol.COCINA`       | `cocina`       | Comandas, delivery, KDS impreso                       |
| `Rol.BODEGA`       | `bodega`       | Remisiones, órdenes de despacho, ingresos             |
| `Rol.OFICINA`      | `oficina`      | Informes internos, listados administrativos           |
| `Rol.ETIQUETA`     | `etiqueta`     | Precios, códigos de barra (impresora angosta)         |

Reglas de estilo del valor: minúsculas, sin tildes, sin espacios. Deben
coincidir exactamente entre Django (`apps/tera_agent/roles.py`), el Agent
Go (`internal/app/ports/config.go`) y el frontend Angular
(`tera-agent.models.ts::RolImpresora`).

## Ruteo por rol

Cuando el backend llama `imprimir_pdf(..., rol=Rol.X)` se disparan **dos
resoluciones en cadena**: primero el backend intenta apuntar directo, si no
puede, delega al Agent.

### Backend (Django)

```
1. Busca Impresora(sede, rol=X, agente__online=True).
   → Sí: manda job al WS de ese agente con printer_id explícito.
2. No hay match:
   → Manda job a cualquier agente online de la sede con
     printer_id="" role="X". El Agent resuelve local (paso 3+).
3. No hay agente online: AgenteNoDisponible.
```

Cuando en el paso 1 hay varias impresoras con el rol X en la sede, la
heurística prefiere: **agente online → predeterminada → primera**.

### Agent (Go)

Cuando llega un job con `printer_id=""` y `role != ""`:

```
1. Impresoras managed activadas con ese rol:
   - 1 sola  → esa.
   - 2+      → predeterminada del agente si es una de ellas, si no la primera.
2. Ninguna con el rol → impresora predeterminada del agente.
3. Sin predeterminada → error (job_failed).
```

El Agent responde con `job_failed { error }` si no encontró impresora, y el
backend lo persiste en `TrabajoImpresion.error`. El error queda visible en el
panel/logs.

## Configuración por sede

El único paso que hace el humano al desplegar una sede es **marcar el rol de
cada impresora** en el panel local del Agent (Impresoras → Rol). Al guardar
se persisten dos efectos:

1. `SaveConfig` actualiza el `config.yaml` en el equipo.
2. `RoleResolver.Update` refresca el resolver en caliente — sin reiniciar.
3. En la próxima `Capabilities` (heartbeat implícito o reconexión) el Agent
   sube el rol al backend Django, que lo persiste en `Impresora.rol`.

Zero-config funciona: si nunca marcas roles, todo cae en la impresora
predeterminada del agente. Los llamados con `rol=…` seguirán funcionando por
el fallback local.

## Escenarios canónicos

### Sede simple: un solo agente, una impresora POS80

- Configuración: nada. Al primer `capabilities` el Agent registra POS80, la
  marcas como `predeterminada=True` en el panel del ERP.
- `imprimir_pdf(..., rol=Rol.FACTURACION)`: backend no encuentra rol
  facturacion → delega al Agent → Agent no encuentra rol → cae en la
  predeterminada → imprime en POS80.
- No hace falta asignar rol en absoluto para el caso feliz.

### Sede restaurante: dos agentes, una impresora cada uno

- AGENTE-CAJA-01 con POS80 (rol `facturacion`) marcada predeterminada.
- AGENTE-COCINA-01 con Xprinter (rol `cocina`).
- `imprimir_pdf(rol=Rol.FACTURACION)` → backend encuentra POS80 con
  agente=AGENTE-CAJA-01 online → envía al WS de ese agente → imprime.
- `imprimir_pdf(rol=Rol.COCINA)` → mismo mecanismo, va a Xprinter en cocina.

### Sede con dos "POS80" (dos cajas)

- AGENTE-CAJA-01 con POS80 (rol `facturacion`, predeterminada agente 01).
- AGENTE-CAJA-02 con POS80 (rol `facturacion`, predeterminada agente 02).
- `imprimir_pdf(rol=Rol.FACTURACION)` → backend encuentra dos filas. Heurística:
  ambas online, ninguna es "la" predeterminada de la sede → primera del
  ordenamiento (`-predeterminada, nombre`). Recomendación: en el panel del ERP
  marca UNA como predeterminada global de la sede para desempate estable.

### HP LaserJet A4 en la misma sede que un POS80

- POS80 con rol `facturacion`, predeterminada.
- HP LaserJet **sin rol** (o rol distinto, p.ej. `oficina`).
- Automático `rol=Rol.FACTURACION` va a POS80.
- Manual desde Angular (usuario elige HP en el dropdown) usa `impresora_id`,
  no toca el rol.

## Modelo de datos

### `Impresora` (Django)

```python
class Impresora(BaseModel):
    nombre         = CharField(150)   # nombre en el SO
    etiqueta       = CharField(150)   # nombre amigable
    agente         = FK(AgenteImpresion, null=True)
    rol            = CharField(20, choices=Rol.CHOICES, blank=True)
    tipo           = CharField(20, choices=[termica, normal])
    ancho_dots     = PositiveIntegerField()
    corte          = BooleanField()
    cajon          = BooleanField()
    predeterminada = BooleanField()
    activa         = BooleanField()

    class Meta:
        constraints = [
            UniqueConstraint(
                fields=["sede", "agente", "nombre"],
                name="unique_impresora_por_sede_agente",
            ),
        ]
```

FK `agente` es nullable para retro-compat con instalaciones mono-agente
anteriores. La UniqueConstraint incluye `agente` para permitir que dos
"POS80" convivan si están en agentes distintos.

### `ManagedPrinter` (Agent Go, `internal/app/ports/config.go`)

```go
type ManagedPrinter struct {
    Name          string
    Role          PrinterRole   // caja/facturacion/cocina/...
    Enabled       bool
    CutFeedDots   int
    TopMarginDots int
}
```

Se persiste en `config.yaml` bajo `printer.managed`.

## Protocolo WebSocket

### Agent → Backend, mensaje `capabilities`

Se envía tras `authenticate` durante el handshake. Ahora incluye rol y
enabled por impresora:

```json
{
  "type": "capabilities",
  "devices": ["printer"],
  "printers": [
    {"name": "POS80", "driver": "escpos", "type": "escpos",
     "role": "facturacion", "enabled": true},
    {"name": "Xprinter-Cocina", "driver": "escpos", "type": "escpos",
     "role": "cocina", "enabled": true}
  ]
}
```

El consumer los persiste en `AgenteImpresion.impresoras_detectadas` para que
el panel del ERP pueda sugerir el rol al registrar impresoras del agente.

### Backend → Agent, payload de `job`

Ahora acepta `role` además de `printer_id`:

```json
{
  "type": "job",
  "id": "<uuid>",
  "job_type": "print",
  "payload": {
    "printer_id": "",
    "role": "cocina",
    "format": "application/pdf",
    "content": "<base64>",
    "cut": true,
    "open_drawer": false,
    "copies": 1
  }
}
```

- `printer_id` no vacío → el Agent obedece literal (path HTTP y path automático
  cuando el backend acertó por rol).
- `printer_id` vacío + `role` no vacío → el Agent resuelve local.

### Backend → Agent, evento `profiles.push`

Empujado por el backend cuando cambia el catálogo (registro/actualización de
`Impresora`) para que el `ProfileCache` del Agent se actualice sin
reconectarse. El handler del consumer reenvía un `profiles_sync` con los
perfiles vigentes del agente.

## Endpoints REST relevantes

| Método | Path                                           | Uso                                                        |
|--------|------------------------------------------------|------------------------------------------------------------|
| POST   | `/tera-agent/imprimir/`                        | HTTP path; acepta `impresora_id`, `impresora`, `rol` (opt) |
| POST   | `/tera-agent/cajon/`                           | Abrir cajón; mismos parámetros                             |
| GET    | `/tera-agent/impresoras/`                      | Catálogo de la sede; devuelve `rol`, `agente_*`            |
| POST   | `/tera-agent/impresoras/`                      | Registra/actualiza; acepta `agente_id`, `rol`              |
| GET    | `/tera-agent/impresoras/por-agente/`           | Catálogo agrupado — para el selector del POS               |
| GET    | `/tera-agent/impresoras/detectadas/por-agente/`| Detectadas agrupadas — para el diálogo de gestión          |

## API Python del backend

Toda impresión automática entra por `apps.tera_agent.services`:

```python
imprimir_pdf(
    pdf,                   # bytes | str (ruta) | file-like
    empresa, sede,         # instancias o UUIDs
    impresora=None,        # str (SO name) o instancia — path HTTP
    impresora_id=None,     # UUID — preferido cuando hay ambigüedad
    rol=None,              # Rol.XXX — path automático recomendado
    cut=True,
    abrir_cajon=False,
    copias=1,
    usuario=None,
)
```

Reglas:

- `impresora_id` gana sobre `impresora` gana sobre `rol` gana sobre la
  predeterminada de la sede.
- Nunca escribir un rol como string suelto (`"cocina"`). Siempre `Rol.COCINA`
  para que `grep` lo encuentre y refactors renombren en cascada.
- `ValueError` si el rol no está en `Rol.VALORES`.

## Fallo elegante

Cuando algo se rompe, la responsabilidad se distribuye así:

| Falla                                                    | Excepción / campo `TrabajoImpresion.error`                     |
|----------------------------------------------------------|----------------------------------------------------------------|
| No hay ningún agente online en la sede                   | `AgenteNoDisponible` (409 en HTTP)                             |
| Rol no válido en el llamado                              | `ValueError`                                                   |
| Rol válido, no hay impresora en catálogo ni default local| `job_failed { error: "no se pudo resolver la impresora ..." }` |
| Impresora existe pero el driver falla                    | `job_failed { error: "<driver-specific>" }` — visible en logs |

El HTTP path del frontend cae al fallback "abrir PDF en navegador" cuando
recibe `FALLIDO`; el path automático del backend solo persiste el estado del
trabajo — el operador puede reintentar desde el panel.

## Chequeo pre-piloto

Antes de encender el path automático en una sede nueva:

1. `python manage.py migrate tera_agent` (migraciones 0004, 0005).
2. Reiniciar el proceso ASGI de Django (consumers cambiaron).
3. Instalar/actualizar el Agent (binario Windows nuevo) en cada equipo.
4. Al primer `capabilities` verificar en `/admin/tera_agent/agenteimpresion/`
   que las impresoras aparecen con `role` poblado.
5. En el panel del ERP, para cada `Impresora`:
   - Marcar `rol`.
   - Marcar `predeterminada` en al menos UNA por sede.
6. Ejecutar `imprimir_pdf(pdf_dummy, empresa, sede, rol=Rol.FACTURACION)` desde
   un shell de Django y confirmar que sale por la POS80.

## Extensión futura (no incluida)

- Mapping por-sede `tipo_documento → rol` en tabla opcional para override sin
  código: descartado por YAGNI. Se añadirá si aparece un caso real.
- Roles adicionales: se añaden por migración de una línea en `roles.py`,
  `config.go` y `RolImpresora`. Bajo costo, no requiere rediseño.
