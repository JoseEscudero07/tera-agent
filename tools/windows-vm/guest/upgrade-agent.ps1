# Instala el mismo instalador ENCIMA, en silencio y SIN /MODE, como un técnico
# que actualiza un equipo que ya funciona (se ejecuta DENTRO de Windows).
#
# Imprime "UPGRADED servicio=<bool> autoarranque=<bool> token=<bool>": el modo
# tiene que ser el mismo de antes y el registro tiene que seguir ahí.
param(
  [Parameter(Mandatory = $true)][string]$Setup,
  [ValidateSet('user', 'service')][string]$Mode = 'user'
)
$ErrorActionPreference = 'Stop'
$t = 'C:\Tests'

$p = Start-Process "$t\$Setup" -Wait -PassThru `
  -ArgumentList '/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', "/LOG=$t\setup-upgrade.log"
if ($p.ExitCode -ne 0) { throw "el instalador salió con $($p.ExitCode); ver $t\setup-upgrade.log" }

$svc = [bool](Get-Service TeraAgent -ErrorAction SilentlyContinue)
$run = [bool](Get-ItemProperty 'HKLM:\Software\Microsoft\Windows\CurrentVersion\Run' `
  -Name TeraAgent -ErrorAction SilentlyContinue)
$cfg = if ($Mode -eq 'service') { "$env:ProgramData\TeraAgent\config.yaml" }
       else { "$env:LOCALAPPDATA\TeraAgent\config.yaml" }
$tok = (Test-Path $cfg) -and ((Get-Content $cfg -Raw) -match "token:\s*'[^']+'")
"UPGRADED servicio=$svc autoarranque=$run token=$tok"
