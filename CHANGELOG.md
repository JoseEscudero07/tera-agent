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
