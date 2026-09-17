# Envía PDFs al Agent instalado como lo haría el ERP y guarda lo que recibió la
# impresora (se ejecuta DENTRO de Windows).
#
# Usa examples/mock-server (compilado para Windows) con el perfil de impresora
# "normal" del ERP (--format pdf). Un trabajo por conexión, esperando a que la
# cola de impresión se vacíe, para poder copiar la salida de cada documento.
#
# Salida: una línea "JOB <doc> <resultado> <segundos> pico=<MB>" por documento y
# el fichero recibido en \\host.lan\Data\out\<Prefix>-<doc>.pdf.
param(
  [Parameter(Mandatory = $true)][string]$Docs,     # nombres sin .pdf, separados por comas
  [Parameter(Mandatory = $true)][string]$Prefix,
  [string]$Printer = 'LASER-PRUEBA',
  [string]$Port = 'C:\Tests\laser.pdf'
)
$ErrorActionPreference = 'Continue'
$t = 'C:\Tests'
New-Item -ItemType Directory -Force \\host.lan\Data\out | Out-Null

foreach ($doc in ($Docs -split ',')) {
  Remove-Item $Port, "$t\mock.out" -ErrorAction SilentlyContinue
  $m = Start-Process "$t\mock-server.exe" -PassThru -WindowStyle Hidden `
    -RedirectStandardOutput "$t\mock.out" -RedirectStandardError "$t\mock.err" `
    -ArgumentList '-format', 'pdf', '-exit', '-printer', "`"$Printer`"", '-pdf', "$t\pdfs\$doc.pdf"

  $peak = 0; $start = Get-Date
  while (-not $m.HasExited -and ((Get-Date) - $start).TotalMinutes -lt 10) {
    $a = Get-Process tera-agent-tray, tera-agent -ErrorAction SilentlyContinue |
      Sort-Object WorkingSet64 -Descending | Select-Object -First 1
    if ($a -and $a.WorkingSet64 -gt $peak) { $peak = $a.WorkingSet64 }
    Start-Sleep -Milliseconds 250
  }
  if (-not $m.HasExited) { $m.Kill(); "JOB $doc timeout 600 pico=$([math]::Round($peak / 1MB))"; continue }

  # La impresora virtual escribe el fichero cuando el spooler termina el trabajo.
  $w = Get-Date
  while ((Get-PrintJob -PrinterName $Printer -ErrorAction SilentlyContinue) -and ((Get-Date) - $w).TotalSeconds -lt 120) { Start-Sleep 1 }
  Start-Sleep 2

  $r = Get-Content "$t\mock.out" -ErrorAction SilentlyContinue | Select-String '^RESULT ' | Select-Object -First 1
  $fields = if ($r) { $r.Line -split ' ', 5 } else { @('RESULT', $doc, 'sin_resultado', '0', '') }
  "JOB $doc $($fields[2]) $($fields[3]) pico=$([math]::Round($peak / 1MB))"
  if ($fields[2] -ne 'job_completed' -and $fields[4]) { "    $($fields[4])" }
  if (Test-Path $Port) { Copy-Item $Port "\\host.lan\Data\out\$Prefix-$doc.pdf" -Force }
}
