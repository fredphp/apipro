#!/usr/bin/env bash
# 守护脚本：脱离会话，常驻运行
cd /home/z/my-project/apipro
export PATH="$HOME/.local/go/bin:$HOME/.local/bin:$HOME/go/bin:$PATH"
pkill -f apipro-rpc 2>/dev/null
pkill -f apipro-api 2>/dev/null
sleep 1
./bin/apipro-rpc -f cmd/rpc/etc/apipro.yaml > /tmp/apipro-rpc.log 2>&1 &
RPCPID=$!
sleep 2
./bin/apipro-api -f cmd/api/etc/apipro.yaml > /tmp/apipro-api.log 2>&1 &
APIPID=$!
echo "$RPCPID" > /tmp/apipro-rpc.pid
echo "$APIPID" > /tmp/apipro-api.pid
# 守护循环：保持脚本不退出
while kill -0 $RPCPID 2>/dev/null || kill -0 $APIPID 2>/dev/null; do
  sleep 5
done
