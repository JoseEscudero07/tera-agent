# Aceptación: probar el instalador en una VM limpia

Owner: QA Engineer.

Esta guía valida el **producto instalado** antes de llevarlo a un cliente. Para
validar el motor de impresión contra una térmica concreta (anchos, corte,
datatype RAW), usa [WINDOWS_TEST.md](WINDOWS_TEST.md) — es otra cosa.

## Por qué en una VM limpia y no en tu equipo

Tres fallos llegaron a clientes porque el equipo de desarrollo tenía cosas que una
máquina limpia no tiene. Cada uno tapó al siguiente:

| Lo que tenía el equipo de desarrollo | Bug que ocultó |
|---|---|
| poppler instalado y en el `PATH` | El instalador no empaquetaba las DLLs de poppler |
| Un `config.yaml` previo válido | La plantilla del instalador generaba YAML inválido → el agente no arrancaba |
| VC++ Redistributable instalado | Faltaba el runtime de Visual C++ → `pdftoppm` moría con `0xc0000135` |

Ninguno era reproducible en el equipo de desarrollo. **Todos** se habrían visto en
diez minutos en una VM limpia.

## Preparar la VM

Cualquiera de estas sirve:

- **Windows Sandbox** — lo más rápido. Viene en Windows Pro/Enterprise: actívalo en
  *Características de Windows → Espacio aislado de Windows*. Se destruye al cerrar,
  así que cada prueba parte de cero de verdad.
- Una VM de Hyper-V / VirtualBox con Windows 10 y 11 **recién instalados**.

Requisito que no se debe cumplir: **no instales poppler, ni el VC++
Redistributable, ni Go**. Es justo lo que se está probando.

> Windows Sandbox no tiene impresoras, así que ahí no se puede probar la impresión
> física. Sirve para todo lo demás (instalación, arranque, config, rasterizado).
> Para la impresión real usa una VM con una impresora compartida o un equipo de
> pruebas.

## 1. Comprobaciones automáticas

Copia a la VM el instalador y `scripts\windows-verify.ps1`. Instala:

```bash
TeraAgent-Setup-1.0.0.exe
```

Elige el modo que vayas a usar en el cliente. Registra el equipo con un token de
pruebas cuando se abra el panel. Después:

```bash
powershell -ExecutionPolicy Bypass -File .\windows-verify.ps1
```

Debe terminar con **`Fallos: 0`**. Comprueba 20 cosas, entre ellas las tres que se
escaparon: que el runtime de Visual C++ viaje junto a `pdftoppm.exe`, que
`pdftoppm` y `pdftocairo` arranquen de verdad (y que `pdftocairo` sepa imprimir y
admita nombres de impresora con tildes), y que el `config.yaml` no tenga rutas entre comillas
dobles.

Variantes:

```bash
powershell -ExecutionPolicy Bypass -File .\windows-verify.ps1 -Printer "XP-80"
powershell -ExecutionPolicy Bypass -File .\windows-verify.ps1 -SkipConnection
```

## 2. Comprobaciones manuales

Lo que ningún script puede ver por ti:

| # | Paso | Qué debe pasar |
|---|---|---|
| 1 | **Reiniciar la VM** e iniciar sesión | El agente arranca solo, sin abrir ninguna ventana ni consola |
| 2 | Mirar la bandeja del sistema | Icono presente; el menú muestra estado **Conectado**, empresa y equipo |
| 3 | Bandeja → *Abrir panel* | Abre el panel en el navegador con el estado en verde |
| 4 | Panel → Impresoras | Lista las impresoras de la VM |
| 5 | Panel → *Probar* | Sale un ticket de prueba **sin que parpadee ninguna ventana negra** |
| 6 | Imprimir un documento **desde el ERP** | Llega y sale por la impresora correcta |
| 7 | Panel → Configuración | Cambiar el token y guardar reconecta el agente |
| 8 | Desinstalar desde *Configuración → Aplicaciones* | Desaparece el servicio/autoarranque; la carpeta de datos se conserva |

En modo **servicio** el paso 2 y 3 no aplican (los servicios de Windows no pueden
mostrar bandeja); en su lugar comprueba que el servicio `TeraAgent` está *En
ejecución* antes de iniciar sesión. El registro (paso 7) NO se hace por el panel
(deja la conexión en solo lectura): usa `tera-agent register --scope service`
desde una consola de administrador.

## 2b. Comprobaciones de seguridad (permisos)

`windows-verify.ps1` ya cubre lo esencial (carpeta sin herencia y sin escritura de
usuarios sin privilegios, config no legible por Usuarios en servicio, `log.file`
dentro de `data_dir`, panel que rechaza POST de otro origen y registro por panel
bloqueado en servicio). Para confirmarlo **a mano**, en modo servicio y con una
**cuenta estándar** (no administrador):

| # | Paso | Qué debe pasar |
|---|---|---|
| 1 | `icacls C:\ProgramData\TeraAgent` | Solo `SYSTEM` y `Administradores`; **no** debe aparecer `Usuarios`/`Users` con `(M)`/`(W)` |
| 2 | Como usuario estándar, abrir `C:\ProgramData\TeraAgent\config.yaml` | **Acceso denegado** (el Token no es legible) |
| 3 | Como usuario estándar, crear un fichero en la carpeta de datos | **Acceso denegado** |
| 4 | Editar `config.yaml` a mano y poner `log.file` fuera de la carpeta | El servicio **no arranca** y deja el motivo en `startup-error.log` |

## 3. Prueba de actualización

Importante y fácil de olvidar: instalar la versión nueva **encima** de la anterior.

1. Instala la versión anterior y registra el equipo.
2. Instala la nueva sin desinstalar.
3. Comprueba que **el token y las impresoras se conservan** y que
   `tera-agent.exe version` reporta la versión nueva.

## Si algo falla

Primero, estos dos ficheros:

```
C:\ProgramData\TeraAgent\logs\tera-agent.log   operación normal
C:\ProgramData\TeraAgent\startup-error.log     solo existe si el arranque falló
```

El segundo es el que importa cuando "se instala y no abre nada": ahí queda el
motivo. Si tampoco existe, es que el proceso ni se lanzó — revisa la entrada de
autoarranque o el servicio.

El log del propio instalador queda en `%TEMP%\Setup Log *.txt`.
