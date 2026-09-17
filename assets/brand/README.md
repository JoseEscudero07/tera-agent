# Marca de Tera

`tera-mark.svg` es la **fuente de verdad** del isotipo (la T con la retícula de
cuadros), en un único trazo por forma y con `fill="currentColor"`, para que
herede el color de donde se use. Azul de marca: `#0A66C2`.

De aquí salen, con [`make-icons.sh`](make-icons.sh), todos los iconos de Windows:

| Fichero | Dónde se ve |
|---|---|
| `cmd/tera-agent/assets/tray.ico` | icono de la bandeja del sistema |
| `installer/tera-agent.ico` | instalador, accesos directos, "Aplicaciones" |
| `installer/wizard-small.bmp[@2x]` | cabecera del asistente de instalación |
| `internal/adapters/ui/web/favicon.ico` | pestaña del navegador (respaldo) |
| `internal/adapters/ui/web/favicon.svg` | pestaña del navegador |

El panel no usa ninguno de esos ficheros: lleva el SVG incrustado en
`index.html` como `<symbol id="tera-mark">` y lo reutiliza con `<use>`, así se
pinta con el color del tema y no pesa una petición más.

Si cambia el logo, se reemplaza `tera-mark.svg` y se ejecuta `make-icons.sh`
(necesita ImageMagick y Pillow); no se editan los binarios a mano.
