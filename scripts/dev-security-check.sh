#!/usr/bin/env bash
# 检查 server.yaml 安全配置（生产交付前运行）
set -euo pipefail

CFG="${1:-./server.yaml}"

if [ ! -f "$CFG" ]; then
  echo "错误: 配置文件不存在: $CFG"
  exit 1
fi

FAIL=0
warn() { echo "  [WARN] $*"; FAIL=1; }
ok()   { echo "  [OK]   $*"; }

echo "==> 安全检查: $CFG"

if grep -qE 'allow_public_bind:\s*true' "$CFG"; then
  warn "allow_public_bind 为 true（生产环境应为 false）"
else
  ok "allow_public_bind 未开启或为 false"
fi

if grep -qE 'listen_hosts:.*0\.0\.0\.0' "$CFG" || grep -qE '-\s*"?0\.0\.0\.0"?' "$CFG"; then
  if grep -qE 'allow_public_bind:\s*true' "$CFG"; then
    warn "listen_hosts 含 0.0.0.0 且 allow_public_bind=true（仅开发可用）"
  else
    ok "listen_hosts 含 0.0.0.0 但 allow_public_bind=false（启动应被拒绝，符合预期）"
  fi
else
  ok "listen_hosts 未绑定 0.0.0.0"
fi

if grep -qE 'password:\s*"?changeme"?' "$CFG"; then
  warn "admin 密码仍为 changeme"
else
  ok "admin 默认密码已修改（或不在配置文件中）"
fi

if grep -qE 'insecure_skip_verify:\s*true' "$CFG"; then
  warn "存在 insecure_skip_verify: true"
else
  ok "未跳过 TLS 校验"
fi

echo ""
if [ "$FAIL" -eq 0 ]; then
  echo "安全检查通过"
else
  echo "存在警告项，请逐项确认（开发环境可忽略部分 WARN）"
  exit 1
fi
