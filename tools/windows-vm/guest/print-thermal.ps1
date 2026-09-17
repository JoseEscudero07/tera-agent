# Imprime el PDF de prueba en la térmica virtual con la CLI del Agent: ejercita
# pdftoppm y el driver RAW del spooler (se ejecuta DENTRO de Windows).
$t = 'C:\Tests'
Remove-Item "$t\termica.prn" -ErrorAction SilentlyContinue
& "$env:ProgramFiles\TeraAgent\tera-agent.exe" print --printer TERMICA-PRUEBA `
  --file "$env:ProgramFiles\TeraAgent\testpage.pdf" --log error 2>&1 | Out-Null
$code = $LASTEXITCODE
Start-Sleep 5
$bytes = (Get-Item "$t\termica.prn" -ErrorAction SilentlyContinue).Length
"THERMAL exit=$code bytes=$([int]$bytes)"
