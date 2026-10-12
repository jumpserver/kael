#!/bin/sh
set -eu

if [ -n "${CORE_HOST:-}" ]; then
    until check "${CORE_HOST%/}/api/health/"; do
        echo "wait for jms_core ${CORE_HOST} ready"
        sleep 2
    done
fi

CONFIG_PATH=${KAEL_CONFIG_FILE:-/opt/kael/config.yml}
if [ -f "$CONFIG_PATH" ]; then
    exec /opt/kael/kael -f "$CONFIG_PATH"
fi
exec /opt/kael/kael
