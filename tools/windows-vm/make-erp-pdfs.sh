#!/usr/bin/env bash
# Genera facturas REALES con el generador del ERP (fpdf2, FacturaA4Report: logo,
# QR de la DIAN, fuentes Helvetica sin incrustar) para que test.sh las imprima.
# Owner: QA Engineer.
#
#   tools/windows-vm/make-erp-pdfs.sh
#
# Necesita el contenedor del backend del ERP en marcha (por defecto tera_backend,
# cámbialo con TERA_BACKEND_CONTAINER). No toca la base de datos: el generador
# trabaja con un diccionario. Deja los PDF en $TERA_VM_HOME/pdfs.
set -euo pipefail

container="${TERA_BACKEND_CONTAINER:-tera_backend}"
export TERA_VM_HOME="${TERA_VM_HOME:-${XDG_DATA_HOME:-$HOME/.local/share}/tera-agent-vm}"
out="$TERA_VM_HOME/pdfs"
mkdir -p "$out"

docker exec "$container" mkdir -p /tmp/tera-pdfs
docker exec -i -w /app -e DJANGO_SETTINGS_MODULE="${TERA_DJANGO_SETTINGS:-Fudo.settings.local}" "$container" python - <<'PY'
import os, sys
sys.path.insert(0, os.getcwd())
import django; django.setup()
from PIL import Image, ImageDraw
from apps.facturacion.pdf.factura import FacturaA4Report

logo = "/tmp/tera-pdfs/logo.png"
img = Image.new("RGB", (900, 360), "#0B5394")
d = ImageDraw.Draw(img)
d.ellipse((60, 60, 300, 300), fill="#F1C232")
d.rectangle((360, 150, 840, 210), fill="white")
img.save(logo)

def factura(fmt, n):
    items = [{"codigo": f"P-{i:05d}", "descripcion": f"Producto de prueba número {i} con una descripción larga de ejemplo",
              "cantidad": "2", "unidad": "UND", "valorUnitario": "12345.00", "precio": "12345.00", "descuento": "0",
              "iva": "19", "porcentajeIva": "19", "total": "24690.00", "subtotal": "24690.00"} for i in range(1, n + 1)]
    return {"format": fmt,
            "empresa": {"emi_razonSocial": "EMPRESA DE PRUEBA S.A.S.", "emi_nit": "900123456-7", "emi_logoUrl": logo,
                        "colorPrincipal": "#0B5394", "colorAcento": "#F1C232"},
            "factura": {"numero": "FE-000123", "fechaEmision": "2026-01-01", "fechaVencimiento": "2026-02-01",
                        "formaPago": "Crédito", "medioPago": "Transferencia", "cufe": "7cf15f9f" * 12},
            "cliente": {"razonSocial": "CLIENTE DE PRUEBA LTDA", "nit": "800111222-3", "direccion": "Calle 1 # 2-3",
                        "ciudad": "Bogotá", "departamento": "Cundinamarca", "telefono": "3000000000", "email": "a@b.co"},
            "qrUrl": "https://catalogo-vpfe-hab.dian.gov.co/document/searchqr?documentkey=" + "7cf15f9f" * 8,
            "items": items}

# Una página en carta, varias en A4 y una larga en carta (la que agotaba la
# memoria en el modo imagen).
for name, fmt, n in (("fpdf-carta-1pag", "letter", 5), ("fpdf-a4-12pag", "A4", 250), ("fpdf-carta-29pag", "letter", 600)):
    open(f"/tmp/tera-pdfs/{name}.pdf", "wb").write(FacturaA4Report.generar(factura(fmt, n)))
    print(name)
PY
for f in fpdf-carta-1pag fpdf-a4-12pag fpdf-carta-29pag; do
  docker cp "$container:/tmp/tera-pdfs/$f.pdf" "$out/$f.pdf" >/dev/null
done
docker exec "$container" rm -rf /tmp/tera-pdfs
ls -la "$out"
