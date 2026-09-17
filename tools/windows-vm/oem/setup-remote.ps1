# Prepara la VM para manejarla por comandos desde Linux (WinRM) SIN instalar
# nada que pueda tapar un fallo del instalador: ni runtimes, ni poppler, ni Go.
$ErrorActionPreference = 'Continue'

# WinRM rechaza conexiones si la red es "Pública".
Get-NetConnectionProfile | Set-NetConnectionProfile -NetworkCategory Private

Enable-PSRemoting -Force -SkipNetworkProfileCheck
Set-Item WSMan:\localhost\Service\AllowUnencrypted -Value $true
Set-Item WSMan:\localhost\Service\Auth\Basic -Value $true
# Sin esto una cuenta local de administrador entra por WinRM con token filtrado
# (sin elevación) y no podría instalar servicios.
New-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System' `
  -Name LocalAccountTokenFilterPolicy -Value 1 -PropertyType DWord -Force | Out-Null
New-NetFirewallRule -Name 'WinRM-TeraTest' -DisplayName 'WinRM (VM de pruebas)' `
  -Direction Inbound -Protocol TCP -LocalPort 5985 -Action Allow -Profile Any | Out-Null

# Windows Update descargando y reiniciando a mitad de una prueba da falsos fallos.
reg add 'HKLM\SOFTWARE\Policies\Microsoft\Windows\WindowsUpdate\AU' /v NoAutoUpdate /t REG_DWORD /d 1 /f

# Foto del estado "limpio": si el runtime de VC++ ya estuviera en System32, la
# prueba del bundle app-local no demostraría nada.
$sys = "$env:SystemRoot\System32"
$report = @(
  "windows: $((Get-CimInstance Win32_OperatingSystem).Caption) $((Get-CimInstance Win32_OperatingSystem).BuildNumber)",
  "culture: $((Get-Culture).Name)"
)
foreach ($dll in 'msvcp140.dll', 'vcruntime140.dll', 'vcruntime140_1.dll', 'ucrtbase.dll') {
  $report += "${dll}: $(Test-Path (Join-Path $sys $dll))"
}
$report | Set-Content C:\OEM\READY.txt
