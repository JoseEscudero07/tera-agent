# Instala Tera Agent en la VM como en un cliente y lo registra contra el ERP
# simulado (se ejecuta DENTRO de Windows). Los ficheros llegan por la carpeta
# compartida \\host.lan\Data que prepara test.sh.
param(
  [Parameter(Mandatory = $true)][string]$Setup,
  [ValidateSet('user', 'service')][string]$Mode = 'user'
)
$ErrorActionPreference = 'Stop'
$share = '\\host.lan\Data'
$t = 'C:\Tests'
New-Item -ItemType Directory -Force $t | Out-Null
Copy-Item "$share\$Setup", "$share\mock-server.exe", "$share\windows-verify.ps1" $t -Force
Copy-Item "$share\pdfs" $t -Recurse -Force

& powershell -NoProfile -ExecutionPolicy Bypass -File "$share\guest\setup-printers.ps1" | Out-Null

$p = Start-Process "$t\$Setup" -Wait -PassThru `
  -ArgumentList '/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', "/MODE=$Mode", "/LOG=$t\setup.log"
if ($p.ExitCode -ne 0) { throw "el instalador salió con $($p.ExitCode); ver $t\setup.log" }
"INSTALLED $(& "$env:ProgramFiles\TeraAgent\tera-agent.exe" version 2>&1)"

# Registro contra el mock-server local (lo arranca print-jobs.ps1 en cada trabajo).
$cfg = "$env:ProgramData\TeraAgent\config.yaml"
(Get-Content $cfg) `
  -replace "^  url: ''.*", "  url: 'ws://127.0.0.1:8765/ws/agent/'" `
  -replace "^  token: ''.*# lo emite.*", "  token: 'token-de-prueba'" |
  Set-Content $cfg -Encoding UTF8
