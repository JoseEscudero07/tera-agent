# Changelog

Formato basado en [Keep a Changelog](https://keepachangelog.com/es-ES/1.0.0/).

## [Unreleased]

### Added
- Scaffold inicial con Clean Architecture (domain / app / adapters / infra).
- Máquina de estados del Agent con transiciones validadas y observadores.
- Ports de `printing`, `device` y `comms`; dispatcher y lifecycle con handshake
  de autenticación por Token.
- 10 subagentes de desarrollo en `.claude/agents/`.
- **Impresión silenciosa (Linux/macOS)** mediante CUPS (`lp`/`lpstat`), sin
  dependencias externas. Salida de CUPS forzada a `LC_ALL=C` para parseo de
  estado independiente del idioma.
- CLI con subcomandos `run`, `printers` y `print` para probar la impresión
  silenciosa sin backend.
- Stub de impresión para plataformas sin backend (mantiene el cross-compile).
- `Makefile` con build, test, race, cover y cross-compile.
- **Motor de impresión extensible por pipeline**
  (`PrintJob → Engine → Resolver → Renderer → Encoder → Driver`) con resolución
  por capacidades (sin condicionales por formato). Diseño en
  `docs/PRINT_ENGINE.md`.
- Puertos del dominio: `Renderer`, `Encoder`, `Driver`, `Rasterizer`,
  `Binarizer`, `Resolver`, `ProfileProvider`/`ProfileCache`.
- `Rasterizer` con implementación `PopplerRasterizer` (reemplazable).
- Renderers PDF/imagen/texto; encoders ESC/POS raster y texto; drivers CUPS
  raw/nativo y `file` (dry-run).
- **Binarización** con estrategia enchufable: `Otsu` (por defecto), `Atkinson`,
  `Threshold`. Recomendación técnica en `docs/BINARIZATION.md`.
- `PrinterProfile` propiedad del ERP, cacheado en memoria (`ProfileCache`).
- CLI `print` sobre el motor: PDF 80mm → ESC/POS raster con corte, y `-out` para
  dry-run a archivo.
- Tests del Resolver (composición y pass-through), binarizers (Otsu/Threshold/
  Atkinson) y encoder ESC/POS.

### Changed
- La impresión ya no usa un `Document`/`Port` plano: pasa por el motor. Los
  paquetes de `adapters/printing` se reorganizan en subpaquetes
  (rasterizer/renderer/encoder/binarizer/driver/discovery/profile).
