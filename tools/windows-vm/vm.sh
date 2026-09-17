#!/usr/bin/env bash
# Gestión de la VM Windows 11 limpia para probar el instalador de Tera Agent.
# Owner: QA Engineer. Ver README.md.
#
#   vm.sh up        crea/arranca la VM (la primera vez descarga e instala Windows)
#   vm.sh wait      espera a que Windows acepte comandos por WinRM
#   vm.sh save      apaga la VM y guarda el estado actual como "limpio"
#   vm.sh reset     vuelve al estado "limpio" y arranca
#   vm.sh stop      apaga la VM ordenadamente
#   vm.sh run CMD   ejecuta PowerShell en la VM (CMD, o "-" para leer de stdin)
#
# Discos, carpeta compartida y entorno Python viven en TERA_VM_HOME, fuera del
# repo (por defecto ~/.local/share/tera-agent-vm). La carpeta compartida se ve
# en Windows como \\host.lan\Data.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
export TERA_VM_HOME="${TERA_VM_HOME:-${XDG_DATA_HOME:-$HOME/.local/share}/tera-agent-vm}"
snap="limpio"

compose() { docker compose -f "$here/compose.yaml" "$@"; }

qimg() {
  # qemu-img de la propia imagen de la VM: misma versión que el QEMU que usa el disco.
  docker run --rm -v "$TERA_VM_HOME/storage:/storage" --entrypoint qemu-img dockurr/windows "$@"
}

python_env() {
  local venv="$TERA_VM_HOME/venv"
  if [[ ! -x "$venv/bin/python" ]]; then
    python3 -m venv "$venv"
    "$venv/bin/pip" install -q -r "$here/requirements.txt"
  fi
  echo "$venv/bin/python"
}

run() { "$(python_env)" "$here/winrun.py" "$@"; }

wait_vm() {
  local start=$SECONDS limit=${1:-600}
  until timeout 20 "$(python_env)" "$here/winrun.py" 'hostname' >/dev/null 2>&1; do
    if (( SECONDS - start > limit )); then
      echo "Windows no responde tras $limit s (¿sigue instalándose? mira http://127.0.0.1:8006)" >&2
      exit 1
    fi
    sleep 10
  done
  echo "Windows listo en $(( SECONDS - start )) s"
}

preflight() {
  if [[ ! -e /dev/kvm ]]; then
    echo "No existe /dev/kvm: activa la virtualización (SVM en AMD, VT-x en Intel) en la BIOS." >&2
    exit 1
  fi
  mkdir -p "$TERA_VM_HOME/storage" "$TERA_VM_HOME/shared"
}

case "${1:-}" in
  up)
    preflight
    compose up -d
    echo "Escritorio: http://127.0.0.1:8006 — la primera instalación de Windows tarda 20-40 min."
    ;;
  wait)
    # La primera instalación puede tardar mucho más que un arranque normal.
    wait_vm "${2:-600}"
    ;;
  save)
    compose stop
    qimg snapshot -d "$snap" /storage/data.qcow2 2>/dev/null || true
    qimg snapshot -c "$snap" /storage/data.qcow2
    compose start
    wait_vm
    ;;
  reset)
    compose stop
    qimg snapshot -a "$snap" /storage/data.qcow2
    compose start
    wait_vm
    ;;
  stop)
    compose stop
    ;;
  run)
    shift
    run "$@"
    ;;
  *)
    sed -n '2,16p' "$0"
    exit 2
    ;;
esac
