#!/bin/bash
#

while [ "$(curl -I -m 10 -o /dev/null -s -w %{http_code} ${CORE_HOST}/api/health/)" != "200" ]
do
    echo "wait for jms_core $CORE_HOST ready"
    sleep 2
done

export WORK_DIR=/opt/kael
export COMPONENT_NAME=kael
export EXECUTE_PROGRAM=/opt/kael/kael

if [ ! "$LOG_LEVEL" ]; then
    export LOG_LEVEL=ERROR
fi

echo
date
echo "KAEL Version $VERSION, more see https://www.jumpserver.org"
echo "Quit the server with CONTROL-C."
echo

cd /opt/kael || exit 1
wisp
