#!/usr/bin/env bash
# Build de release de Tera Agent para Windows compilado DESDE LINUX.
# Owner: DevOps Engineer. Equivalente a scripts/build-release.ps1: mismo staging,
# mismo instalador (Inno Setup bajo Wine, installer/wine/Dockerfile).
#
#   scripts/build-release.sh 1.1.0
#   scripts/build-release.sh 1.1.0 --skip-installer     # solo binarios
#
# Requiere Go, Docker, curl, unzip y sha256sum. Produce en dist/:
#   tera-agent.exe                  binario de consola (servicio, CLI, diagnostico)
#   tera-agent-tray.exe             mismo programa sin consola (autoarranque/bandeja)
#   TeraAgent-Setup-<version>.exe   instalador
#   SHA256SUMS.txt                  hashes para el manifiesto /agent/version del ERP
#
# Antes de llevarlo a un cliente, pásalo por la VM limpia: tools/windows-vm.
set -euo pipefail

version="${1:-}"
shift || true
skip_installer=false
for arg in "$@"; do
  case "$arg" in
    --skip-installer) skip_installer=true ;;
    *) echo "opción desconocida: $arg" >&2; exit 2 ;;
  esac
done

# El comparador del auto-update y el manifiesto del ERP asumen X.Y.Z: una versión
# mal formada rompería la actualización de todos los clientes en silencio.
if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
  echo "uso: $0 <version semver X.Y.Z[-pre]> [--skip-installer]" >&2
  exit 2
fi

repo="$(cd "$(dirname "$0")/.." && pwd)"
dist="$repo/dist"
staging="$repo/installer/staging"
cache="${XDG_CACHE_HOME:-$HOME/.cache}/tera-agent"
pkg=github.com/teraerp/tera-agent/internal/infra/di
cd "$repo"

step() { printf '\n\033[36m-> %s\033[0m\n' "$*"; }

# El .syso con el icono y los datos de version de los .exe es un artefacto de
# build: se genera con la version de este release y se borra al salir, para que
# un `go build` a mano no arrastre la version de otro.
syso="$repo/cmd/tera-agent/rsrc_windows_amd64.syso"
cleanup() {
  rm -f "$syso"
  if [[ -n "${tmp:-}" ]]; then rm -rf "$tmp"; fi
}
trap cleanup EXIT

echo "== Tera Agent $version (build desde Linux) =="

# --- 1. Binarios --------------------------------------------------------------
mkdir -p "$dist"
ld="-s -w -X $pkg.AgentVersion=$version"
step "recursos de Windows (icono y datos de version)"
go run ./tools/winres -ico installer/tera-agent.ico -version "$version" -out "$syso"
step "compilando tera-agent.exe (consola)"
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$ld" -o "$dist/tera-agent.exe" ./cmd/tera-agent
step "compilando tera-agent-tray.exe (sin consola)"
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$ld -H=windowsgui" -o "$dist/tera-agent-tray.exe" ./cmd/tera-agent

# Aquí no se puede ejecutar el .exe para preguntarle la versión, como hace
# build-release.ps1. Comprobación equivalente: que el -X haya dejado la cadena
# en el binario. Si no cuajó, el binario diría "0.0.0-dev" y el auto-update
# quedaría roto en producción.
if ! grep -aqF "$version" "$dist/tera-agent.exe"; then
  echo "La versión $version no está en tera-agent.exe: el -ldflags no se aplicó" >&2
  exit 1
fi
echo "   versión inyectada: $version"

write_sums() {
  (cd "$dist" && ls tera-agent.exe tera-agent-tray.exe "TeraAgent-Setup-$version.exe" 2>/dev/null |
    xargs sha256sum > SHA256SUMS.txt)
  cat "$dist/SHA256SUMS.txt"
}

if $skip_installer; then
  echo "-> instalador omitido (--skip-installer)"
  write_sums
  exit 0
fi

# --- 2. Poppler ---------------------------------------------------------------
# Versión y hash en installer/poppler.env (compartido con release.yml). Se lee
# como datos, no con `source`: no ejecuta nada.
poppler_var() { sed -n "s/^$1=\\(.*\\)$/\\1/p" installer/poppler.env | tr -d '[:space:]'; }
POPPLER_VERSION="$(poppler_var POPPLER_VERSION)"
POPPLER_SHA256="$(poppler_var POPPLER_SHA256)"

zip="$cache/poppler-$POPPLER_VERSION.zip"
if [[ ! -f "$zip" ]]; then
  step "descargando poppler $POPPLER_VERSION"
  mkdir -p "$cache"
  curl -fL --retry 3 -o "$zip.part" \
    "https://github.com/oschwartz10612/poppler-windows/releases/download/v$POPPLER_VERSION/Release-$POPPLER_VERSION.zip"
  mv "$zip.part" "$zip"
fi
# Se verifica siempre, también la copia en caché.
echo "$POPPLER_SHA256  $zip" | sha256sum -c --quiet - || {
  echo "SHA256 de $zip no coincide con installer/poppler.env; bórralo y vuelve a intentar" >&2
  exit 1
}

tmp="$(mktemp -d)" # lo borra cleanup()
unzip -q "$zip" -d "$tmp"
poppler_bin="$(dirname "$(find "$tmp" -name pdftoppm.exe | head -n1)")"
# COPYING.gpl2 es la licencia de Poppler; share/poppler/COPYING es el aviso de poppler-data.
license="$(find "$tmp" -name COPYING.gpl2 | head -n1)"
[[ -f "$poppler_bin/pdftoppm.exe" && -f "$license" ]] || {
  echo "el zip de poppler no trae pdftoppm.exe o COPYING.gpl2" >&2
  exit 1
}

# --- 3. Staging ---------------------------------------------------------------
step "preparando staging"
rm -rf "$staging"
mkdir -p "$staging/poppler/bin"
cp "$dist/tera-agent.exe" "$dist/tera-agent-tray.exe" "$staging/"
# PDF de prueba: windows-verify.ps1 y el soporte ejercitan el pipeline sin papel.
cp internal/adapters/ui/assets/testpage.pdf "$staging/"
# Solo las DLLs que cargan pdftoppm y pdftocairo, y manifiesto UTF-8 en pdftocairo.
go run ./tools/popplerbundle -src "$poppler_bin" -dst "$staging/poppler/bin" | sed -n 1p
cp "$license" "$staging/poppler/COPYING"

# Runtime de Visual C++ app-local. Aquí no hay System32 del que tomarlo como en
# build-release.ps1: tiene que venir en el bundle (poppler 26.07+ lo trae). Sin
# él, poppler muere con 0xc0000135 en cualquier equipo limpio.
for dll in msvcp140.dll vcruntime140.dll vcruntime140_1.dll; do
  [[ -f "$staging/poppler/bin/$dll" ]] || {
    echo "falta $dll en el bundle de poppler $POPPLER_VERSION: usa 26.07 o posterior" >&2
    exit 1
  }
done

# --- 4. Instalador ------------------------------------------------------------
# La etiqueta de la imagen depende del Dockerfile: si cambia (otra versión de
# Inno Setup), se reconstruye sola.
image="tera-agent-innosetup:$(sha256sum installer/wine/Dockerfile | cut -c1-12)"
if ! docker image inspect "$image" >/dev/null 2>&1; then
  step "construyendo la imagen de Inno Setup (solo la primera vez)"
  docker build -q -t "$image" installer/wine
fi

step "compilando instalador"
docker run --rm -v "$repo:/work" "$image" /Q "/DAgentVersion=$version" 'Z:\work\installer\tera-agent.iss' 2>&1 |
  grep -v XDG_RUNTIME_DIR || true
setup="$dist/TeraAgent-Setup-$version.exe"
[[ -f "$setup" ]] || { echo "ISCC no generó $setup" >&2; exit 1; }
# Wine corre como root dentro del contenedor: devolver el fichero al usuario.
docker run --rm -v "$dist:/dist" --entrypoint chown "$image" "$(id -u):$(id -g)" "/dist/$(basename "$setup")"

# --- 5. Hashes para el manifiesto del ERP ------------------------------------
printf '\n== Listo ==\n'
printf '%s  %s MB\n' "$(basename "$setup")" "$(( $(stat -c %s "$setup") / 1024 / 1024 ))"
write_sums
