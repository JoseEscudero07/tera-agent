# Impresoras virtuales de prueba (se ejecuta DENTRO de Windows). Escriben a
# fichero, así se puede inspeccionar exactamente lo que recibió cada una:
#   TERMICA-PRUEBA        Generic / Text Only  -> C:\Tests\termica.prn  (ESC/POS crudo)
#   LASER-PRUEBA          Microsoft Print To PDF -> C:\Tests\laser.pdf  (driver GDI)
#   LÁSER Facturación Ñ   igual, con tildes y ñ en el nombre (manifiesto UTF-8 de pdftocairo)
$ErrorActionPreference = 'Stop'
New-Item -ItemType Directory -Force C:\Tests | Out-Null
Add-PrinterDriver -Name 'Generic / Text Only' -ErrorAction SilentlyContinue

$printers = @(
  @{ Name = 'TERMICA-PRUEBA'; Driver = 'Generic / Text Only'; Port = 'C:\Tests\termica.prn' },
  @{ Name = 'LASER-PRUEBA'; Driver = 'Microsoft Print To PDF'; Port = 'C:\Tests\laser.pdf' },
  @{ Name = 'LÁSER Facturación Ñ'; Driver = 'Microsoft Print To PDF'; Port = 'C:\Tests\acentos.pdf' }
)
foreach ($p in $printers) {
  if (-not (Get-PrinterPort -Name $p.Port -ErrorAction SilentlyContinue)) { Add-PrinterPort -Name $p.Port }
  if (-not (Get-Printer -Name $p.Name -ErrorAction SilentlyContinue)) {
    Add-Printer -Name $p.Name -DriverName $p.Driver -PortName $p.Port
  }
}
Get-Printer | Select-Object Name, DriverName, PortName | Format-Table -AutoSize | Out-String
