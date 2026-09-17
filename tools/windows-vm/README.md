# VM Windows limpia para probar el instalador (desde Linux)

Owner: QA Engineer. Complementa [docs/ACCEPTANCE.md](../../docs/ACCEPTANCE.md).

Los fallos que llegaron a clientes (DLLs de poppler, runtime de Visual C++,
`config.yaml` inválido, A4/carta que agotaba la memoria, impresoras con tildes)
solo se ven en un Windows **recién instalado** y con documentos **reales**. Esta
carpeta monta ese Windows en Docker, lo deja guardado en un estado limpio y
automatiza la prueba de un instalador, sin necesitar un PC con Windows.

## Qué necesitas

- Linux con **virtualización activada en la BIOS** (SVM en AMD, VT-x en Intel):
  tiene que existir `/dev/kvm`.
- Docker, Go, Python 3 y `poppler-utils` (`pdfinfo`, `pdfimages`, `pdftocairo`).
- ~70 GB libres y 4 GB de RAM para la VM mientras está encendida.

Todo lo pesado vive fuera del repo, en `TERA_VM_HOME` (por defecto
`~/.local/share/tera-agent-vm`): el disco de Windows, la carpeta compartida y el
entorno Python.

## Primera vez

```bash
tools/windows-vm/vm.sh up            # descarga Windows 11 (es-ES) e instala solo, 20-40 min
tools/windows-vm/vm.sh wait 3600     # espera a que acepte comandos
tools/windows-vm/vm.sh save          # guarda este Windows como "limpio"
```

Mientras se instala puedes mirar el escritorio en <http://127.0.0.1:8006>.

- **Qué queda en la VM:** Windows 11 Pro 25H2 en español con el usuario `cajero` y
  WinRM activo solo en `127.0.0.1` (lo configura `oem/setup-remote.ps1`).
- **Qué NO se instala, a propósito:** nada más. Ni poppler ni el runtime de
  Visual C++, justo lo que el instalador tiene que traer.

## Probar un instalador

```bash
scripts/build-release.sh 1.1.0                                   # instalador desde Linux
tools/windows-vm/make-erp-pdfs.sh                                # opcional: facturas reales del ERP
tools/windows-vm/test.sh dist/TeraAgent-Setup-1.1.0.exe          # modo usuario
tools/windows-vm/test.sh dist/TeraAgent-Setup-1.1.0.exe --mode service
```

`test.sh` vuelve al Windows limpio y hace, en este orden:

1. **Instala** en silencio y registra el Agent contra `examples/mock-server`.
2. **Reinicia** en modo usuario, para comprobar que la bandeja arranca sola.
3. **Pasa `scripts/windows-verify.ps1`.**
4. **Imprime en impresoras virtuales que escriben a fichero:**
   - una térmica, por `pdftoppm` y RAW;
   - una láser, con los PDF en modo vectorial;
   - una impresora llamada `LÁSER Facturación Ñ`.
5. **Comprueba desde Linux** lo que recibió cada impresora: mismas páginas, mismo
   papel y ninguna página rasterizada.

Termina con código 0 solo si todo va bien. Lo impreso queda en
`$TERA_VM_HOME/shared/out`.

`make-erp-pdfs.sh` usa el generador de facturas del ERP (fpdf2) dentro de su
contenedor (`tera_backend`) y no toca la base de datos.

## Uso manual

```bash
tools/windows-vm/vm.sh run 'Get-Service TeraAgent'     # PowerShell dentro de Windows
tools/windows-vm/vm.sh reset                            # volver al estado limpio
tools/windows-vm/vm.sh stop                             # apagarla y liberar la RAM
```

- **Carpeta compartida:** `$TERA_VM_HOME/shared` se ve en Windows como
  `\\host.lan\Data`.
- **Escritorio remoto:** RDP en `127.0.0.1:3389`, usuario `cajero` y contraseña
  `tera-test` (o la de `TERA_VM_PASSWORD`).

## Impresora real en la red

La VM sale a la red local por NAT, así que puede imprimir en una impresora real.
Por ejemplo, para una HP Laser MFP 135w con el driver de HP:

1. Extrae el driver del paquete de HP sin ejecutarlo:
   `tar -xf HP_Laser_..._Full_Software_and_Drivers.exe Printer`.
2. Instálalo con `pnputil /add-driver Printer\SPL\shm4m.inf /install` y
   `Add-PrinterDriver`.
3. Añade la impresora con un puerto TCP/IP (`Add-PrinterPort -PrinterHostAddress <ip>`).

Comprueba antes la firma con `Get-AuthenticodeSignature`.

## Límites

- **Impresoras:** las virtuales no tienen márgenes físicos ni drivers host-based;
  la calidad en papel se valida en una impresora real.
- **Modo servicio:** hoy `windows-verify.ps1` marca "el panel no responde"
  porque el servicio no sirve el panel.
- **Windows 10 1809 y anteriores:** no se prueban aquí. Allí las impresoras con
  tildes caen a modo imagen.
