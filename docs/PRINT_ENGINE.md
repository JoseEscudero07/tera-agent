# Motor de Impresión — Diseño

> Estado: **propuesta de arquitectura** (pendiente de aprobación del Software
> Architect). No es un "impresor de PDFs": es un motor extensible capaz de
> soportar cualquier formato de entrada y cualquier impresora a lo largo de los
> años, añadiendo componentes **sin modificar el Core**.

## Objetivos

- Añadir formatos (PDF, PNG, texto, HTML, ESC/POS, ZPL, EPL, CPCL…) e impresoras
  (térmicas, láser, etiquetas Zebra) **registrando componentes**, no editando el Core.
- **Nunca** existir `if PDF -> ...`. La selección del pipeline es por capacidades.
- Cada etapa con una única responsabilidad y dependencias hacia el dominio.
- El rasterizador (poppler) es un detalle **reemplazable** tras una interfaz.

## Flujo

```
PrintJob → Engine → Resolver → Renderer → Encoder → Driver → Hardware
           (dispatch) (elige   (contenido  (a lenguaje (envía
                       pipeline) → artefacto) del device) bytes)
```

- **Engine** (Print Dispatcher): recibe el `PrintJob`. No conoce PDF, ESC/POS ni
  impresoras. Pide un `Pipeline` al `Resolver` y ejecuta las etapas.
- **Resolver**: decide el pipeline por **capacidades** (formato de entrada +
  perfil/capacidades de la impresora + config del ERP). Extensible.
- **Renderer**: convierte el contenido de entrada en un **Artefacto** intermedio
  (raster/texto/bytes). No conoce impresoras.
- **Encoder**: convierte el artefacto al **lenguaje del dispositivo** (ESC/POS,
  ZPL…). No conoce el transporte.
- **Driver**: **solo envía bytes** al dispositivo. No renderiza, no convierte.

## Modelo de dominio (`internal/domain/printing`)

```go
// Formatos de ENTRADA del documento (lo que manda el ERP).
type SourceFormat string // "application/pdf", "image/png", "text/plain",
                         // "application/vnd.escpos", "application/vnd.zpl"

// Formatos que CONSUME un dispositivo/driver.
type DeviceFormat string // "escpos", "zpl", "epl", "pdf", "png", "raw"

// PrintJob es lo que recibe el Engine.
type PrintJob struct {
    PrinterID string
    Format    SourceFormat
    Content   []byte
    Options   Options
}

type Options struct {
    Copies     int
    Cut        bool
    OpenDrawer bool
}

// Artefacto intermedio entre Renderer y Encoder (Composition over Inheritance).
type ArtifactKind string
const (
    ArtifactRaster ArtifactKind = "raster"
    ArtifactText   ArtifactKind = "text"
    ArtifactBytes  ArtifactKind = "bytes" // pass-through (ya en formato final)
)

type Artifact interface{ Kind() ArtifactKind }

type RasterArtifact struct { Pages []image.Image; WidthDots int } // Kind()=raster
type TextArtifact   struct { Body string }                        // Kind()=text
type BytesArtifact  struct { Data []byte; Format DeviceFormat }   // Kind()=bytes

// PhysicalPrinter es lo que el Agent DETECTA y reporta al ERP. Solo hechos
// físicos; ninguna lógica de negocio.
type PhysicalPrinter struct {
    Name    string
    Driver  string
    Port    string
    Status  Status
    DPI     int // si es detectable
}

// PrinterProfile pertenece al ERP (catálogo de impresoras configurado por el
// administrador). El Agent NUNCA lo descubre ni lo persiste como lógica: lo
// recibe del Backend y lo mantiene en memoria (ver ProfileProvider).
type PrinterProfile struct {
    PrinterID      string
    NativeFormats  []DeviceFormat // lo que el device consume directamente, en prioridad
    WidthDots      int            // p.ej. 576 para 80mm @203dpi
    DPI            int
    SupportsCut    bool
    SupportsDrawer bool
    // Metadatos del catálogo del ERP (tecnología, driver lógico, márgenes, logo,
    // configuración personalizada) viajan en un mapa extensible para no acoplar
    // el Core a campos concretos del ERP.
    Meta map[string]string
}
```

## Puertos (interfaces del dominio)

```go
type Renderer interface {
    CanRender(SourceFormat) bool
    Produces() ArtifactKind
    Render(ctx context.Context, content []byte, opts RenderOptions) (Artifact, error)
}

type Encoder interface {
    Accepts() ArtifactKind
    Produces() DeviceFormat
    Encode(ctx context.Context, a Artifact, opts EncodeOptions) ([]byte, error)
}

type Driver interface {
    Accepts(DeviceFormat) bool
    Send(ctx context.Context, printerID string, data []byte) error
}

// Rasterizador REEMPLAZABLE (poppler hoy; mupdf/pdfium/ghostscript mañana).
type Rasterizer interface {
    Rasterize(ctx context.Context, pdf []byte, opts RasterOptions) ([]image.Image, error)
}

// ProfileProvider entrega el perfil de una impresora desde una CACHÉ EN MEMORIA.
// La caché la puebla/actualiza la capa de comunicación cuando el Backend envía
// los perfiles (al autenticar y cuando cambian). El job solo trae printer_id.
type ProfileProvider interface {
    Profile(printerID string) (PrinterProfile, error)
}

// ProfileCache es la parte de escritura, usada por la capa de comunicación.
type ProfileCache interface {
    ProfileProvider
    Set(PrinterProfile)          // upsert al recibir del Backend
    SetAll([]PrinterProfile)     // sincronización completa
}

type Pipeline struct {
    Renderer Renderer
    Encoder  Encoder
    Driver   Driver
}

type Resolver interface {
    Resolve(format SourceFormat, profile PrinterProfile) (Pipeline, error)
}

type RenderOptions struct { WidthDots, DPI int; Color bool }
type EncodeOptions struct { WidthDots int; Cut, OpenDrawer bool }
type RasterOptions struct { WidthDots, DPI int; Gray bool }
```

## Resolver — composición por capacidades (sin `if PDF`)

El Resolver mantiene registros de `Renderer`, `Encoder` y `Driver`. Compone la
cadena emparejando **salidas con entradas**:

```
Resolve(format, profile):
  para cada targetFormat en profile.NativeFormats (por prioridad):
     driver = driver que Accepts(targetFormat); si no hay, siguiente
     si format ya es targetFormat:               # p.ej. PDF a impresora que consume PDF
        return Pipeline{PassThroughRenderer, PassThroughEncoder(targetFormat), driver}
     para cada encoder con Produces()==targetFormat:
        para cada renderer con CanRender(format) y Produces()==encoder.Accepts():
           return Pipeline{renderer, encoder, driver}
  error: no hay pipeline para (format → profile)
```

Añadir un formato/impresora = **registrar** un Renderer/Encoder/Driver nuevo; el
Resolver descubre los caminos automáticamente. No se toca el Engine ni el Core.

## Ejecución (Engine)

```
Engine.Print(ctx, job):
  profile  = profiles.Profile(job.PrinterID)
  pipeline = resolver.Resolve(job.Format, profile)
  art      = pipeline.Renderer.Render(ctx, job.Content, renderOpts(profile))
  bytes    = pipeline.Encoder.Encode(ctx, art, encodeOpts(profile, job.Options))
  return pipeline.Driver.Send(ctx, job.PrinterID, bytes)
```

## Pipelines de ejemplo (todos, mismo Engine)

| Entrada | Perfil impresora | Renderer | Encoder | Driver |
|---|---|---|---|---|
| PDF | térmica `escpos`, 576 dots | PDFRaster (Rasterizer) | ESCPOSRaster (+corte) | CUPS raw |
| PDF | oficina `pdf` | PassThrough | PassThrough(pdf) | CUPS nativo |
| PNG | térmica `escpos` | Image | ESCPOSRaster | CUPS raw |
| Texto | térmica `escpos` | Text | ESCPOSText | CUPS raw |
| PDF | Zebra `zpl` | PDFRaster | **ZPLRaster** (nuevo) | CUPS raw |

> El **mismo PDF** de tu backend (FPDF) va a `ESC/POS raster` en la térmica o a
> `PDF nativo` en una láser **según el perfil**, sin una sola condición sobre "PDF".
> Soportar Zebra = añadir `ZPLRasterEncoder` + un perfil `zpl`; nada más cambia.

### Impresoras de hoja en Windows: vectorial o imagen

Windows no tiene un "CUPS nativo" que acepte PDF, así que el perfil `pdf` del ERP
se traduce **al leerlo** (`platform.NormalizingProfileCache`) según el modo de la
impresora (`ports.PageMode`, editable en el panel):

| Modo | Formatos tras normalizar | Pipeline de un PDF |
|---|---|---|
| `vector` (defecto) | `[pdf, gdi-raster]` | PassThrough → `driver/pdfvector` (`pdftocairo -print -noshrink`) |
| `image` | `[gdi-raster]` | PDFRaster → MultiPNG → `driver/gdi` (StretchDIBits) |

- `gdi-raster` queda detrás de `pdf` en vectorial: los PNG/JPEG solo tienen ese
  camino, y si `pdftocairo` no está instalado `pdfvector.Accepts(pdf)` es falso y
  el resolver cae a imagen **al componer**, antes de enviar nada (nunca imprime
  dos veces).
- Se normaliza al leer, no al guardar, para que cambiar el modo en el panel valga
  desde el siguiente trabajo sin olvidar el perfil que envió el ERP.
- `platform.EffectivePageMode` manda a imagen las impresoras con nombre no ASCII
  en Windows anteriores a 10 1903: `pdftocairo` usa las API ANSI y solo encuentra
  esos nombres gracias al manifiesto UTF-8 que le pone `tools/popplerbundle`.

Por qué vectorial por defecto: el camino raster decodificaba todas las páginas a
~600 ppp dos veces en memoria (8,6 GB comprometidos con 29 páginas) y el driver
GDI reducía las páginas con color a ~170 ppp para caber en el presupuesto de DIB
de los drivers host-based. Con `pdftocairo`, 29 páginas en 25 s y 24 MB, con el
texto idéntico al PDF.

## Ubicación (Clean Architecture)

```
internal/domain/printing/         Tipos + puertos (Renderer, Encoder, Driver,
                                   Rasterizer, Resolver, Pipeline, PrinterProfile)
internal/app/print/               Engine (dispatcher) + Resolver + ProfileProvider
internal/adapters/printing/
    rasterizer/poppler/           PopplerRasterizer (exec pdftoppm)   ← reemplazable
    popplerbin/                   buscar/lanzar pdftoppm y pdftocairo (compartido)
    renderer/                     PDFRaster, Image, Text, PassThrough
    encoder/escpos/               ESCPOSRaster, ESCPOSText  (Go stdlib puro)
    encoder/zpl/                  (futuro)
    driver/cups/                  CUPSRawDriver, CUPSNativeDriver
    driver/spooler/               Windows RAW (winspool)
    driver/pdfvector/             Windows: PDF vectorial en láser (pdftocairo)
    driver/gdi/                   Windows: páginas raster por GDI (modo imagen)
    platform/                     drivers y normalización de perfiles por SO
    profile/                      ProfileProvider desde config del ERP
```

El registro de componentes y el perfil por impresora se cablean en
`internal/infra/di` (composition root). El handler de `job.KindPrint` adapta el
`job.Job` entrante a un `PrintJob` y llama a `Engine.Print`.

## Alcance del MVP (primer pipeline)

Implementar, tras aprobación:

1. Puertos del dominio (arriba) y el `Engine` + `Resolver` (registro/composición).
2. `PopplerRasterizer` (tras la interfaz `Rasterizer`).
3. `PDFRasterRenderer`, `ESCPOSRasterEncoder` (GS v 0 + init + corte), `PassThrough`.
4. `CUPSRawDriver` (`lp -o raw`) y `CUPSNativeDriver` (`lp`).
5. `ProfileProvider` desde config (perfil de la térmica 80mm: escpos, 576 dots, corte).
6. Wiring en DI + adaptación del handler PRINT + subcomando `print` del CLI.
7. Tests del `Resolver` y del `ESCPOSRasterEncoder` (QA, pure Go, sin hardware).

Dependencia externa nueva: **poppler-utils** (`pdftoppm`) en el equipo cliente,
aislada tras `Rasterizer`. El código Go sigue usando **solo la stdlib**.

## Fuera de alcance (YAGNI por ahora)

HTMLRenderer, ZPL/EPL/CPCL, colas/reintentos por impresora, spooling persistente.
Quedan como puntos de extensión que **no** requieren tocar el Core.

## Decisiones del Architect (resueltas)

1. **Origen del perfil** — El `PrinterProfile` **pertenece al ERP**. Flujo:
   el Agent arranca → reporta las impresoras físicas detectadas (`PhysicalPrinter`)
   → Django las asocia a su catálogo → el Backend envía los perfiles **una vez**
   (y cuando cambien) → el Agent los mantiene **en memoria** (`ProfileCache`).
   Cada `PrintJob` trae **solo `printer_id`**. El Agent nunca descubre ni persiste
   lógica de negocio de impresoras. (Ver también `docs/PROTOCOL.md`.)
2. **Rasterización** — Poppler (`pdftoppm`) en el MVP, **siempre tras la interfaz
   `Rasterizer`**; sin dependencia directa. Futuro: MuPDF/PDFium/Ghostscript sin
   tocar el resto del sistema.
3. **Conversión a 1 bit** — Estrategia **enchufable** (`Binarizer`), con
   recomendación técnica y comparativa en `docs/BINARIZATION.md`. Prioridad:
   texto legible > QR escaneable > barcode fiable > velocidad > memoria.

## Cuestiones abiertas menores

- **Multipágina en térmica**: ¿imagen continua o corte entre páginas? Propuesta:
  corte configurable por `Options`.
```
