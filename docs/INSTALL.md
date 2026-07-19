# Manual de instalación — Tera Agent (MVP)

Agente de impresión multiplataforma. Este manual cubre **Linux** y **Windows**.

## 1. Requisitos

| Plataforma | Imprescindible | Para imprimir PDF térmico |
|---|---|---|
| **Linux/macOS** | CUPS (`lp`, `lpstat`) — normalmente ya instalado | `poppler-utils` (`pdftoppm`) |
| **Windows** | Spooler de impresión (incluido en Windows) | `poppler` para Windows (`pdftoppm.exe` en el PATH) |

- `raw`, `text` y ESC/POS **no** requieren poppler.
- Solo la conversión **PDF → ESC/POS** usa `pdftoppm` (rasterizador, reemplazable).

### Instalar poppler

```bash
# Debian/Ubuntu
sudo apt install poppler-utils
# Fedora
sudo dnf install poppler-utils
# macOS (Homebrew)
brew install poppler
```

Windows: descargar poppler (p. ej. build de `oschwartz10612/poppler-windows`),
descomprimir y añadir su carpeta `bin` al `PATH`.

## 2. Opción A — Ejecutables precompilados

Copia el binario a la máquina:

- Linux: `tera-agent-linux`  → `chmod +x tera-agent-linux`
- Windows: `tera-agent.exe`

No requiere instalación adicional; es un único binario estático (salvo poppler
para PDF).

## 3. Opción B — Compilar desde el código

Requiere **Go 1.23+** (<https://go.dev/dl/>).

```bash
git clone <repo> && cd tera-agent
make build            # binario para tu plataforma
# o multiplataforma:
make cross            # genera dist/ con linux/darwin/windows
```

Compilación manual:

```bash
go build -o tera-agent ./cmd/tera-agent
GOOS=windows GOARCH=amd64 go build -o tera-agent.exe ./cmd/tera-agent
```

## 4. Configuración

Opcional para los comandos de impresión; necesaria para `run` conectado (futuro).

```bash
cp config.example.yaml config.yaml   # edita a tu gusto
```

`config.yaml` (ver [config.example.yaml](../config.example.yaml)):

```yaml
server:
  url: ""            # vacío = modo local (sin backend)
  token: ""
printer:
  default: "XP-80"
log:
  level: "info"
```

## 5. Verificar la instalación

```bash
./tera-agent-linux printers          # lista impresoras (Linux/macOS: CUPS)
./tera-agent.exe    printers          # Windows: spooler
```

Ver [USAGE en el README](../README.md#motor-de-impresión) para ejemplos de
impresión.

## 6. Ejecutar como agente residente

```bash
./tera-agent-linux run                # modo local; imprime vía CLI; sin backend
```

En Windows puede registrarse como tarea/servicio (pendiente el instalador —
ver [MVP.md](MVP.md), problemas conocidos).
