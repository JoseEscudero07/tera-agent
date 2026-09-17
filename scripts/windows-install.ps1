# Despliegue desatendido de Tera Agent en Windows. Owner: DevOps Engineer.
#
# Para instalar en UN equipo, lo normal es hacer doble clic en
# TeraAgent-Setup-<version>.exe y seguir el asistente. Este script es para
# instalar en MUCHOS equipos sin interaccion (GPO, RMM, script de despliegue).
#
#   .\windows-install.ps1                                      # busca el Setup en .\dist
#   .\windows-install.ps1 -Setup \\srv\share\TeraAgent-Setup-1.0.0.exe
#   .\windows-install.ps1 -Mode service                         # equipo desatendido
#   .\windows-install.ps1 -Url https://erp/agent/download/latest/windows/amd64
#
# EJECUTAR COMO ADMINISTRADOR.
#
# NOTA: la version anterior de este script copiaba tera-agent.exe a mano. Ya no:
# copiar solo el binario deja la impresion de PDF rota, porque pdftoppm.exe
# necesita poppler.dll y sus 25 DLLs. El instalador las empaqueta; este script
# solo lo invoca.
param(
  # 'user'    = arranca al iniciar sesion, con bandeja y panel (recomendado).
  # 'service' = servicio de Windows, arranca sin que nadie inicie sesion.
  [ValidateSet("user", "service")][string]$Mode = "user",
  [string]$Setup,
  [string]$Url,
  [switch]$Uninstall
)
$ErrorActionPreference = "Stop"

$admin = ([Security.Principal.WindowsPrincipal] `
    [Security.Principal.WindowsIdentity]::GetCurrent()
).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $admin) { throw "Ejecuta PowerShell como Administrador." }

# --- Desinstalacion ----------------------------------------------------------
# El desinstalador de Inno queda registrado en Uninstall\<AppId>_is1. Buscarlo
# ahi es mas fiable que asumir la ruta de Program Files.
if ($Uninstall) {
  $key = "HKLM:\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\{8F3C1A72-5B4E-4D9A-9E27-3A6D2C8B1F04}_is1"
  if (-not (Test-Path $key)) {
    $key = "HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\{8F3C1A72-5B4E-4D9A-9E27-3A6D2C8B1F04}_is1"
  }
  if (-not (Test-Path $key)) { throw "Tera Agent no aparece instalado en este equipo." }
  $unins = (Get-ItemProperty $key).UninstallString.Trim('"')
  Write-Host "Desinstalando con $unins"
  & $unins /VERYSILENT /SUPPRESSMSGBOXES /NORESTART | Out-Null
  Write-Host "Tera Agent desinstalado. La config y los logs siguen en $env:ProgramData\TeraAgent" -ForegroundColor Green
  return
}

# --- Localizar el instalador -------------------------------------------------
if ($Url) {
  # Descarga desde el ERP. Se guarda en un temporal y se borra al terminar.
  $Setup = Join-Path $env:TEMP "TeraAgent-Setup.exe"
  Write-Host "Descargando el instalador desde $Url"
  Invoke-WebRequest -Uri $Url -OutFile $Setup -UseBasicParsing
}

if (-not $Setup) {
  $repo = Split-Path -Parent $PSScriptRoot
  $Setup = Get-ChildItem (Join-Path $repo "dist") -Filter "TeraAgent-Setup-*.exe" -ErrorAction SilentlyContinue |
    Sort-Object LastWriteTime -Descending | Select-Object -First 1 -ExpandProperty FullName
}
if (-not $Setup -or -not (Test-Path $Setup)) {
  throw "No se encuentra el instalador. Pasa -Setup <ruta>, -Url <url>, o compilalo con scripts\build-release.ps1."
}

# --- Instalar ----------------------------------------------------------------
# /VERYSILENT no muestra nada; /MODE lo lee la funcion ModeParam del .iss.
Write-Host "Instalando $(Split-Path $Setup -Leaf) en modo '$Mode'..."
$p = Start-Process -FilePath $Setup -Wait -PassThru -ArgumentList @(
  "/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART",
  "/MODE=$Mode",
  "/LOG=$env:TEMP\TeraAgent-Setup.log"
)
# 3010 = instalado correctamente, pide reinicio. No es un fallo.
if ($p.ExitCode -notin @(0, 3010)) {
  throw "El instalador devolvio $($p.ExitCode). Revisa $env:TEMP\TeraAgent-Setup.log"
}

Write-Host "`nTera Agent instalado (modo $Mode)." -ForegroundColor Green
if ($Mode -eq "service") {
  sc.exe query TeraAgent | Select-String "STATE"
} else {
  Write-Host "  Arrancara con la bandeja al iniciar sesion el usuario."
}
Write-Host "  Config: $env:ProgramData\TeraAgent\config.yaml"
Write-Host "  Panel:  http://127.0.0.1:9180   <- registra el Token aqui"
