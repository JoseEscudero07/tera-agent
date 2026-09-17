#!/usr/bin/env python3
"""Ejecuta PowerShell dentro de la VM de pruebas por WinRM.

    winrun.py 'Get-Service TeraAgent'
    winrun.py - < script.ps1

Sale con el mismo código que el script remoto. Normalmente se usa a través de
vm.sh (vm.sh run ...), que prepara el entorno Python con pywinrm.
Owner: QA Engineer.
"""
import os
import sys

import winrm

HOST = os.environ.get("WIN_HOST", "http://127.0.0.1:5985/wsman")
USER = os.environ.get("WIN_USER", "cajero")
PASS = os.environ.get("TERA_VM_PASSWORD", "tera-test")


def main() -> int:
    if len(sys.argv) != 2:
        print(__doc__, file=sys.stderr)
        return 2
    script = sys.stdin.read() if sys.argv[1] == "-" else sys.argv[1]
    # UTF-8 en la salida: los mensajes del agente y de Windows van en español.
    script = "[Console]::OutputEncoding=[Text.Encoding]::UTF8\n$ProgressPreference='SilentlyContinue'\n" + script
    session = winrm.Session(HOST, auth=(USER, PASS), transport="basic",
                            operation_timeout_sec=1700, read_timeout_sec=1800)
    r = session.run_ps(script)
    sys.stdout.write(r.std_out.decode("utf-8", "replace"))
    err = r.std_err.decode("utf-8", "replace")
    # pywinrm devuelve el progreso de PowerShell como CLIXML en stderr; solo
    # interesa si el script falló.
    if err and r.status_code != 0:
        sys.stderr.write(err)
    return r.status_code


if __name__ == "__main__":
    sys.exit(main())
