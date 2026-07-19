# Instalador de Tera Agent como servicio de Windows.
# EJECUTAR COMO ADMINISTRADOR (clic derecho en PowerShell -> "Ejecutar como administrador").
#
#   .\windows-install.ps1                       # usa .\tera-agent.exe
#   .\windows-install.ps1 -Source C:\ruta\tera-agent.exe
param(
  [string]$Source = ".\tera-agent.exe",
  [string]$InstallDir = "$env:ProgramFiles\TeraAgent",
  [string]$DataDir = "$env:ProgramData\TeraAgent"
)
$ErrorActionPreference = "Stop"

$admin = ([Security.Principal.WindowsPrincipal] `
  [Security.Principal.WindowsIdentity]::GetCurrent()
).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $admin) { Write-Error "Ejecuta PowerShell como Administrador."; exit 1 }
if (-not (Test-Path $Source)) { Write-Error "No se encuentra $Source"; exit 1 }

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
New-Item -ItemType Directory -Force -Path $DataDir | Out-Null

$exe = Join-Path $InstallDir "tera-agent.exe"
Copy-Item $Source $exe -Force

$cfg = Join-Path $DataDir "config.yaml"
if (-not (Test-Path $cfg)) {
@"
server:
  url: ""
  token: ""
printer:
  default: ""
log:
  level: "info"
"@ | Set-Content -Encoding UTF8 $cfg
}

& $exe service install --config $cfg
& $exe service start

Write-Host "`nTera Agent instalado como servicio 'TeraAgent' y arrancado." -ForegroundColor Green
Write-Host "  Ejecutable: $exe"
Write-Host "  Config:     $cfg"
Write-Host "Para desinstalar:  .\windows-uninstall.ps1"
