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

## 6b. Servicio de Linux (systemd)

```bash
sudo cp dist/tera-agent-linux /usr/local/bin/tera-agent
sudo mkdir -p /etc/tera-agent && sudo cp config.example.yaml /etc/tera-agent/config.yaml
sudo cp deploy/systemd/tera-agent.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now tera-agent
journalctl -u tera-agent -f          # logs
```

El servicio necesita acceso a CUPS (usuario `root` o del grupo `lp`).

## 7. Servicio de Windows

Con `tera-agent.exe` y los scripts de `scripts/` en una carpeta, en **PowerShell
como Administrador**:

```powershell
.\windows-install.ps1                  # copia a Program Files, crea y arranca el servicio
```

Esto instala el servicio **TeraAgent** (arranque automático), con la config en
`C:\ProgramData\TeraAgent\config.yaml`. Gestión manual:

```powershell
tera-agent.exe service install|uninstall|start|stop --config <ruta>
# o desinstalar todo:
.\windows-uninstall.ps1
```

## 8. Solución de problemas

### Linux: la impresora imprime PostScript / instala driver equivocado (p. ej. Digital POS KL200)

Las térmicas ESC/POS (KL200, XP-80, …) **no** deben usar un driver PostScript/GDI:
CUPS convertiría el trabajo a PostScript y la impresora escupe código. La solución
es una **cola RAW** (sin driver). Tera Agent ya envía con `-o raw`, pero conviene
que la cola sea raw:

```bash
# 1) Encuentra la URI del dispositivo (USB/red)
lpinfo -v
#    ejemplo: usb://Digital-POS/KL200?serial=...

# 2) Borra la cola mal configurada (si existe) y crea una RAW
sudo lpadmin -x KL200 2>/dev/null
sudo lpadmin -p KL200 -E -v 'usb://Digital-POS/KL200?serial=...' -m raw

# 3) Marca como lista y prueba (dry-run, no imprime)
tera-agent print --printer KL200 --file recibo.pdf --out /tmp/ticket.bin
```

Si tu versión de CUPS no acepta `-m raw`, crea la cola **sin PPD** (queda raw) o
usa la interfaz web `http://localhost:631` → *Add Printer* → modelo **"Raw"**.

> El KL200 es 80mm → usa `--paper 80` (576 dots), el valor por defecto.
