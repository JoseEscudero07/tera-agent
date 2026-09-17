# Verificacion de aceptacion de una instalacion de Tera Agent en Windows.
# Owner: QA Engineer.
#
# Se ejecuta DESPUES de instalar, idealmente en una VM limpia (ver
# docs/ACCEPTANCE.md). Comprueba automaticamente todo lo que no requiere ojos
# humanos y falla con codigo != 0 si algo no esta bien.
#
#   .\windows-verify.ps1
#   .\windows-verify.ps1 -Printer "XP-80"     # anade impresion fisica de prueba
#   .\windows-verify.ps1 -SkipConnection      # equipo aun sin registrar
#
# Cada comprobacion existe porque algo fallo de verdad en un cliente. No quitar
# ninguna sin entender cual.
param(
  [string]$InstallDir = "$env:ProgramFiles\TeraAgent",
  [string]$DataDir = "$env:ProgramData\TeraAgent",
  [string]$PanelUrl = "http://127.0.0.1:9180",
  [string]$Printer,
  [switch]$SkipConnection
)

# El panel responde JSON en UTF-8; sin esto la consola muestra "AplicaciÃ³n".
try { [Console]::OutputEncoding = [Text.Encoding]::UTF8 } catch {}

$script:Pass = 0
$script:Fail = 0
$script:Skip = 0

function Check {
  param([string]$Name, [scriptblock]$Test)
  try {
    $r = & $Test
    if ($r -is [string] -and $r) {
      # Una cadena devuelta significa fallo con explicacion.
      Write-Host "  [FALLO] $Name" -ForegroundColor Red
      Write-Host "          $r" -ForegroundColor DarkGray
      $script:Fail++
    } else {
      Write-Host "  [ OK  ] $Name" -ForegroundColor Green
      $script:Pass++
    }
  } catch {
    Write-Host "  [FALLO] $Name" -ForegroundColor Red
    Write-Host "          $($_.Exception.Message)" -ForegroundColor DarkGray
    $script:Fail++
  }
}

# Note es solo informativo y no cuenta para nada.
function Note($msg) { Write-Host "  [ i  ] $msg" -ForegroundColor DarkGray }

# Skip marca una comprobacion que NO se pudo hacer. Se cuenta aparte a proposito:
# un check omitido no es un check superado, y contarlo como OK daria una falsa
# sensacion de "todo verificado".
function Skip($name, $why) {
  Write-Host "  [OMIT ] $name" -ForegroundColor Yellow
  Write-Host "          $why" -ForegroundColor DarkGray
  $script:Skip++
}

function Section($t) { Write-Host "`n== $t ==" -ForegroundColor Cyan }

# ---------------------------------------------------------------- 1. Ficheros
Section "1. Ficheros instalados"

$exe = Join-Path $InstallDir "tera-agent.exe"
$tray = Join-Path $InstallDir "tera-agent-tray.exe"
$popplerBin = Join-Path $InstallDir "poppler\bin"

Check "tera-agent.exe existe" { if (-not (Test-Path $exe)) { "No esta en $exe" } }
Check "tera-agent-tray.exe existe" { if (-not (Test-Path $tray)) { "No esta en $tray" } }

# El bundle de poppler es la fuente de los fallos mas caros: pdftoppm.exe solo NO
# funciona, necesita las DLLs de poppler Y el runtime de Visual C++.
Check "pdftoppm.exe empaquetado" {
  if (-not (Test-Path (Join-Path $popplerBin "pdftoppm.exe"))) { "Falta $popplerBin\pdftoppm.exe" }
}
# Sin pdftocairo.exe el Agent no falla: las laser caen al modo imagen, que es el
# que imprimia borroso (paginas con color reducidas a ~170 ppp) y agotaba la
# memoria con documentos largos. Por eso su ausencia es un fallo y no un aviso.
Check "pdftocairo.exe empaquetado (impresion vectorial en laser)" {
  if (-not (Test-Path (Join-Path $popplerBin "pdftocairo.exe"))) { "Falta $popplerBin\pdftocairo.exe: las laser imprimiran en modo imagen" }
}
Check "runtime de Visual C++ app-local (msvcp140 / vcruntime140 / vcruntime140_1)" {
  $faltan = @('msvcp140.dll', 'vcruntime140.dll', 'vcruntime140_1.dll') |
    Where-Object { -not (Test-Path (Join-Path $popplerBin $_)) }
  if ($faltan) { "Faltan: $($faltan -join ', ') -> pdftoppm morira con 0xc0000135 en equipos sin el Redistributable" }
}
# Ya no se cuenta DLLs: el instalador empaqueta solo las que cargan pdftoppm y
# pdftocairo (tools/popplerbundle), y que esten todas lo demuestran los checks de
# "arranca" de la seccion 2.

# ------------------------------------------------------- 2. pdftoppm ejecutable
Section "2. Rasterizador"

# Esta es la comprobacion directa del 0xc0000135: si le falta cualquier DLL,
# Windows no arranca el proceso y el codigo de salida lo delata.
foreach ($tool in "pdftoppm", "pdftocairo") {
  Check "$tool.exe arranca (todas sus DLLs resuelven)" {
    $p = Join-Path $popplerBin "$tool.exe"
    if (-not (Test-Path $p)) { return "no instalado" }
    $errFile = Join-Path $env:TEMP "tera-verify-$tool.txt"
    $proc = Start-Process $p -ArgumentList "-v" -Wait -PassThru -NoNewWindow -RedirectStandardError $errFile
    if ($proc.ExitCode -eq 0) { return }
    if ($proc.ExitCode -eq -1073741515) { return "0xC0000135 STATUS_DLL_NOT_FOUND: falta una DLL (normalmente el runtime de Visual C++) junto a $tool.exe" }
    "exit code $($proc.ExitCode)"
  }
}

# pdftocairo solo imprime si se compilo con la superficie win32 de cairo. Si un
# bundle futuro viniera sin ella, cada PDF en una laser fallaria sin caer al modo
# imagen (el driver vectorial solo comprueba que el .exe exista).
Check "pdftocairo.exe sabe imprimir en Windows (-print)" {
  $p = Join-Path $popplerBin "pdftocairo.exe"
  if (-not (Test-Path $p)) { return "no instalado" }
  $help = (& cmd /c "`"$p`" -h 2>&1") -join "`n"
  if ($help -notmatch '(?m)^\s*-print\b') { "pdftocairo no ofrece -print: este bundle de poppler no imprime en Windows" }
}

# Sin el manifiesto UTF-8, las impresoras con tildes o ene en el nombre fallan
# con "Printer not found" (pdftocairo usa las funciones ANSI de Windows).
Check "pdftocairo.exe admite nombres de impresora con tildes (manifiesto UTF-8)" {
  $p = Join-Path $popplerBin "pdftocairo.exe"
  if (-not (Test-Path $p)) { return "no instalado" }
  $raw = [Text.Encoding]::ASCII.GetString([IO.File]::ReadAllBytes($p))
  if ($raw -notmatch '<activeCodePage[^>]*>UTF-8</activeCodePage>') { "sin activeCodePage UTF-8: las impresoras con tildes o ene fallaran en modo vectorial" }
}

# --------------------------------------------------------------- 3. Config
Section "3. Configuracion"

$cfg = Join-Path $DataDir "config.yaml"
Check "config.yaml existe" { if (-not (Test-Path $cfg)) { "No esta en $cfg" } }

# Las rutas de Windows entre comillas DOBLES son YAML invalido (\P, \T no son
# escapes): el agente moria al arrancar sin mostrar nada.
Check "config.yaml no usa comillas dobles en rutas de Windows" {
  if (-not (Test-Path $cfg)) { return "config ausente" }
  $malas = Get-Content $cfg | Select-String -Pattern '^\s*\w+:\s*"[A-Za-z]:\\'
  if ($malas) { "Rutas entre comillas dobles (YAML invalido): $($malas.Line.Trim() -join ' | ')" }
}

# fatalStartup escribe aqui cuando el arranque falla. Su existencia es un fallo.
Check "sin errores de arranque registrados" {
  $f = Join-Path $DataDir "startup-error.log"
  if ((Test-Path $f) -and (Get-Item $f).Length -gt 0) {
    "Hay errores en $f : " + ((Get-Content $f -Tail 2) -join ' | ')
  }
}

# ------------------------------------------------------------- 4. Autoarranque
Section "4. Arranque automatico"

$svc = Get-Service TeraAgent -ErrorAction SilentlyContinue
$runKey = Get-ItemProperty "HKLM:\Software\Microsoft\Windows\CurrentVersion\Run" -Name TeraAgent -ErrorAction SilentlyContinue

Check "hay exactamente un modo de arranque configurado" {
  $modos = @()
  if ($svc) { $modos += "servicio" }
  if ($runKey) { $modos += "app de usuario" }
  if ($modos.Count -eq 0) { return "Ninguno: el agente no arrancara solo" }
  if ($modos.Count -gt 1) { return "DOS a la vez ($($modos -join ' + ')): habria dos agentes con el mismo token y el ERP recibiria cada impresion duplicada" }
  Note "modo detectado: $($modos[0])"
}

if ($svc) {
  Check "servicio con arranque automatico" {
    if ($svc.StartType -notin @('Automatic', 'AutomaticDelayedStart')) { "StartType = $($svc.StartType)" }
  }
  Check "servicio en ejecucion" { if ($svc.Status -ne 'Running') { "Status = $($svc.Status)" } }
  # Se lee del registro y no de "sc.exe qfailure": su salida va traducida
  # ("REINICIAR" en Windows en espanol) y buscar "RESTART" daba un falso fallo.
  # FailureActions es SERVICE_FAILURE_ACTIONS: 20 bytes de cabecera (cActions en
  # el offset 12) y luego pares {Type, Delay}; Type 1 = SC_ACTION_RESTART.
  Check "servicio con reintentos automaticos configurados" {
    $fa = (Get-ItemProperty "HKLM:\SYSTEM\CurrentControlSet\Services\TeraAgent" -Name FailureActions -ErrorAction SilentlyContinue).FailureActions
    $restart = $false
    if ($fa -and $fa.Length -ge 20) {
      $n = [BitConverter]::ToUInt32($fa, 12)
      for ($i = 0; $i -lt $n -and (20 + $i * 8 + 4) -le $fa.Length; $i++) {
        if ([BitConverter]::ToUInt32($fa, 20 + $i * 8) -eq 1) { $restart = $true }
      }
    }
    if (-not $restart) { "sin acciones de recuperacion: un crash dejaria el POS sin imprimir hasta un reinicio manual" }
  }
  Check "servicio depende del spooler de impresion" {
    $q = (sc.exe qc TeraAgent) -join " "
    if ($q -notmatch "Spooler") { "sin dependencia de Spooler: puede arrancar antes que la cola de impresion" }
  }
}

if ($runKey) {
  Check "la entrada de autoarranque apunta al binario sin consola" {
    if ($runKey.TeraAgent -notmatch 'tera-agent-tray\.exe') { "apunta a: $($runKey.TeraAgent) (mostraria una consola al iniciar sesion)" }
  }
  Check "proceso de bandeja en ejecucion" {
    if (-not (Get-Process tera-agent-tray -ErrorAction SilentlyContinue)) { "tera-agent-tray.exe no esta corriendo" }
  }
}

# ------------------------------------------------------------------ 5. Panel
Section "5. Panel y estado"

$status = $null
Check "el panel responde en $PanelUrl" {
  try { $script:status = Invoke-RestMethod "$PanelUrl/api/status" -TimeoutSec 10 }
  catch { return "sin respuesta: $($_.Exception.Message)" }
}

if ($script:status) {
  Check "el panel expone el modo de arranque y no filtra el token" {
    $c = Invoke-RestMethod "$PanelUrl/api/config" -TimeoutSec 10
    if ($null -eq $c.tokenSet) { return "falta 'tokenSet': el binario instalado es anterior al arreglo del panel" }
    if ($c.PSObject.Properties.Name -contains 'token') { return "GET /api/config devuelve el token en claro" }
    Note "version del agente: $($script:status.version) / modo: $($c.runMode)"
  }

  if (-not $SkipConnection) {
    Check "equipo registrado" { if (-not $script:status.registered) { "sin token o sin URL: registra el equipo en $PanelUrl" } }
    Check "conectado al ERP (estado CONNECTED)" {
      if ($script:status.state -ne 'CONNECTED') {
        "estado = $($script:status.state). Si se queda en RECONNECTING, revisa el log: un 403 en el handshake suele ser el validador de Origin de Django"
      }
    }
  } else {
    Skip "registro y conexion al ERP" "omitido con -SkipConnection"
  }
}

# -------------------------------------------------------------------- 6. Log
Section "6. Registro"

# El log en fichero no escribia nada en el binario sin consola: io.MultiWriter
# abortaba porque os.Stderr es un handle invalido sin consola.
Check "el agente escribe log en fichero" {
  $log = Join-Path $DataDir "tera-agent.log"
  if (-not (Test-Path $log)) { return "no existe $log (revisa 'log.file' en config.yaml)" }
  if ((Get-Item $log).Length -eq 0) { return "$log esta a 0 bytes: el agente no esta registrando nada" }
}

# ------------------------------------------------------------- 7. Impresion
Section "7. Impresion"

Check "el agente descubre impresoras" {
  $out = & $exe printers --log error 2>&1 | Out-String
  if ($out -notmatch "NAME\s+SYSTEM") { "salida inesperada: $($out.Trim())" }
  $n = ([regex]::Matches($out, "winspool")).Count
  if ($n -eq 0) { return "ninguna impresora detectada" }
  Note "$n impresora(s) detectada(s)"
}

# Pipeline completo PDF -> raster -> ESC/POS sin gastar papel. Es el unico check
# que ejercita poppler de verdad end-to-end (el paso 2 solo comprueba que arranca).
$pdf = Join-Path $InstallDir "testpage.pdf"
if (Test-Path $pdf) {
  Check "pipeline PDF -> ESC/POS (dry-run, no imprime)" {
    $out = Join-Path $env:TEMP "tera-verify-ticket.bin"
    Remove-Item $out -ErrorAction SilentlyContinue
    & $exe print --printer VERIFY --file $pdf --out $out --log error 2>&1 | Out-Null
    if (-not (Test-Path $out)) { return "no se genero salida" }
    $b = [IO.File]::ReadAllBytes($out)
    if ($b.Length -lt 100) { return "salida sospechosamente corta ($($b.Length) bytes)" }
    if ($b[0] -ne 0x1B -or $b[1] -ne 0x40) { return "no empieza por ESC @ (1B 40)" }
  }
} else {
  Skip "pipeline PDF -> ESC/POS (dry-run)" "falta $pdf (instalacion anterior a que el instalador lo desplegara)"
}

if ($Printer) {
  Check "impresion fisica de prueba en '$Printer' (IMPRIME)" {
    $out = & $exe text --printer $Printer --text "Tera Agent - verificacion OK" --log error 2>&1 | Out-String
    if ($LASTEXITCODE -ne 0) { "fallo: $($out.Trim())" }
  }
} else {
  Skip "impresion fisica" "pasa -Printer 'NOMBRE' para probarla"
}

# ------------------------------------------------------------------ Resumen
Write-Host "`n=============================================" -ForegroundColor Cyan
Write-Host " Correctas: $script:Pass   Fallos: $script:Fail   Omitidas: $script:Skip" -ForegroundColor $(if ($script:Fail -gt 0) { "Red" } else { "Green" })
Write-Host "=============================================" -ForegroundColor Cyan

if ($script:Fail -gt 0) {
  Write-Host "`nLa instalacion NO esta lista. Revisa los fallos de arriba." -ForegroundColor Red
  Write-Host "Pasos manuales pendientes en docs/ACCEPTANCE.md (reinicio, bandeja, impresion real)."
  exit 1
}

Write-Host "`nComprobaciones automaticas superadas." -ForegroundColor Green
Write-Host "Faltan los pasos MANUALES de docs/ACCEPTANCE.md:"
Write-Host "  - Reiniciar el equipo y confirmar que el agente arranca solo."
Write-Host "  - Ver el icono en la bandeja y abrir el panel desde ahi."
Write-Host "  - Imprimir un documento real desde el ERP."
exit 0
