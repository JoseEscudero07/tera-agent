#!/usr/bin/env bash
# Prueba de aceptación automática de un instalador en la VM Windows 11 limpia.
# Owner: QA Engineer. Ver README.md y docs/ACCEPTANCE.md.
#
#   tools/windows-vm/test.sh dist/TeraAgent-Setup-1.1.0.exe
#   tools/windows-vm/test.sh dist/TeraAgent-Setup-1.1.0.exe --mode service
#
# 1. Vuelve la VM al estado "limpio" (sin poppler, sin runtime de Visual C++).
# 2. Instala en modo usuario o servicio, crea impresoras virtuales y registra el
#    Agent contra examples/mock-server.
# 3. Reinicia (modo usuario: comprueba el autoarranque) y pasa windows-verify.ps1.
# 4. Imprime una térmica (pdftoppm) y PDFs en una láser y en una impresora con
#    tildes en el nombre (pdftocairo), y comprueba en Linux lo que recibieron:
#    mismas páginas, mismo tamaño de papel y sin imágenes de página completa.
#    En modo servicio la láser se omite: Microsoft Print To PDF no acepta trabajos
#    de SYSTEM.
#
# PDFs para la láser: una página carta generada a partir del testpage.pdf del
# Agent y, si existen, los de $TERA_VM_HOME/pdfs (facturas reales del ERP, ver
# make-erp-pdfs.sh). Requiere poppler-utils en Linux. Sale con código distinto de
# 0 si algo falla.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
repo="$(cd "$here/../.." && pwd)"
export TERA_VM_HOME="${TERA_VM_HOME:-${XDG_DATA_HOME:-$HOME/.local/share}/tera-agent-vm}"
vm="$here/vm.sh"

setup="${1:?uso: test.sh <TeraAgent-Setup.exe> [--mode user|service]}"
shift
mode=user
while (($#)); do
  case "$1" in
    --mode) mode="${2:?}"; shift 2 ;;
    *) echo "opción desconocida: $1" >&2; exit 2 ;;
  esac
done
[[ -f "$setup" ]] || { echo "no existe $setup" >&2; exit 2; }
[[ "$mode" == user || "$mode" == service ]] || { echo "--mode user|service" >&2; exit 2; }

for tool in pdfinfo pdfimages pdftocairo; do
  command -v "$tool" >/dev/null || { echo "falta $tool: instala poppler-utils" >&2; exit 2; }
done

fails=0
fail() { printf '\033[31m[FALLO]\033[0m %s\n' "$*"; fails=$((fails + 1)); }
ok() { printf '\033[32m[ OK  ]\033[0m %s\n' "$*"; }
skips=0
skip() { printf '\033[33m[OMIT ]\033[0m %s\n' "$*"; skips=$((skips + 1)); }
step() { printf '\n\033[36m== %s\033[0m\n' "$*"; }

# --- Carpeta compartida ---------------------------------------------------------
shared="$TERA_VM_HOME/shared"
step "Preparando \\\\host.lan\\Data"
rm -rf "$shared/guest" "$shared/pdfs" "$shared/out"
mkdir -p "$shared/guest" "$shared/pdfs" "$shared/out"
cp "$setup" "$shared/"
cp "$repo/scripts/windows-verify.ps1" "$shared/"
cp "$here"/guest/*.ps1 "$shared/guest/"
(cd "$repo" && GOOS=windows GOARCH=amd64 go build -o "$shared/mock-server.exe" ./examples/mock-server)
if compgen -G "$TERA_VM_HOME/pdfs/*.pdf" >/dev/null; then
  cp "$TERA_VM_HOME"/pdfs/*.pdf "$shared/pdfs/"
fi
# El testpage.pdf del Agent es de tamaño ticket (300x220 pt), que no es un papel de
# láser: se usa en la térmica y, ampliado a carta, en la láser.
pdftocairo -pdf -paper letter -expand "$repo/internal/adapters/ui/assets/testpage.pdf" "$shared/pdfs/carta-prueba.pdf"
docs="$(cd "$shared/pdfs" && ls *.pdf | sed 's/\.pdf$//' | paste -sd, -)"
echo "PDFs: $docs"

# --- Instalación en limpio ---------------------------------------------------
step "Volviendo al Windows limpio"
"$vm" reset

step "Instalando $(basename "$setup") (modo $mode)"
out="$("$vm" run "& powershell -NoProfile -ExecutionPolicy Bypass -File \\\\host.lan\\Data\\guest\\install-agent.ps1 -Setup '$(basename "$setup")' -Mode $mode" 2>&1)" || true
echo "$out" | grep -E '^INSTALLED|^REGISTERED|rror' || true
echo "$out" | grep -q '^INSTALLED tera-agent' && ok "instalado" || fail "instalación: $out"
echo "$out" | grep -q '^REGISTERED ' && ok "registrado con tera-agent register --scope $mode" || fail "registro: $out"

if [[ "$mode" == user ]]; then
  # Reinicio real: es lo que demuestra que la bandeja arranca sola al iniciar sesión.
  "$vm" run 'Restart-Computer -Force' >/dev/null 2>&1 || true
  sleep 45
  "$vm" wait
  sleep 40
else
  "$vm" run 'Restart-Service TeraAgent; Start-Sleep 5' >/dev/null
fi

# --- windows-verify.ps1 ---------------------------------------------------------
step "windows-verify.ps1"
# La carpeta de datos depende del modo: en servicio, ProgramData (el valor por
# defecto de windows-verify.ps1); en modo usuario, el perfil del usuario que
# inicia sesión, que es el mismo con el que entra WinRM.
data_dir='$env:ProgramData\TeraAgent'
[[ "$mode" == user ]] && data_dir='$env:LOCALAPPDATA\TeraAgent'
verify="$("$vm" run "& powershell -NoProfile -ExecutionPolicy Bypass -File C:\\Tests\\windows-verify.ps1 -SkipConnection -DataDir \"$data_dir\" 2>&1 | Out-String" 2>&1 || true)"
echo "$verify" | grep -E 'FALLO|OMIT|Correctas' | sed 's/^/  /' || true
if echo "$verify" | grep -q 'Fallos: 0'; then ok "verificación sin fallos"; else fail "windows-verify.ps1 con fallos (arriba)"; fi

# --- Impresión ----------------------------------------------------------------
step "Térmica (pdftoppm + RAW)"
thermal="$("$vm" run '& powershell -NoProfile -ExecutionPolicy Bypass -File \\host.lan\Data\guest\print-thermal.ps1' 2>&1 || true)"
echo "  $thermal"
[[ "$thermal" =~ exit=0\ bytes=([0-9]+) ]] && (( BASH_REMATCH[1] > 0 )) && ok "ticket ESC/POS en la cola" || fail "térmica"

run_jobs() { # printer port prefix docs
  "$vm" run "& powershell -NoProfile -ExecutionPolicy Bypass -File \\\\host.lan\\Data\\guest\\print-jobs.ps1 -Printer '$1' -Port '$2' -Prefix '$3' -Docs '$4'" 2>&1 || true
}

# En modo servicio la láser no se puede comprobar con esta VM: "Microsoft Print To
# PDF" deja en error los trabajos de SYSTEM, la cuenta del servicio (pasa igual con
# un Out-Printer sin el agente). Se omite y no cuenta como correcta.
if [[ "$mode" == service ]]; then
  step "Láser e impresora con tildes"
  skip "Microsoft Print To PDF no acepta trabajos de SYSTEM: valida la impresión del servicio en una impresora real"
else
  step "Láser (vectorial por defecto)"
  jobs="$(run_jobs LASER-PRUEBA 'C:\Tests\laser.pdf' laser "$docs")"
  echo "$jobs" | sed 's/^/  /'
  step "Impresora con tildes: LÁSER Facturación Ñ"
  first_doc="${docs%%,*}"
  jobs_accents="$(run_jobs 'LÁSER Facturación Ñ' 'C:\Tests\acentos.pdf' acentos "$first_doc")"
  echo "$jobs_accents" | sed 's/^/  /'

  # --- Qué recibieron las impresoras (desde Linux) -------------------------------
  step "Salida de las impresoras"
  {
    # Nombre del papel ("letter", "A4") o, si pdfinfo no lo reconoce, medidas en
    # puntos redondeadas: el driver de Windows redondea (595.28 -> 595.32).
    paper() {
      pdfinfo "$1" | awk -F': +' '/^Page size/{
        if (match($2, /\(([^)]*)\)/)) { print substr($2, RSTART + 1, RLENGTH - 2) }
        else { split($2, d, " "); printf "%.0f x %.0f pts\n", d[1], d[3] } }'
    }
    check_output() { # prefix doc
      local orig="$shared/pdfs/$2.pdf" got="$shared/out/$1-$2.pdf"
      if [[ ! -f "$got" ]]; then fail "$1/$2: la impresora no recibió nada"; return; fi
      local po pg so sg big
      po="$(pdfinfo "$orig" | awk '/^Pages/{print $2}')"
      pg="$(pdfinfo "$got" | awk '/^Pages/{print $2}')"
      so="$(paper "$orig")"
      sg="$(paper "$got")"
      # Una imagen de más de 1000 px de ancho es una página rasterizada entera: el
      # modo vectorial no debe producir ninguna.
      big="$(pdfimages -list "$got" | awk 'NR>2 && $4>1000' | wc -l)"
      if [[ "$po" == "$pg" && "$so" == "$sg" && "$big" == 0 ]]; then
        ok "$1/$2: $pg pág, $sg, vectorial"
      else
        fail "$1/$2: páginas $po→$pg, papel $so→$sg, páginas rasterizadas: $big"
      fi
    }
    for d in ${docs//,/ }; do
      grep -q "^JOB $d job_completed" <<<"$jobs" || fail "láser/$d: el ERP simulado no recibió job_completed"
      check_output laser "$d"
    done
    grep -q "^JOB $first_doc job_completed" <<<"$jobs_accents" || fail "acentos/$first_doc: sin job_completed"
    check_output acentos "$first_doc"
  }
fi

# --- Actualización encima ------------------------------------------------------
# El caso real de cada release: el cliente ya tiene el agente y se instala la
# versión nueva a base de "siguiente". El instalador tiene que conservar el modo
# (si cambiara, quedarían el servicio y la app de usuario con el mismo Token, y
# el ERP recibiría cada impresión duplicada) y el registro del equipo.
step "Actualizar instalando encima (sin /MODE)"
up="$("$vm" run "& powershell -NoProfile -ExecutionPolicy Bypass -File \\\\host.lan\\Data\\guest\\upgrade-agent.ps1 -Setup '$(basename "$setup")' -Mode $mode" 2>&1 || true)"
echo "  $(echo "$up" | grep -E '^UPGRADED|rror' | paste -sd' ' -)"
want_svc=False; want_run=True
[[ "$mode" == service ]] && { want_svc=True; want_run=False; }
if grep -q "^UPGRADED servicio=$want_svc autoarranque=$want_run token=True" <<<"$up"; then
  ok "sigue en modo $mode y conserva el registro"
else
  fail "la actualización cambió el modo o perdió el registro: $up"
fi

step "Resultado"
if ((fails)); then
  printf '\033[31m%d fallo(s)\033[0m. Salidas en %s/out\n' "$fails" "$shared"
  exit 1
fi
note=""
((skips)) && note=", $skips omitida(s)"
printf '\033[32mTodo correcto\033[0m (modo %s%s). Salidas en %s/out\n' "$mode" "$note" "$shared"
