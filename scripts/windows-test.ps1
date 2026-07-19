# Tera Agent — validación en Windows real
#
# Uso (PowerShell, en la carpeta con tera-agent.exe):
#   .\windows-test.ps1 -Printer "XP-80"
#   .\windows-test.ps1 -Printer "XP-80" -Exe "C:\ruta\tera-agent.exe"
#
# Los pasos 3-5 IMPRIMEN físicamente. Usa una térmica con papel cargado.
param(
  [Parameter(Mandatory = $true)][string]$Printer,
  [string]$Exe = ".\tera-agent.exe"
)

$ErrorActionPreference = "Continue"

function Section($n, $t) { Write-Host "`n== $n. $t ==" -ForegroundColor Cyan }

Section 1 "Ayuda / comandos"
& $Exe help

Section 2 "Listar impresoras (nombre / sistema / estado / tipo)"
& $Exe printers

Section 3 "Texto ESC/POS (IMPRIME)"
& $Exe text --printer $Printer --text "Tera Agent - prueba Windows OK"

Section 4 "RAW: ticket ESC/POS minimo (IMPRIME)"
# ESC @ (init) + texto + LF + GS V 0 (corte total)
$bytes = [byte[]]@(0x1B, 0x40) `
  + [System.Text.Encoding]::ASCII.GetBytes("RAW OK`n`n`n") `
  + [byte[]]@(0x1D, 0x56, 0x00)
[System.IO.File]::WriteAllBytes("ticket.bin", $bytes)
& $Exe raw --printer $Printer --file ticket.bin

Section 5 "PDF -> ESC/POS raster (IMPRIME; requiere pdftoppm en PATH)"
if (Test-Path "factura.pdf") {
  & $Exe print --printer $Printer --file factura.pdf --paper 80 --cut
} else {
  Write-Host "  (omitido: coloca un 'factura.pdf' junto al script para probarlo)"
}

Section 6 "Dry-run (NO imprime): inspeccionar bytes generados"
& $Exe text --printer $Printer --text "dry-run" --out out.bin
if (Test-Path "out.bin") {
  $len = (Get-Item out.bin).Length
  Write-Host "  Generado out.bin ($len bytes) — deberia empezar por 1B 40 (ESC @)"
}

Write-Host "`nListo. Reporta: que pasos imprimieron, la salida de 'printers' y cualquier error." -ForegroundColor Green
