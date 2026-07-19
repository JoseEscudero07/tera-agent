# Validación en Windows real

El driver de Windows (spooler `winspool`) **compila** para amd64/386/arm64 pero
necesita validarse en una máquina Windows real. Esta guía te lo pone fácil.

## Requisitos

- Windows 10/11 (o Server) con el **Servicio de cola de impresión** activo.
- Una impresora térmica **ESC/POS** instalada (p. ej. XP-80) — anota su nombre
  exacto tal como aparece en *Configuración → Impresoras*.
- Para probar PDF: **poppler** con `pdftoppm.exe` en el `PATH`
  (ver [INSTALL.md](INSTALL.md)). No hace falta para `raw`/`text`.

## Pasos

1. Copia `tera-agent.exe` y `scripts/windows-test.ps1` a una carpeta.
2. Abre **PowerShell** en esa carpeta.
3. Ejecuta (sustituye por el nombre real de tu impresora):

   ```powershell
   .\windows-test.ps1 -Printer "XP-80"
   ```

   > Si PowerShell bloquea el script:
   > `powershell -ExecutionPolicy Bypass -File .\windows-test.ps1 -Printer "XP-80"`

## Qué debe pasar (resultados esperados)

| Paso | Esperado |
|---|---|
| 2. `printers` | Lista tu(s) impresora(s) con `SYSTEM=windows`, `TYPE=winspool` y un `STATUS` (READY/BUSY/OFFLINE/ERROR) |
| 3. `text` | Sale un ticket con "Tera Agent - prueba Windows OK" y corta |
| 4. `raw` | Sale un ticket "RAW OK" y corta (bytes ESC/POS enviados sin modificar) |
| 5. `print` (PDF) | El PDF se imprime como ticket térmico (rasterizado → ESC/POS) |
| 6. dry-run | Crea `out.bin` que empieza por `1B 40` (ESC @); no imprime |

## Qué reportar

- Salida completa de `tera-agent printers`.
- Qué pasos imprimieron correctamente y cuáles no.
- Cualquier mensaje de error (cópialo tal cual).
- Modelo/nombre de la impresora y si es 58mm o 80mm.

Con eso ajusto lo que haga falta (nombre de datatype RAW, estado, anchos, corte).

## Notas de implementación (para diagnóstico)

- Impresión RAW directa al spooler: `OpenPrinter` → `StartDocPrinter` (datatype
  `RAW`) → `WritePrinter` → `EndDoc`. No usa el driver gráfico/GDI.
- Enumeración: `EnumPrinters` nivel 4; estado: `GetPrinter` nivel 6.
- Si `raw` funciona pero `print` (PDF) no, casi seguro falta `pdftoppm.exe` en el
  `PATH`.
