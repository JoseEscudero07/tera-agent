# Inno Setup 6 bajo Wine, para compilar el instalador de Windows desde Linux.
# Lo construye y usa scripts/build-release.sh; no hace falta invocarlo a mano.
# Owner: DevOps Engineer.
FROM debian:trixie-slim

# Versión y SHA256 del instalador oficial. Al subir de versión, actualiza los dos:
# el hash evita que un asset reemplazado en GitHub acabe compilando los
# instaladores que llegan a los clientes.
ARG INNO_VERSION=6.7.3
ARG INNO_SHA256=9c73c3bae7ed48d44112a0f48e66742c00090bdb5bef71d9d3c056c66e97b732

RUN dpkg --add-architecture i386 \
 && apt-get update \
 && apt-get install -y --no-install-recommends wine wine32:i386 wine64 xvfb xauth ca-certificates curl \
 && rm -rf /var/lib/apt/lists/*

ENV WINEDEBUG=-all WINEPREFIX=/opt/wine

# Release oficial (repositorio jrsoftware/issrc), versión fijada.
RUN set -e; tag="is-$(echo "$INNO_VERSION" | tr . _)"; \
    curl -fsSL -o /tmp/is.exe "https://github.com/jrsoftware/issrc/releases/download/${tag}/innosetup-${INNO_VERSION}.exe"; \
    echo "${INNO_SHA256}  /tmp/is.exe" | sha256sum -c -; \
    xvfb-run -a wine /tmp/is.exe /VERYSILENT /SUPPRESSMSGBOXES /NORESTART /SP- /CURRENTUSER "/DIR=C:\\InnoSetup" || true; \
    wineserver -w; rm -f /tmp/is.exe; \
    test -f /opt/wine/drive_c/InnoSetup/ISCC.exe; \
    test -f /opt/wine/drive_c/InnoSetup/Languages/Spanish.isl

ENTRYPOINT ["wine", "C:\\InnoSetup\\ISCC.exe"]
