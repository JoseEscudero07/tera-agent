# Binarización para impresoras térmicas (203 DPI) — Informe técnico

> Decisión de la etapa **Renderer → Encoder**: cómo convertir el raster en
> escala de grises a **1 bit** (punto encendido/apagado) para térmicas de 203 DPI.
> Prioridad del Architect: **(1) texto legible, (2) QR escaneable, (3) barcode
> fiable, (4) velocidad, (5) memoria.** Recomendación antes de implementar.

## 1. Contexto físico

- 203 DPI = **8 puntos/mm**. Un cabezal térmico solo hace **negro o nada** (sin gris).
- **Dot gain / bleed térmico**: el punto impreso “crece” y se difumina → la imagen
  tiende a **oscurecerse**. Los algoritmos que añaden muchos puntos negros
  (difusión de error completa) producen resultados **embarrados** en térmica.
- Un recibo típico de POS **no es fotográfico**: es **texto + QR + código de
  barras**, es decir *line-art* de alto contraste, más un logo ocasional.

**Insight clave:** el _dithering_ existe para simular **tono continuo** (fotos).
Aplicarlo a texto y códigos —que ya son de alto contraste— **los degrada**:
dispersa los bordes en ruido y rompe los módulos de QR/barcode. Por tanto, para
el contenido dominante de un recibo, **el umbral es superior al dithering**.

## 2. Comparativa

Página ~576×1600 px (~0.9 MP). Tiempos/memoria en ese orden de magnitud en Go.

| Algoritmo | Texto pequeño | QR | Barcode | Foto/Logo | Velocidad | Memoria | Veredicto |
|---|---|---|---|---|---|---|---|
| **Threshold fijo** | Bueno (depende del valor) | Excelente | Excelente | Malo | Máxima (1 pasada) | Mínima | Base, pero valor manual frágil |
| **Otsu (umbral global adaptativo)** | **Muy bueno** | **Excelente** | **Excelente** | Malo | Muy alta (histograma+1 pasada) | Mínima | **Recomendado por defecto** |
| **Sauvola/Niblack (umbral local)** | Muy bueno | Excelente | Excelente | Regular | Media (ventanas) | Media | Útil si el origen tiene sombreado |
| **Floyd–Steinberg** | Malo (bordes ruidosos) | Malo (rompe módulos) | Malo | Muy bueno | Alta | Baja (2 filas) | Solo foto; **no** para recibos |
| **Atkinson** | Regular | Malo | Malo | **Excelente en térmica** | Alta | Baja | **Mejor opción para logos/fotos** |
| **Ordered (Bayer)** | Malo (patrón visible) | Malo (moiré) | Malo | Regular | Máxima | Mínima | Descartado en recibos |
| **Sierra** | Malo | Malo | Malo | Muy bueno | Media | Media (3 filas) | Kernel ancho → más blur; descartado |
| **Jarvis-Judice-Ninke** | Muy malo | Muy malo | Muy malo | Muy bueno | Baja | Media (3 filas) | El más borroso; descartado |

### Por qué Otsu como umbral
Elige **automáticamente** el punto de corte que maximiza la separación entre
fondo y tinta (imagen bimodal texto-sobre-blanco). Evita el valor fijo frágil,
preserva **bordes nítidos** (clave para texto y módulos de códigos), y es
**O(n) en una pasada** con 256 bins de histograma → memoria despreciable.

### Por qué Atkinson (y no Floyd–Steinberg) para fotos/logos en térmica
Atkinson difunde **solo 6/8 del error** (no todo). Resultado: **mayor contraste**,
mantiene blancos limpios y negros sólidos, y **usa menos tinta** → encaja con el
*dot gain* térmico y evita el aspecto embarrado de Floyd–Steinberg. Fue diseñado
precisamente para dispositivos de 1 bit.

## 3. Regla crítica (independiente del algoritmo)

Para QR y códigos de barras, lo que más importa **no** es el dithering sino el
**escalado**:

- Rasterizar al **ancho nativo en puntos** (p. ej. 576) y hacer que cada módulo
  del código ocupe un **número entero de puntos** (idealmente ≥ 3–4 puntos/módulo
  a 203 DPI). Así el umbral los reproduce **perfectamente**.
- **Nunca** difuminar (dither) las regiones de códigos ni de texto.

Esto es tan importante como la elección del binarizador para que un QR sea
escaneable y un barcode fiable.

## 4. Recomendación

1. **Por defecto: Otsu (umbral global adaptativo).** Cumple prioridades 1–3
   (texto/QR/barcode nítidos), es de las más rápidas y de menor memoria (4–5).
2. **Alternativa enchufable: Atkinson**, seleccionable por perfil/opciones para
   trabajos con **logo o imagen** dominante.
3. **`Sauvola` (umbral local)** como tercera estrategia opcional, por si algún
   origen llega con sombreado/gradiente de fondo.
4. **Escalado entero al ancho nativo** en el Renderer y **exclusión de dither**
   para texto/códigos.
5. **Compensación de dot gain** opcional (leve erosión morfológica 1px) como
   parámetro, no por defecto.

### Arquitectura (coherente con el motor extensible)

```go
// domain/printing
type Binarizer interface {
    // Convierte gris (0..255) a 1 bit; true = punto encendido (negro).
    Binarize(img image.Image) *image.Paletted // o [][]bool / bitmap 1bpp
    Name() string
}
```

Implementaciones en `adapters/printing/binarizer/`: `OtsuBinarizer` (default),
`AtkinsonBinarizer`, `ThresholdBinarizer`, `SauvolaBinarizer`. La estrategia se
elige por `PrinterProfile`/`Options`; **añadir una nueva no toca el Core**. A
futuro, binarización **por región** (texto→Otsu, imagen→Atkinson) sin cambiar la
interfaz.

## 5. Validación (antes de dar por buena la calidad)

Imprimir un **patrón de prueba** en la térmica real y verificar:
- Texto 8–10 pt legible sin “baba”.
- **QR escaneado** con un móvil a varias distancias.
- **Barcode (EAN/Code128) leído** con lector.
- Comparar Otsu vs Atkinson en el mismo patrón.

Métrica objetiva mínima: % de QR/barcodes escaneados a la primera sobre N tiradas.

## 6. Conclusión

Para térmicas de 203 DPI y contenido de recibo, **la mejor estrategia no es la
más sofisticada de dithering, sino el umbral adaptativo (Otsu)** con escalado
entero, reservando **Atkinson** para imágenes. Esto maximiza legibilidad de texto
y fiabilidad de códigos —las prioridades del producto— con coste mínimo de CPU y
memoria, y encaja en el motor como una estrategia `Binarizer` intercambiable.
