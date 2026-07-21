# Ajustar el corte del papel (ESC/POS)

Guía para calibrar **dónde cae el corte** en las impresoras térmicas y volver a
desplegar el agente. Pensada para hacerlo tú mismo las veces que haga falta.

## El problema

La cuchilla está unos **10–18 mm por encima** del cabezal de impresión (varía por
modelo). Si tras imprimir no avanzamos suficiente papel antes de cortar, el corte
cae **sobre la última línea** (p. ej. la fecha del ticket de prueba sale partida
por la mitad). Si avanzamos de más, queda **papel en blanco** de sobra.

Por eso, antes de cortar, el agente hace un avance controlado (`ESC J n`, alimentar
`n` puntos) y luego corta (`GS V 0`).

## El único valor a tocar

Archivo: [`internal/adapters/printing/encoder/escpos/raster.go`](../internal/adapters/printing/encoder/escpos/raster.go)

```go
const cutFeedDots = 200 // ~25mm at 203dpi
```

Regla:

| Síntoma                                             | Acción            |
|-----------------------------------------------------|-------------------|
| Corta el contenido / la última línea sale partida   | **SUBIR** el número |
| Deja demasiado papel en blanco tras el corte        | **BAJAR** el número |

Conversión: a 203 dpi, **1 mm ≈ 8 dots**. Ejemplos: 120 ≈ 15 mm, 160 ≈ 20 mm,
200 ≈ 25 mm, 240 ≈ 30 mm.

⚠️ **Límite: 0–255.** `ESC J` usa un solo byte, así que el máximo real es `255`
(≈ 31 mm). No pongas más de 255.

> Este valor lo usan por igual los tickets renderizados (raster) y el ticket de
> texto de prueba, así que se calibra una sola vez.

## Recompilar y volver a montar el panel

Desde la raíz del repo (`/workspace/proyectos/tera-agent`):

```bash
# 1) Compilar el binario de Linux
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o dist/tera-agent-linux ./cmd/tera-agent

# 2) Instalar el binario nuevo (necesita sudo).
#    Hay que PARAR el servicio antes de copiar: no se puede sobrescribir un
#    binario en ejecución ("text file busy").
sudo systemctl stop tera-agent
sudo install -m 0755 dist/tera-agent-linux /usr/local/bin/tera-agent
sudo systemctl start tera-agent

# 3) Comprobar que arrancó
sudo systemctl status tera-agent --no-pager
```

Abre el panel en **http://127.0.0.1:9180** y pulsa **Probar** en una impresora
para verificar el corte. Repite subiendo/bajando `cutFeedDots` hasta que quede
justo debajo del contenido.

> Ojo: `systemctl restart` **no** cambia el binario; hay que hacer el `stop` +
> `install` + `start` de arriba para que tome la nueva compilación.

### Para Windows

```bash
GOOS=windows GOARCH=amd64 go build -o dist/tera-agent.exe ./cmd/tera-agent
```

Copia `dist/tera-agent.exe` al equipo, cierra la app (icono de bandeja → Salir) y
vuelve a abrirla.

## Verificación rápida por tests

El test del encoder comprueba que siempre se alimenta antes de cortar (no valida
el valor exacto, para que puedas calibrarlo sin romper tests):

```bash
go test ./internal/adapters/printing/encoder/...
```
