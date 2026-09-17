#!/usr/bin/env bash
# Genera los iconos de Windows a partir de tera-mark.svg (la marca vectorial).
# Owner: DevOps Engineer. Solo hay que ejecutarlo si cambia el logo.
#
#   assets/brand/make-icons.sh
#
# Necesita ImageMagick y Pillow (python3-pil). Escribe:
#   cmd/tera-agent/assets/tray.ico      icono de la bandeja
#   installer/tera-agent.ico            icono del instalador, accesos directos y desinstalador
#   internal/adapters/ui/web/favicon.ico  pestaña del panel (respaldo de favicon.svg)
#   installer/wizard-small.bmp[@2x]     cabecera del asistente (Inno Setup solo admite BMP)
#
# El .ico lleva las entradas pequeñas en BMP y solo la de 256 en PNG: es lo que
# esperan LoadImage (bandeja), el compilador de Inno Setup y el Explorador.
set -euo pipefail
cd "$(dirname "$0")/../.."

blue='#0A66C2'   # azul de la marca
tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT

sed "s/currentColor/$blue/" assets/brand/tera-mark.svg > "$tmp/mark.svg"
magick -background none -density 600 "$tmp/mark.svg" -resize 1024x1024 "png32:$tmp/mark.png"

python3 - "$tmp/mark.png" <<'PY'
import struct, sys
from PIL import Image

SIZES = [16, 20, 24, 32, 48, 64, 128, 256]
src = Image.open(sys.argv[1]).convert('RGBA')


def square(size):
    """La marca centrada en un lienzo cuadrado, sin deformarla."""
    im = src.copy()
    im.thumbnail((size, size), Image.LANCZOS)
    canvas = Image.new('RGBA', (size, size), (0, 0, 0, 0))
    canvas.alpha_composite(im, ((size - im.width) // 2, (size - im.height) // 2))
    return canvas


def bmp_entry(im):
    """BITMAPINFOHEADER + BGRA de abajo arriba + máscara AND, como pide el formato ICO."""
    w, h = im.size
    px = im.load()
    xor = bytearray()
    for y in range(h - 1, -1, -1):
        for x in range(w):
            r, g, b, a = px[x, y]
            xor += bytes((b, g, r, a))
    stride = ((w + 31) // 32) * 4          # filas de la máscara alineadas a 4 bytes
    mask = bytearray()
    for y in range(h - 1, -1, -1):
        row = bytearray(stride)
        for x in range(w):
            if px[x, y][3] == 0:
                row[x // 8] |= 0x80 >> (x % 8)
        mask += row
    header = struct.pack('<IiiHHIIiiII', 40, w, h * 2, 1, 32, 0, len(xor) + len(mask), 0, 0, 0, 0)
    return header + bytes(xor) + bytes(mask)


def png_entry(im):
    from io import BytesIO
    buf = BytesIO()
    im.save(buf, format='PNG', optimize=True)
    return buf.getvalue()


def write_ico(path, sizes):
    blobs = [(s, bmp_entry(square(s)) if s < 256 else png_entry(square(s))) for s in sizes]
    out = bytearray(struct.pack('<HHH', 0, 1, len(blobs)))
    offset = 6 + 16 * len(blobs)
    for size, blob in blobs:
        out += struct.pack('<BBBBHHII', size % 256, size % 256, 0, 0, 1, 32, len(blob), offset)
        offset += len(blob)
    for _, blob in blobs:
        out += blob
    open(path, 'wb').write(out)
    print(f'{path}: {len(blobs)} tamaños, {len(out) // 1024} KB')


# El instalador y los accesos directos sí llegan a los iconos grandes del
# Explorador; la bandeja nunca pinta por encima de 32 px (48 al 200 % de
# escalado) y la pestaña del navegador, de 48. Cada fichero lleva solo lo suyo:
# va embebido en un binario.
write_ico('installer/tera-agent.ico', SIZES)
write_ico('cmd/tera-agent/assets/tray.ico', [16, 20, 24, 32, 48])
write_ico('internal/adapters/ui/web/favicon.ico', [16, 32, 48])

# Cabecera del asistente de Inno Setup: BMP de 24 bits sobre blanco, que es el
# fondo de la página. Se dan los dos tamaños y el instalador elige según el DPI.
for name, side in (('installer/wizard-small.bmp', 55), ('installer/wizard-small@2x.bmp', 110)):
    icon = square(side * 2)
    canvas = Image.new('RGB', (side, side), (255, 255, 255))
    icon.thumbnail((side - 8, side - 8), Image.LANCZOS)
    canvas.paste(icon, ((side - icon.width) // 2, (side - icon.height) // 2), icon)
    canvas.save(name, format='BMP')
    print(f'{name}: {side}x{side}')
PY
