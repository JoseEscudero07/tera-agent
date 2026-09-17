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

# Registro contra el mock-server local (lo arranca print-jobs.ps1 en cada trabajo),
# por el mismo camino que un técnico: `tera-agent register`. Ya no vale editar el
# config.yaml: en servicio vive en una carpeta que solo SYSTEM y Administradores
# pueden tocar, y en modo usuario está en %LOCALAPPDATA% del usuario (este mismo,
# el que inicia sesión y arranca la bandeja). En servicio, register además
# reinicia el servicio. El Token va en un fichero, nunca como argumento.
$tok = "$t\token.txt"
Set-Content $tok 'token-de-prueba' -NoNewline -Encoding ASCII
try {
  # Continue: con Stop, PowerShell 5.1 convierte cualquier línea de stderr de un
  # ejecutable nativo en excepción, antes de poder mirar el código de salida.
  $ErrorActionPreference = 'Continue'
  # "$_" deja solo el texto: sin él, cada línea de stderr sale como un
  # NativeCommandError de PowerShell con su traza.
  $reg = & "$env:ProgramFiles\TeraAgent\tera-agent.exe" register --scope $Mode `
    --url 'ws://127.0.0.1:8765/ws/agent/' --token-file $tok 2>&1 | ForEach-Object { "$_" } | Out-String
  if ($LASTEXITCODE -ne 0) { throw "register salió con ${LASTEXITCODE}: $reg" }
  "REGISTERED $($reg.Trim() -replace '\r?\n', ' | ')"
} finally {
  $ErrorActionPreference = 'Stop'
  Remove-Item $tok -Force -ErrorAction SilentlyContinue
}
