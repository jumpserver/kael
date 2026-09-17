#!/bin/sh
set -eu

CONFIG_PATH=${KAEL_CONFIG_FILE:-/opt/kael/config.yml}
if [ -f "$CONFIG_PATH" ]; then
    exec /opt/kael/kael -f "$CONFIG_PATH"
fi
exec /opt/kael/kael
