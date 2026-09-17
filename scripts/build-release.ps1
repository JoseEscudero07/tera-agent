# Build de release de Tera Agent para Windows. Owner: DevOps Engineer.
#
#   .\scripts\build-release.ps1 -Version 1.0.0
#   .\scripts\build-release.ps1 -Version 1.0.0 -SkipInstaller   # solo binarios
#
# Produce en dist\:
#   tera-agent.exe                  binario de consola (servicio, CLI, diagnostico)
#   tera-agent-tray.exe             mismo programa sin consola (autoarranque/bandeja)
#   TeraAgent-Setup-<version>.exe   instalador (requiere Inno Setup 6)
#   SHA256SUMS.txt                  hashes para el manifiesto /agent/version del ERP
#
# La version NUNCA se escribe en el codigo: se inyecta aqui con -ldflags. Un
# binario compilado sin este script reporta "0.0.0-dev" al ERP a proposito.
param(
  [Parameter(Mandatory = $true)][string]$Version,
  [string]$PopplerBin = "C:\Program Files\poppler-26.09.0\Library\bin",
  # Licencia de Poppler (GPL v2). OJO: share\poppler\COPYING NO lo es; es el aviso
  # de los datos de poppler-data.
  [string]$PopplerLicense = "C:\Program Files\poppler-26.09.0\share\poppler\COPYING.gpl2",
  # Ruta a ISCC.exe. Por defecto se busca en Program Files; pasala si usas una
  # copia portable de Inno Setup (instalada con /PORTABLE=1).
  [string]$Iscc,
  [switch]$SkipInstaller
)
$ErrorActionPreference = "Stop"

$repo    = Split-Path -Parent $PSScriptRoot
$dist    = Join-Path $repo "dist"
$staging = Join-Path $repo "installer\staging"
$pkg     = "github.com/teraerp/tera-agent/internal/infra/di"

# Escribe dist\SHA256SUMS.txt con los artefactos que existan. El campo sha256 del
# manifiesto /agent/version es obligatorio: el Applier se niega a instalar una
# descarga que no pueda verificar, asi que estos hashes son parte del release.
function Write-SumsFile {
  param([string]$Dir, [string]$Ver)
  $names = @("tera-agent.exe", "tera-agent-tray.exe", "TeraAgent-Setup-$Ver.exe")
  $sums  = Join-Path $Dir "SHA256SUMS.txt"
  $names |
    ForEach-Object { Join-Path $Dir $_ } |
    Where-Object { Test-Path $_ } |
    ForEach-Object {
      "{0}  {1}" -f (Get-FileHash $_ -Algorithm SHA256).Hash.ToLower(), (Split-Path $_ -Leaf)
    } |
    Set-Content -Encoding ASCII $sums
  return $sums
}

# Rechazamos versiones que no sean semver: el comparador del modulo update y el
# manifiesto del ERP asumen X.Y.Z, y una version mal formada rompe el auto-update
# de todos los clientes en silencio.
if ($Version -notmatch '^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$') {
  throw "Version '$Version' no es semver valido (esperado X.Y.Z o X.Y.Z-pre)"
}

Write-Host "== Tera Agent $Version ==" -ForegroundColor Cyan

# --- 1. Binarios -------------------------------------------------------------
New-Item -ItemType Directory -Force $dist | Out-Null
$ldflags = "-s -w -X $pkg.AgentVersion=$Version"

Write-Host "-> compilando tera-agent.exe (consola)"
& go build -trimpath -ldflags $ldflags -o (Join-Path $dist "tera-agent.exe") ./cmd/tera-agent
if ($LASTEXITCODE -ne 0) { throw "go build (consola) fallo" }

# -H=windowsgui evita la ventana de consola negra al iniciar sesion. Es el mismo
# codigo: solo cambia el subsistema declarado en el PE.
Write-Host "-> compilando tera-agent-tray.exe (sin consola)"
& go build -trimpath -ldflags "$ldflags -H=windowsgui" -o (Join-Path $dist "tera-agent-tray.exe") ./cmd/tera-agent
if ($LASTEXITCODE -ne 0) { throw "go build (gui) fallo" }

# Comprobacion de humo: que el binario reporte la version inyectada. Si esto
# falla, el ldflags no cuajo y el auto-update quedaria roto en produccion.
$reported = (& (Join-Path $dist "tera-agent.exe") version) -join ""
if ($reported -notmatch [regex]::Escape($Version)) {
  throw "El binario reporta '$reported'; se esperaba la version $Version"
}
Write-Host "   version verificada: $reported" -ForegroundColor Green

if ($SkipInstaller) {
  Write-Host "-> instalador omitido (-SkipInstaller)" -ForegroundColor Yellow
  Get-Content (Write-SumsFile -Dir $dist -Ver $Version)
  exit 0
}

# --- 2. Staging del instalador ----------------------------------------------
# Se empaquetan dos herramientas de poppler:
#   pdftoppm.exe    rasteriza PDF (tickets termicos y modo imagen de las laser).
#   pdftocairo.exe  imprime PDF en laser/inyeccion en modo vectorial.
#
# tools/popplerbundle copia SOLO las DLLs que cargan (leyendo sus imports, tambien
# los de carga diferida) y pone a pdftocairo.exe el manifiesto UTF-8 que necesita
# para encontrar impresoras con tildes o ene en el nombre. Copiar *.dll como antes
# metia mas de 100 MB de ICU, glib, Kerberos... que no usa nadie.
if (-not (Test-Path $PopplerBin)) {
  throw "No se encuentra poppler en '$PopplerBin'. Descargalo de github.com/oschwartz10612/poppler-windows y pasa -PopplerBin."
}

Write-Host "-> preparando staging"
if (Test-Path $staging) { Remove-Item -Recurse -Force $staging }
$popStage = Join-Path $staging "poppler\bin"
New-Item -ItemType Directory -Force $popStage | Out-Null

Copy-Item (Join-Path $dist "tera-agent.exe")      $staging -Force
Copy-Item (Join-Path $dist "tera-agent-tray.exe") $staging -Force
# PDF de prueba (600 bytes): permite que windows-verify.ps1 y el soporte ejerciten
# el pipeline PDF->raster->ESC/POS en el equipo del cliente sin gastar papel.
Copy-Item (Join-Path $repo "internal\adapters\ui\assets\testpage.pdf") $staging -Force

# Sin pdftoppm.exe o pdftocairo.exe la herramienta falla, y debe: sin pdftocairo
# las laser caerian al modo imagen, que imprime borroso y agota la memoria con
# documentos largos. Un release asi no debe salir.
& go run ./tools/popplerbundle -src $PopplerBin -dst $popStage
if ($LASTEXITCODE -ne 0) { throw "tools/popplerbundle fallo" }

# Runtime de Visual C++ (despliegue "app-local", junto a pdftoppm.exe y pdftocairo.exe).
#
# Los bundles de poppler 26.07+ ya lo traen y popplerbundle lo copia. Los
# anteriores no: en un equipo de desarrollo existe en System32 porque el VC++
# Redistributable esta instalado, pero en un cliente limpio no esta y el proceso
# muere con 0xc0000135 (STATUS_DLL_NOT_FOUND) sin llegar a ejecutar nada. En ese
# caso se toma de System32 de la maquina de build.
#
# Se despliegan junto al ejecutable en vez de exigir el instalador del
# Redistributable: el cargador de Windows busca primero el directorio del propio
# .exe, asi que no hay que instalar nada mas ni pedir reinicio. Microsoft permite
# esta redistribucion app-local de estos ficheros.
$vcRuntime = @("msvcp140.dll", "vcruntime140.dll", "vcruntime140_1.dll")
$vcSource  = Join-Path $env:SystemRoot "System32"
foreach ($dll in $vcRuntime) {
  if (Test-Path (Join-Path $popStage $dll)) { continue }
  $src = Join-Path $vcSource $dll
  if (-not (Test-Path $src)) {
    throw "Falta $dll en el bundle de poppler y en $vcSource. Usa un bundle de poppler 26.07+ o instala el 'Microsoft Visual C++ Redistributable (x64)' en la maquina de build: sin el, la impresion de PDF fallaria en todos los clientes con 0xc0000135."
  }
  Write-Host "   runtime VC++: $dll tomado de $vcSource"
  Copy-Item $src $popStage -Force
}

if (Test-Path $PopplerLicense) {
  Copy-Item $PopplerLicense (Join-Path $staging "poppler\COPYING") -Force
} else {
  # Poppler es GPL: no se distribuye sin su licencia.
  throw "No se encuentra la licencia de poppler en '$PopplerLicense'. Pasa -PopplerLicense (share\poppler\COPYING.gpl2 del bundle)."
}

$fileCount = (Get-ChildItem $popStage -File).Count
$stageMB  = [math]::Round((Get-ChildItem $staging -Recurse -File | Measure-Object Length -Sum).Sum / 1MB, 1)
Write-Host "   poppler: $fileCount ficheros; staging total ${stageMB} MB"

# --- 3. Instalador ----------------------------------------------------------
if ($Iscc) {
  if (-not (Test-Path $Iscc)) { throw "No se encuentra ISCC.exe en '$Iscc'" }
  $iscc = $Iscc
} else {
  $iscc = @(
    "${env:ProgramFiles(x86)}\Inno Setup 6\ISCC.exe",
    "$env:ProgramFiles\Inno Setup 6\ISCC.exe"
  ) | Where-Object { Test-Path $_ } | Select-Object -First 1
}

if (-not $iscc) {
  throw "Inno Setup 6 no esta instalado. Instalalo una vez con:  winget install JRSoftware.InnoSetup  (o pasa -Iscc <ruta a ISCC.exe>)"
}

Write-Host "-> compilando instalador"
& $iscc "/DAgentVersion=$Version" (Join-Path $repo "installer\tera-agent.iss")
if ($LASTEXITCODE -ne 0) { throw "ISCC fallo" }

# --- 4. Hashes para el manifiesto del ERP -----------------------------------
$sums = Write-SumsFile -Dir $dist -Ver $Version

Write-Host "`n== Listo ==" -ForegroundColor Green
$setup = Join-Path $dist "TeraAgent-Setup-$Version.exe"
if (Test-Path $setup) {
  "{0}  {1} MB" -f (Split-Path $setup -Leaf), [math]::Round((Get-Item $setup).Length / 1MB, 1)
}
Get-Content $sums
