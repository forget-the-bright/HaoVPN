#!/usr/bin/env bash
# 冒烟测试：编译（若需要）→ 启动 server → health 检查 → 停止
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

BIN="./haovpn-server"
CFG="${1:-./server.yaml}"
HEALTH_URL="http://127.0.0.1:8080/api/v1/health"
PID=""

cleanup() {
  if [ -n "$PID" ] && kill -0 "$PID" 2>/dev/null; then
    echo "==> 停止 server (pid $PID)"
    kill "$PID" 2>/dev/null || true
    wait "$PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

if [ ! -f go.mod ]; then
  echo "跳过: 未找到 go.mod，代码落地后再运行"
  exit 0
fi

if [ ! -x "$BIN" ]; then
  echo "==> 编译 server"
  go build -o "$BIN" ./cmd/server
fi

echo "==> 启动 server (-c $CFG)"
"$BIN" -c "$CFG" &
PID=$!
sleep 2

if ! kill -0 "$PID" 2>/dev/null; then
  echo "错误: server 启动失败，查看日志"
  exit 1
fi

echo "==> health check: $HEALTH_URL"
if command -v curl &>/dev/null; then
  curl -sf "$HEALTH_URL" && echo "" && echo "冒烟测试通过"
else
  echo "提示: 未安装 curl，请手动访问 $HEALTH_URL"
fi
