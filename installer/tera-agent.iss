; Instalador de Tera Agent para Windows (Inno Setup 6).
; Owner: DevOps Engineer.
;
; Compilar:  iscc installer\tera-agent.iss /DAgentVersion=1.0.0
; Normalmente no se invoca a mano: scripts\build-release.ps1 compila los binarios,
; prepara el bundle de poppler en installer\staging\ y llama aquí.
;
; Qué resuelve este instalador (y el script de PowerShell no resolvía):
;   * Empaqueta poppler COMPLETO: pdftoppm.exe (rasterizado de tickets y modo
;     imagen), pdftocairo.exe (impresión vectorial en láser/inyección) y sus DLLs.
;     Importan poppler.dll, cairo.dll y lcms2.dll: copiar solo los .exe deja la
;     impresión de PDF rota.
;   * Elige UN dueño del WebSocket (servicio O app de usuario, nunca los dos):
;     dos agentes con el mismo token duplicarían los trabajos de impresión.
;   * Arranque automático real, desinstalador en "Aplicaciones", y actualización
;     in-place sobre la instalación anterior (mismo AppId).

#ifndef AgentVersion
  #define AgentVersion "0.0.0-dev"
#endif

#define AppName      "Tera Agent"
#define AppPublisher "Grupo Tera"
#define ServiceName  "TeraAgent"
; Valor de HKLM\...\Run que arranca la bandeja al iniciar sesión. Se declara
; aquí porque lo escribe [Registry] y lo lee InstalledMode en [Code].
#define RunValueName "TeraAgent"
#define PanelURL     "http://127.0.0.1:9180"

[Setup]
; El AppId es la identidad de la instalación: NO cambiarlo entre versiones o
; cada release se instalaría al lado de la anterior en vez de actualizarla.
AppId={{8F3C1A72-5B4E-4D9A-9E27-3A6D2C8B1F04}
AppName={#AppName}
AppVersion={#AgentVersion}
AppPublisher={#AppPublisher}
DefaultDirName={autopf}\TeraAgent
DefaultGroupName={#AppName}
OutputDir=..\dist
OutputBaseFilename=TeraAgent-Setup-{#AgentVersion}
Compression=lzma2/max
SolidCompression=yes
; El servicio y las carpetas de Program Files necesitan elevación.
PrivilegesRequired=admin
ArchitecturesInstallIn64BitMode=x64compatible
ArchitecturesAllowed=x64compatible
WizardStyle=modern
; Icono del propio instalador, de los accesos directos y de la entrada en
; "Aplicaciones". Los .exe no llevan icono incrustado, así que se instala el
; .ico junto a ellos y se apunta aquí (ver assets/brand/make-icons.sh).
SetupIconFile=tera-agent.ico
WizardSmallImageFile=wizard-small.bmp,wizard-small@2x.bmp
UninstallDisplayIcon={app}\tera-agent.ico
; Cierra el tray si está corriendo, para poder reemplazar el .exe sin reiniciar.
CloseApplications=yes
RestartApplications=no
SetupLogging=yes

[Languages]
Name: "es"; MessagesFile: "compiler:Languages\Spanish.isl"

[CustomMessages]
es.ModePageCaption=Modo de ejecución
es.ModePageDescription=¿Cómo debe arrancar Tera Agent en este equipo?
es.ModeUser=Al iniciar sesión de Windows, con icono en la bandeja y panel (recomendado)
es.ModeService=Como servicio de Windows: arranca sin que nadie inicie sesión, sin icono en la bandeja
es.ModeWarning=Elige solo una: si el agente corriera como servicio Y como aplicación de usuario a la vez, habría dos agentes conectados con el mismo Token y el ERP recibiría cada impresión duplicada.
es.ModeKeepUser=Este equipo ya tiene Tera Agent instalado como aplicación de usuario y así se va a dejar. Cámbialo solo si sabes que quieres cambiarlo.
es.ModeKeepService=Este equipo ya tiene Tera Agent instalado como servicio y así se va a dejar. Cámbialo solo si sabes que quieres cambiarlo.

[Tasks]
Name: "desktopicon"; Description: "Crear un acceso directo al panel en el escritorio"; GroupDescription: "Accesos directos:"

[Files]
; Binario de consola: servicio, CLI y diagnóstico (printers, print, version).
Source: "staging\tera-agent.exe";      DestDir: "{app}"; Flags: ignoreversion
; Binario GUI (-H=windowsgui): el mismo programa sin consola, para que el
; autoarranque no muestre una ventana negra al iniciar sesión.
Source: "staging\tera-agent-tray.exe"; DestDir: "{app}"; Flags: ignoreversion
; Poppler en {app}\poppler\bin\ — es uno de los sitios donde el Agent lo busca
; (ver popplerbin.Find en adapters/printing/popplerbin). Las DLLs van en la
; misma carpeta que pdftoppm.exe y pdftocairo.exe porque Windows resuelve primero
; el directorio del propio ejecutable, así no hace falta tocar el PATH del sistema.
Source: "staging\poppler\bin\*";       DestDir: "{app}\poppler\bin"; Flags: ignoreversion recursesubdirs
; Poppler es GPL: distribuirlo obliga a acompañarlo de su licencia.
Source: "staging\poppler\COPYING";     DestDir: "{app}\poppler"; Flags: ignoreversion
; PDF de una página (600 bytes) para que scripts\windows-verify.ps1 y el soporte
; puedan ejercitar el pipeline completo PDF→raster→ESC/POS sin gastar papel.
Source: "staging\testpage.pdf";        DestDir: "{app}"; Flags: ignoreversion
; Icono de la marca: lo usan los accesos directos, el desinstalador y la
; entrada de "Aplicaciones". No viene de staging: es fuente del repositorio.
Source: "tera-agent.ico";              DestDir: "{app}"; Flags: ignoreversion

[Dirs]
; Carpeta de datos del SERVICIO: config (con el Token), logs y estado de trabajos.
; Solo se crea en modo servicio. Antes llevaba "Permissions: users-modify", lo que
; dejaba que CUALQUIER usuario del equipo leyera el Token y reescribiera la config
; que el servicio arranca como LocalSystem (escalada de privilegios). Ahora NO se
; abren permisos aquí: en [Run], "setup-data --scope service" restringe la carpeta
; a SYSTEM y Administradores (ver adapters/fsacl). En modo usuario la config vive
; en el perfil del usuario (%LOCALAPPDATA%), protegida por los permisos del perfil,
; y la crea el propio agente en su primer arranque.
Name: "{commonappdata}\TeraAgent"; Check: IsServiceMode

[Icons]
; IconFilename en los accesos al panel: sin él, Windows les pondría el icono del
; navegador predeterminado, porque apuntan a una URL y no a un .exe.
Name: "{group}\Panel de Tera Agent";        Filename: "{#PanelURL}"; IconFilename: "{app}\tera-agent.ico"
; La carpeta de datos solo tiene una ruta fija en modo servicio (ProgramData). En
; modo usuario vive en el perfil de cada usuario; ábrela desde el panel.
Name: "{group}\Carpeta de datos y logs";    Filename: "{commonappdata}\TeraAgent"; Check: IsServiceMode
Name: "{group}\{cm:UninstallProgram,{#AppName}}"; Filename: "{uninstallexe}"
Name: "{autodesktop}\Panel de Tera Agent";  Filename: "{#PanelURL}"; IconFilename: "{app}\tera-agent.ico"; Tasks: desktopicon

[Registry]
; Autoarranque al iniciar sesión (modo "app de usuario"). HKLM y no HKCU: el
; instalador corre elevado, así que HKCU escribiría en el perfil del
; administrador y no en el del cajero que usa el POS.
; --scope user (no una ruta fija): el agente resuelve la config en el perfil del
; usuario que inicia sesión (%LOCALAPPDATA%\TeraAgent), no en el del administrador
; que instaló. Cada cuenta tiene su propio registro; nada se comparte entre ellas.
Root: HKLM; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; \
  ValueType: string; ValueName: "{#RunValueName}"; \
  ValueData: """{app}\tera-agent-tray.exe"" run --tray --scope user"; \
  Flags: uninsdeletevalue; Check: IsUserMode

[Run]
; --- Modo servicio ---
; 1) setup-data crea la carpeta de datos y RESTRINGE sus permisos (SYSTEM +
;    Administradores) y escribe la config por defecto. Sustituye al antiguo
;    WriteDefaultConfig en Pascal: al escribirla el propio agente desaparece el
;    bug de las comillas YAML y la carpeta nunca pasa por permisos abiertos.
Filename: "{app}\tera-agent.exe"; Parameters: "setup-data --scope service"; \
  StatusMsg: "Preparando la carpeta de datos (permisos seguros)..."; Flags: runhidden waituntilterminated; Check: IsServiceMode
; 2) El servicio se instala y arranca con --scope service (ver service_windows.go):
;    resuelve ProgramData, verifica que sus permisos son seguros y, si no lo son,
;    se niega a arrancar.
Filename: "{app}\tera-agent.exe"; Parameters: "service install"; \
  StatusMsg: "Instalando el servicio de Windows..."; Flags: runhidden waituntilterminated; Check: IsServiceMode
Filename: "{app}\tera-agent.exe"; Parameters: "service start"; \
  StatusMsg: "Arrancando el servicio..."; Flags: runhidden waituntilterminated; Check: IsServiceMode
; 3) Registro del equipo: en servicio se hace por consola de administrador, no por
;    el panel (que deja la conexión en solo lectura). Pide URL y Token sin eco.
Filename: "{app}\tera-agent.exe"; Parameters: "register --scope service"; \
  Description: "Registrar el equipo ahora (introducir URL y Token del ERP)"; \
  Flags: postinstall waituntilterminated skipifsilent; Check: IsServiceMode

; --- Modo app de usuario: arranca ya, sin esperar a reiniciar sesión ---
; runasoriginaluser es importante: el instalador corre elevado, y sin este flag
; la bandeja arrancaría como administrador — distinto de cómo arrancará al
; iniciar sesión, y escribiendo en el perfil equivocado. --scope user hace que el
; agente cree/lea la config en el perfil de ESE usuario.
Filename: "{app}\tera-agent-tray.exe"; Parameters: "run --tray --scope user"; \
  Description: "Iniciar Tera Agent ahora"; Flags: nowait postinstall skipifsilent runasoriginaluser; Check: IsUserMode

; --- Abrir el panel para registrar el Token (solo modo usuario) ---
; También como usuario original, para que abra SU navegador y su sesión. En modo
; servicio el registro es por consola (paso 3), no por el panel.
Filename: "{#PanelURL}"; Description: "Abrir el panel para registrar el equipo"; \
  Flags: postinstall shellexec skipifsilent nowait runasoriginaluser; Check: IsUserMode

[UninstallRun]
; Parar y borrar el servicio antes de quitar los ficheros. RunOnceId evita que se
; repita si el desinstalador se reejecuta.
Filename: "{app}\tera-agent.exe"; Parameters: "service stop";      Flags: runhidden; RunOnceId: "StopSvc"
Filename: "{app}\tera-agent.exe"; Parameters: "service uninstall"; Flags: runhidden; RunOnceId: "DelSvc"
; Cerrar el proceso de bandeja para poder borrar el .exe.
Filename: "{sys}\taskkill.exe"; Parameters: "/F /IM tera-agent-tray.exe"; Flags: runhidden; RunOnceId: "KillTray"

[Code]
var
  ModePage: TInputOptionWizardPage;

const
  ModeUserIndex    = 0;
  ModeServiceIndex = 1;

// InstalledMode dice con qué modo está ya instalado el agente en este equipo:
// 'service' si existe el servicio, 'user' si está el autoarranque de sesión, y
// cadena vacía si es una instalación nueva. El instalador corre en modo 64 bits
// (ArchitecturesInstallIn64BitMode), así que lee las mismas claves que escribe.
function InstalledMode: String;
begin
  if RegKeyExists(HKEY_LOCAL_MACHINE, 'SYSTEM\CurrentControlSet\Services\{#ServiceName}') then
    Result := 'service'
  else if RegValueExists(HKEY_LOCAL_MACHINE, 'Software\Microsoft\Windows\CurrentVersion\Run', '{#RunValueName}') then
    Result := 'user'
  else
    Result := '';
end;

// Página de elección del modo. Son excluyentes a propósito (radio buttons):
// el WebSocket contra el ERP debe tener un único dueño en el equipo.
procedure InitializeWizard;
begin
  ModePage := CreateInputOptionPage(wpSelectTasks,
    ExpandConstant('{cm:ModePageCaption}'),
    ExpandConstant('{cm:ModePageDescription}'),
    ExpandConstant('{cm:ModeWarning}'),
    True,   { True = radio buttons, excluyentes }
    False);
  ModePage.Add(ExpandConstant('{cm:ModeUser}'));
  ModePage.Add(ExpandConstant('{cm:ModeService}'));

  // En una actualización se premarca el modo que el equipo ya tiene: instalar
  // encima a base de "siguiente" no debe cambiarlo sin querer. Si cambiara,
  // quedarían el servicio y la app de usuario con el mismo Token y el ERP
  // recibiría cada impresión dos veces.
  if InstalledMode = 'service' then
  begin
    ModePage.Values[ModeServiceIndex] := True;
    ModePage.SubCaptionLabel.Caption := ExpandConstant('{cm:ModeKeepService}');
  end
  else
  begin
    ModePage.Values[ModeUserIndex] := True;
    if InstalledMode = 'user' then
      ModePage.SubCaptionLabel.Caption := ExpandConstant('{cm:ModeKeepUser}');
  end;
end;

// En instalación silenciosa no hay asistente que responder, así que el modo se
// pasa por línea de comandos para poder desplegar en masa:
//
//   TeraAgent-Setup-1.0.0.exe /VERYSILENT /MODE=service
//
// Sin /MODE se respeta lo elegido en el asistente (por defecto, modo usuario).
function ModeParam: String;
begin
  Result := Lowercase(Trim(ExpandConstant('{param:MODE|}')));
end;

function IsUserMode: Boolean;
begin
  if ModeParam <> '' then
    Result := (ModeParam = 'user')
  else
    Result := ModePage.Values[ModeUserIndex];
end;

function IsServiceMode: Boolean;
begin
  if ModeParam <> '' then
    Result := (ModeParam = 'service')
  else
    Result := ModePage.Values[ModeServiceIndex];
end;

// La configuración inicial ya NO se escribe aquí en Pascal, sino con
// "tera-agent.exe setup-data" (ver [Run]): así el propio agente crea la carpeta
// con permisos seguros y escribe un YAML válido (el antiguo bug de las comillas
// dobles en las rutas de Windows desaparece porque lo genera Go, no este script).

// Al desinstalar, dejamos la config y los logs: si el cliente reinstala no tiene
// que volver a registrar el equipo, y los logs sirven para el soporte posterior.
// Borrarlos es decisión del usuario (el atajo a la carpeta sigue en el menú).
