#!/bin/sh
USER_ID=${LOCAL_UID:-1000}
GROUP_ID=${LOCAL_GID:-1000}

# Verifica se o ID de Grupo ja existe dentro do Alpine, se nao existir, cria.
if ! getent group "$GROUP_ID" >/dev/null 2>&1; then
    addgroup -g "$GROUP_ID" appgroup
fi

# Verifica se o ID de Usuario ja existe, se nao existir, cria vinculando ao grupo correto.
if ! getent passwd "$USER_ID" >/dev/null 2>&1; then
    GROUP_NAME=$(getent group "$GROUP_ID" | cut -d: -f1)
    adduser -D -u "$USER_ID" -G "$GROUP_NAME" appuser
fi

# Resolve de forma dinamica o nome do usuario para evitar colisoes com UIDs de sistema.
USER_NAME=$(getent passwd "$USER_ID" | cut -d: -f1)

mkdir -p /app/data
chown -R "$USER_NAME" /app/data

exec su-exec "$USER_NAME" "$@"
