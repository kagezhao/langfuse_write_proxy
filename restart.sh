#!/bin/bash
SCRIPT_PATH=$(cd `dirname $0`; pwd)
cd ${SCRIPT_PATH}

killall langfuse-write-proxy

sleep 1

chmod +x langfuse-write-proxy

nohup ./langfuse-write-proxy &

chmod +r nohup.out

