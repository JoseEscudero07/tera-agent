# Web service HTTP (integración con Django)

Servicio HTTP local del Agent para que un backend (Django) envíe PDFs a imprimir.
Es un adaptador de entrada que usa el mismo motor de impresión que la CLI.
Complementa al futuro transporte WebSocket (agente-inicia); pensado para
integración/pruebas en la misma máquina o LAN.

## Arrancar

```bash
tera-agent serve --addr 127.0.0.1:9100          # solo local (recomendado)
tera-agent serve --addr 0.0.0.0:9100 --token SECRETO   # accesible en la LAN, con token
```

O como parte del agente residente: pon `http.addr` en `config.yaml` y ejecuta
`tera-agent run` (o instálalo como servicio de Windows).

> **Seguridad**: por defecto escucha en `127.0.0.1` (solo local). Si lo expones a
> la red (`0.0.0.0`), usa `--token` y mándalo en `Authorization: Bearer <token>`.

## Endpoints

| Método | Ruta | Descripción |
|---|---|---|
| GET | `/health` | `{"status":"ok"}` |
| GET | `/printers` | `{"printers":[{"id","name","driver","status"}]}` |
| POST | `/print` | Imprime un documento. Ver abajo. |

### POST /print

Acepta **`multipart/form-data`** (recomendado para PDFs) o **`application/json`**.

Campos:

| Campo | Tipo | Por defecto | Descripción |
|---|---|---|---|
| `printer` | string | — (requerido) | Id/nombre de la impresora |
| `file` (multipart) / `content` (JSON base64) | binario | — (requerido) | El documento |
| `format` | string | por extensión / `application/pdf` | MIME de origen |
| `paper` | int | 80 | Ancho térmico mm (58 u 80) |
| `width` | int | — | Ancho en dots (anula `paper`) |
| `cut` | bool | true | Corte al final |
| `drawer` | bool | false | Abrir cajón |

Respuestas: `200 {"status":"printed","printer":"..."}`, `400` (petición inválida),
`401` (token), `502 {"error":{"code":"print_failed","message":"..."}}`.

## Ejemplos

### curl (multipart)

```bash
curl -F "file=@factura.pdf;type=application/pdf" \
     -F "printer=KL200" -F "paper=80" -F "cut=true" \
     http://127.0.0.1:9100/print
```

### Django / Python (requests)

```python
import requests

def imprimir_pdf(agent_url, printer, pdf_bytes, token=None):
    headers = {"Authorization": f"Bearer {token}"} if token else {}
    r = requests.post(
        f"{agent_url}/print",
        files={"file": ("factura.pdf", pdf_bytes, "application/pdf")},
        data={"printer": printer, "paper": "80", "cut": "true"},
        headers=headers,
        timeout=30,
    )
    r.raise_for_status()          # 200 -> impreso
    return r.json()

# En tu vista, tras generar el PDF con FPDF:
#   pdf_bytes = pdf.output(dest="S").encode("latin-1")   # FPDF clásico
#   imprimir_pdf("http://192.168.1.50:9100", "KL200", pdf_bytes)
```

### JSON (base64)

```python
import base64, requests
requests.post("http://127.0.0.1:9100/print", json={
    "printer": "KL200",
    "format": "application/pdf",
    "paper": 80,
    "content": base64.b64encode(pdf_bytes).decode(),
})
```

## Notas

- Genera el PDF a **80mm de ancho** en FPDF: `FPDF('P','mm',(80,alto))`.
- El Agent rasteriza el PDF (`pdftoppm`), binariza con **Otsu** y envía **ESC/POS**
  con corte a la impresora (misma tubería que la CLI). Requiere `poppler-utils`.
- En Linux, la cola de la térmica debe ser **RAW** (ver [INSTALL.md §8](INSTALL.md)).
