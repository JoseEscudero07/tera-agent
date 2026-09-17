# Versionamiento, releases y distribución desde el ERP

Owner: DevOps Engineer. Contrato del manifiesto: Security Engineer.

## 1. Versionamiento

**SemVer estricto**, y una única fuente de verdad: el **tag de git**.

| Cambio | Incremento | Ejemplo |
|---|---|---|
| Rompe el protocolo con el Backend | `MAJOR` | mensajes nuevos obligatorios, cambia el handshake |
| Funcionalidad nueva compatible | `MINOR` | soporte de una impresora, endpoint nuevo del panel |
| Corrección sin cambio de contrato | `PATCH` | ajuste del corte, fuga de memoria |

La versión **nunca se escribe en el código**. `di.AgentVersion` es `var` con
valor `"0.0.0-dev"` y se inyecta al compilar:

```bash
go build -trimpath -ldflags "-X github.com/teraerp/tera-agent/internal/infra/di.AgentVersion=1.0.0" ./cmd/tera-agent
```

Ese `0.0.0-dev` es deliberado: si un binario llega a producción sin pasar por el
build de release, se ve inmediatamente en el heartbeat y en el panel. No falla
en silencio.

Esa misma versión va también en los **recursos de Windows** del `.exe` (el
nombre y la versión que enseñan las propiedades del fichero y el cuadro de UAC):
los genera `tools/winres` desde `installer/tera-agent.ico` justo antes de
compilar, en un `.syso` que el enlazador incrusta y que el build borra al
terminar. Nunca se versiona: llevaría la versión de otro release.

`scripts/build-release.ps1` valida el formato semver y **verifica que el binario
reporte la versión inyectada** antes de empaquetar. Si el `-ldflags` no cuaja, el
build falla ahí y no cuando 200 clientes dejen de actualizarse.

## 2. Sacar un release

```bash
# 1. Cierra el CHANGELOG: mueve [Unreleased] a [1.0.0] con la fecha.
# 2. Tag y push: el workflow de Release hace el resto.
git tag v1.0.0
git push origin v1.0.0
```

`.github/workflows/release.yml` compila en `windows-latest` (los tests de GDI y
del spooler solo se ejecutan ahí), descarga poppler, genera el instalador y
publica un **release en borrador** con `SHA256SUMS.txt`. Es borrador a propósito:
publicarlo es lo que hace que los clientes empiecen a descargar, así que verifica
los hashes antes.

## 3. Distribución desde el ERP

El Agent no conoce GitHub. Pregunta a **tu ERP** qué versión hay, y el ERP le
devuelve un manifiesto. El contrato ya está en
[`internal/domain/update/update.go`](../internal/domain/update/update.go).

### 3.1 Endpoint del manifiesto

El Agent deriva la URL del **mismo dominio del WebSocket** (`wss://erp/ws/agent/`
→ `https://erp/agent/version`), así que no hay nada nuevo que configurar en el
cliente. Ver [`adapters/update/manifest`](../internal/adapters/update/manifest/http.go).

```
GET /agent/version?os=windows&arch=amd64&current=1.0.0
```

```json
{
  "latest": "1.1.0",
  "url": "https://erp/agent/download/1.1.0/windows/amd64",
  "sha256": "76acc5c2a6dccfb77bdfebe20b3cf55ee3b849733bc962c66d81c60c1b060dbe",
  "mandatory": false,
  "min_supported": "1.0.0",
  "notes": "Corrige el corte en XP-80."
}
```

| Campo | Para qué |
|---|---|
| `latest` | versión disponible para ese `os`/`arch` |
| `url` | descarga HTTPS del binario |
| `sha256` | **obligatorio**. Sin él el Agent se niega a instalar |
| `mandatory` | fuerza la actualización sin que el usuario decida |
| `min_supported` | por debajo de esto el Agent se considera obsoleto y actualiza igual |
| `notes` | se muestran en el panel |

`mandatory` y `min_supported` son la palanca para cuando cambies el protocolo:
sube `min_supported` y los agentes viejos se actualizan solos en vez de fallar
con errores raros.

### 3.2 Modelo Django

```python
class AgentRelease(models.Model):
    version       = models.CharField(max_length=32)      # "1.1.0", semver
    os            = models.CharField(max_length=16)      # windows | linux | darwin
    arch          = models.CharField(max_length=16)      # amd64 | arm64 | 386
    file          = models.FileField(upload_to="agent/")
    sha256        = models.CharField(max_length=64)
    mandatory     = models.BooleanField(default=False)
    min_supported = models.CharField(max_length=32, blank=True)
    notes         = models.TextField(blank=True)
    published     = models.BooleanField(default=False)   # borrador vs visible
    rollout_pct   = models.PositiveSmallIntegerField(default=100)

    class Meta:
        unique_together = ("version", "os", "arch")
```

Tres cosas que evitan incidentes:

1. **Calcula el `sha256` en el servidor al subir el fichero**, no lo copies a
   mano del `SHA256SUMS.txt`. Un hash mal pegado deja a todos los clientes sin
   poder actualizar (fallan cerrado, que es lo correcto, pero nadie avanza).
2. **Sirve el binario desde almacenamiento** (S3/CDN), no desde Django: son
   ~20 MB por descarga y multiplicado por los POS de todos tus clientes.
3. **Despliega por fases** con `rollout_pct` o una lista de empresas piloto. Un
   release malo que llega a 200 POS a la vez es un día perdido; a 5, un rollback.

### 3.3 Endpoint de descarga

```
GET /agent/download/<version>/<os>/<arch>
```

Protégelo con el mismo Token del Agent. El binario no es secreto, pero un
endpoint abierto es superficie de ataque gratis.

## 4. Estado del auto-update en el Agent

Implementado y con tests:

- `domain/update` — manifiesto, comparación semver, puertos.
- `app/update` — servicio de comprobación (`Check`/`Apply`, cacheado para el panel).
- `adapters/update/manifest` — cliente HTTPS del manifiesto.
- `adapters/update/selfupdate` — descarga con verificación SHA256 obligatoria.

**Pendiente** (sin esto el auto-update no funciona todavía):

- `update.Applier` completo: reemplazar el `.exe` en uso (renombrar a `.old`,
  mover el nuevo, reiniciar el servicio) y hacer rollback si el nuevo no arranca.
- Cablearlo en `infra/di` y exponer `/api/update` en el panel.

Mientras eso no esté, la actualización de un cliente es: descargar el instalador
nuevo del ERP y ejecutarlo encima. El instalador actualiza in-place (mismo
`AppId`) y **conserva la configuración y el Token**.

## 5. Firma de código

Los binarios no están firmados. Cada instalación dispara SmartScreen ("Editor
desconocido") y es candidata a falso positivo de antivirus. Un certificado de
firma de código (OV ~200–400 €/año; EV si quieres reputación desde el primer día)
es lo que separa "instalar en un clic" de "el cliente llama porque Windows lo
bloqueó". Se firma en el workflow con `signtool` sobre los dos `.exe` **y** sobre
el instalador.
