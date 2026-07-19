# Desinstalador del servicio Tera Agent. EJECUTAR COMO ADMINISTRADOR.
param(
  [string]$InstallDir = "$env:ProgramFiles\TeraAgent"
)
$ErrorActionPreference = "Continue"

$exe = Join-Path $InstallDir "tera-agent.exe"
if (Test-Path $exe) {
  & $exe service stop
  & $exe service uninstall
} else {
  Write-Warning "No se encontró $exe; intentando via sc.exe"
  sc.exe stop TeraAgent | Out-Null
  sc.exe delete TeraAgent | Out-Null
}
Write-Host "Servicio TeraAgent desinstalado." -ForegroundColor Green
Write-Host "Puedes borrar la carpeta $InstallDir y %ProgramData%\TeraAgent si ya no los necesitas."
