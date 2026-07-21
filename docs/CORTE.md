# Ajustar el corte del papel (ESC/POS)

Guía para calibrar **dónde cae el corte** y **cuánto papel deja arriba**, por
impresora, **sin recompilar** el agente. Solo se edita el `config.yaml` y se
reinicia el servicio.

## El problema

La cuchilla está unos **10–18 mm por encima** del cabezal de impresión (varía por
modelo). Si tras imprimir no avanzamos suficiente papel antes de cortar, el corte
cae **sobre la última línea** (p. ej. la fecha sale partida). Si avanzamos de más,
queda **papel en blanco** de sobra. Y arriba, el margen en blanco del documento se
expulsa como papel desperdiciado.

Por eso el agente, antes de cortar, hace un avance controlado (`ESC J n`) y recorta
las filas en blanco de arriba dejando un pequeño margen.

## Configuración (lo que ajustas — sin recompilar)

En el `config.yaml` del agente (en el equipo; en Linux `/etc/tera-agent/config.yaml`):

```yaml
printer:
  default: "XP-80"
  # Defaults globales (para todas las impresoras):
  cut_feed_dots: 232      # avance antes del corte
  top_margin_dots: 16     # filas en blanco a dejar arriba
  # Overrides POR impresora (cada modelo pone la cuchilla a distinta distancia):
  managed:
    - name: "XP-80"
      cut_feed_dots: 248
      top_margin_dots: 12
    - name: "EPSON TM-T20"
      cut_feed_dots: 210
```

Resolución: si la impresora tiene su propio valor, gana; si no, se usa el global;
si tampoco, el default interno del agente (`cut_feed_dots=232`, `top_margin_dots=16`).
Un `0` significa "usar el default".

### Reglas de calibración

**Corte inferior (`cut_feed_dots`)**

| Síntoma                                           | Acción              |
|---------------------------------------------------|---------------------|
| Corta el contenido / la última línea sale partida | **SUBIR** el número |
| Deja demasiado papel en blanco tras el corte      | **BAJAR** el número |

**Margen superior (`top_margin_dots`)** = filas en blanco que se dejan arriba.
Súbelo para más margen, bájalo para menos. No recorta contenido, solo blanco.

Conversión: **1 mm ≈ 8 dots** @203dpi. Ejemplos: 120≈15mm, 200≈25mm, 232≈29mm.
⚠️ **`cut_feed_dots` máximo 255** (`ESC J` es de un solo byte).

> Estos valores los usan por igual los tickets renderizados (facturas) y el
> ticket de texto de *Probar*, así que se calibran una sola vez por impresora.

## Aplicar los cambios (solo reiniciar)

Editar el `config.yaml` **no** requiere recompilar; basta reiniciar el servicio
para que relea la config:

```bash
# Linux (servicio systemd)
sudo systemctl restart tera-agent
sudo systemctl status tera-agent --no-pager
```

En Windows: cierra la app (bandeja → Salir) y vuelve a abrirla.

Luego abre el panel (**http://127.0.0.1:9180**) → **Probar** en la impresora y
verifica. Repite ajustando los dos valores hasta que quede el corte justo debajo
del contenido y un margen mínimo arriba.

## Cambiar los DEFAULTS internos (opcional, requiere recompilar)

Solo si quieres cambiar el valor por defecto para instalaciones nuevas. En
[`raster.go`](../internal/adapters/printing/encoder/escpos/raster.go):

```go
const (
    defaultCutFeedDots   = 232
    defaultTopMarginDots = 16
)
```

Recompilar e instalar:

```bash
# Linux
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o dist/tera-agent-linux ./cmd/tera-agent
sudo systemctl stop tera-agent
sudo install -m 0755 dist/tera-agent-linux /usr/local/bin/tera-agent
sudo systemctl start tera-agent

# Windows
GOOS=windows GOARCH=amd64 go build -o dist/tera-agent.exe ./cmd/tera-agent
```

> Ojo: `systemctl restart` **no** cambia el binario; para una nueva compilación
> hay que `stop` + `install` + `start`. Para cambiar solo la calibración, NO
> necesitas esto: edita el `config.yaml` y reinicia.

## Verificación por tests

```bash
go test ./internal/adapters/printing/encoder/... ./internal/adapters/config/...
```
