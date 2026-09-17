# Manual de instalación — Tera Agent (MVP)

Agente de impresión multiplataforma. Este manual cubre **Linux** y **Windows**.

## 1. Requisitos

| Plataforma | Imprescindible | Para imprimir PDF térmico |
|---|---|---|
| **Linux/macOS** | CUPS (`lp`, `lpstat`) — normalmente ya instalado | `poppler-utils` (`pdftoppm`) |
| **Windows 10 1903+ / 11** | Spooler de impresión (incluido en Windows) | nada: el instalador empaqueta poppler |

> **Windows:** el instalador empaqueta dos herramientas de poppler 26.09:
>
> - `pdftoppm.exe` convierte PDF en imagen para las **térmicas** (ESC/POS) y para
>   las láser en **modo imagen**.
> - `pdftocairo.exe` imprime PDF en **láser / inyección en modo vectorial** (el
>   modo por defecto; ver [§7](#modo-de-impresión-de-las-láser-vectorial-o-imagen)).
>
> No basta con copiar los `.exe`: importan `poppler.dll`, `cairo.dll`, ICU, el
> runtime de Visual C++… `tools/popplerbundle` copia exactamente las DLLs que
> cargan (30 ficheros de los 143 del bundle), leyendo sus imports. El runtime de
> Visual C++ va *app-local* junto a los `.exe`, así que el cliente no necesita
> instalar el Redistributable — sin él, en un equipo limpio, poppler muere con
> `0xc0000135` (`STATUS_DLL_NOT_FOUND`) sin ejecutar nada.
>
> Todo va en `poppler\bin\`; el Agent lo encuentra ahí sin tocar el `PATH` del
> sistema — importante, porque el servicio corre como LocalSystem y no ve el
> `PATH` del usuario. Para forzar otra copia: variables `TERA_PDFTOPPM` y
> `TERA_PDFTOCAIRO` con la **ruta absoluta** al `.exe`.

- `raw`, `text` y ESC/POS **no** requieren poppler.
- La conversión **PDF → ESC/POS** usa `pdftoppm` (rasterizador, reemplazable).
- En Windows, las láser en modo vectorial usan `pdftocairo`.

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

## 7. Windows: instalador (recomendado)

La forma normal de instalar en un equipo cliente es el instalador
`TeraAgent-Setup-<versión>.exe`: **doble clic → siguiente → listo**. Incluye todo
lo necesario, incluido poppler; el cliente no tiene que instalar dependencias.

Durante el asistente se elige el **modo de ejecución**. Son excluyentes a
propósito: el WebSocket contra el ERP debe tener un único dueño en el equipo, o
el ERP recibiría cada impresión duplicada.

| Modo | Cuándo arranca | Bandeja y panel | Para qué |
|---|---|---|---|
| **App de usuario** (por defecto) | al iniciar sesión en Windows | sí | punto de venta atendido |
| **Servicio de Windows** | con el equipo, sin que nadie inicie sesión | no (los servicios no pueden mostrar bandeja) | equipos desatendidos |

Tras la instalación se abre el panel en <http://127.0.0.1:9180> para **registrar
el equipo con el Token** del ERP. El panel queda accesible siempre: desde el
icono de la bandeja (*Abrir panel*) o desde el acceso directo del menú Inicio.

Qué deja en el equipo:

```
C:\Program Files\TeraAgent\tera-agent.exe        servicio, CLI y diagnóstico
C:\Program Files\TeraAgent\tera-agent-tray.exe   mismo programa sin consola (bandeja)
C:\Program Files\TeraAgent\poppler\bin\          pdftoppm.exe + sus DLLs
C:\ProgramData\TeraAgent\config.yaml             configuración y Token
C:\ProgramData\TeraAgent\tera-agent.log          log rotativo
```

Se desinstala como cualquier programa: **Configuración → Aplicaciones → Tera
Agent**. La configuración y los logs se conservan, para que una reinstalación no
obligue a volver a registrar el equipo.

### Compilar el instalador

Desde **Linux** (Go y Docker; descarga poppler y verifica su SHA256):

```bash
scripts/build-release.sh 1.0.0
```

Desde **Windows**, requiere **Inno Setup 6** (una sola vez:
`winget install JRSoftware.InnoSetup`) y el bundle de poppler para Windows.

```powershell
.\scripts\build-release.ps1 -Version 1.0.0
```

Genera en `dist\`: los dos binarios, `TeraAgent-Setup-1.0.0.exe` y
`SHA256SUMS.txt` (los hashes que necesita el manifiesto del ERP). Ver
[RELEASE.md](RELEASE.md).

Por defecto espera poppler en `C:\Program Files\poppler-26.09.0\` (bundle de
`github.com/oschwartz10612/poppler-windows`); si está en otro sitio, pásalo con
`-PopplerBin <bundle>\Library\bin -PopplerLicense <bundle>\share\poppler\COPYING.gpl2`.
Ojo con la licencia: `share\poppler\COPYING` es el aviso de poppler-data, no la
GPL de Poppler.

El script llama a `go run ./tools/popplerbundle`, que además de elegir las DLLs
pone a `pdftocairo.exe` un **manifiesto UTF-8**: sin él, las impresoras con tildes
o ñ en el nombre fallan con `Printer not found` en modo vectorial.

### Modo de impresión de las láser: vectorial o imagen

En **Panel → Impresoras**, cada impresora de tipo *Láser / PDF* tiene un selector:

| Modo | Cómo imprime | Cuándo |
|---|---|---|
| **Vectorial** (por defecto) | `pdftocairo` dibuja el PDF en el driver de la impresora: el texto llega como texto y la impresora lo imprime a su resolución nativa. Carta sale en carta y A4 en A4. | Siempre que funcione |
| **Imagen** (compatibilidad) | Cada página se rasteriza y se envía como bitmap por GDI. Aplica el margen configurado para la impresora. | Solo si el driver de una impresora concreta da problemas con el vectorial |

Medido en Windows 11 limpio con facturas del ERP (fpdf2): 29 páginas en **25 s y
24 MB** en vectorial; en modo imagen la misma factura agotaba la memoria de un
equipo de 4 GB, y las páginas con color (logo, cabecera) llegaban a la impresora
a ~170 ppp. El cambio de modo aplica al siguiente trabajo, sin reiniciar. En
`config.yaml` queda como `page_mode: image` (el vectorial no se escribe).

Casos que caen solos a modo imagen, sin fallar:

- `pdftocairo.exe` no está instalado.
- Trabajos PNG/JPEG (no tienen camino vectorial).
- Impresoras con tildes o ñ en el nombre en un Windows **anterior a 10 1903**,
  que no respeta el manifiesto UTF-8.

Los PDF de fpdf2 con fuentes estándar (Helvetica) **no incrustan la fuente**: en
Windows se imprimen con Arial, que es la misma que muestran Acrobat, Edge o Chrome
al abrir el PDF. Los avisos `No display font for 'Symbol' / 'ArialNarrow'…` del
log son fuentes que el documento no usa.

### Antes de llevarlo a un cliente

Prueba el instalador en una **VM limpia** y pasa la verificación automática:

```bash
powershell -ExecutionPolicy Bypass -File .\scripts\windows-verify.ps1
```

Debe terminar con `Fallos: 0`. Los pasos manuales (reinicio, bandeja, impresión
real) y el porqué de todo esto están en [ACCEPTANCE.md](ACCEPTANCE.md).

### Despliegue en muchos equipos (silencioso)

```powershell
.\scripts\windows-install.ps1 -Mode service     # o -Mode user
.\scripts\windows-install.ps1 -Url https://tu-erp/agent/download/latest/windows/amd64
.\scripts\windows-install.ps1 -Uninstall
```

O directamente sobre el instalador:

```powershell
TeraAgent-Setup-1.0.0.exe /VERYSILENT /SUPPRESSMSGBOXES /MODE=service
```

### Gestión manual del servicio

```powershell
tera-agent.exe service install|uninstall|start|stop --config <ruta>
```

El servicio se crea con **arranque automático retrasado**, dependencia del
**spooler de impresión** y **reintentos automáticos** (5s, 15s, luego cada
minuto), para que un POS vuelva a imprimir solo sin que nadie reinicie el equipo.

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
