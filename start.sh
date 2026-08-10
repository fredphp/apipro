#!/usr/bin/env bash
# Start script: launches Redis (if not running), apipro-rpc, and apipro-api.
set -e
cd "$(dirname "$0")"

export PATH="$HOME/.local/go/bin:$HOME/.local/bin:$HOME/go/bin:$PATH"

# 1. Redis
if ! redis-cli -p 6399 ping >/dev/null 2>&1; then
  echo "starting redis on :6399 ..."
  redis-server --port 6399 --daemonize yes --save "" --appendonly no
fi

# 2. Build
echo "building ..."
go build -o bin/apipro-rpc ./cmd/rpc
go build -o bin/apipro-api ./cmd/api

# 3. Start RPC (background)
pkill -f bin/apipro-rpc 2>/dev/null || true
pkill -f bin/apipro-api 2>/dev/null || true
sleep 1
nohup ./bin/apipro-rpc -f cmd/rpc/etc/apipro.yaml > /tmp/apipro-rpc.log 2>&1 &
echo "apipro-rpc pid=$! (log: /tmp/apipro-rpc.log)"
sleep 2

# 4. Start API (foreground)
echo "starting apipro-api on :3100 ..."
exec ./bin/apipro-api -f cmd/api/etc/apipro.yaml
