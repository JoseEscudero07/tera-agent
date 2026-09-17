# Desinstalacion de Tera Agent. EJECUTAR COMO ADMINISTRADOR.
#
# Se mantiene por compatibilidad con la documentacion anterior; la logica vive en
# windows-install.ps1 -Uninstall para no duplicarla (una sola fuente de verdad
# sobre como se localiza el desinstalador).
#
# Normalmente el usuario desinstala desde Windows: Configuracion > Aplicaciones >
# Tera Agent > Desinstalar.
$ErrorActionPreference = "Stop"
& (Join-Path $PSScriptRoot "windows-install.ps1") -Uninstall
