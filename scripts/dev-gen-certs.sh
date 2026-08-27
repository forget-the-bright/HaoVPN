#!/usr/bin/env bash
# 生成开发用自签 CA + 服务端证书 → ./certs/
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
CERT_DIR="${1:-./certs}"
DAYS=3650

mkdir -p "$CERT_DIR"

if ! command -v openssl &>/dev/null; then
  echo "错误: 需要 openssl"
  exit 1
fi

echo "==> 生成 CA"
openssl genrsa -out "$CERT_DIR/ca.key" 4096 2>/dev/null
openssl req -new -x509 -days $DAYS -key "$CERT_DIR/ca.key" -out "$CERT_DIR/ca.crt" \
  -subj "/CN=HaoVPN Dev CA"

echo "==> 生成服务端证书"
openssl genrsa -out "$CERT_DIR/server.key" 2048 2>/dev/null
openssl req -new -key "$CERT_DIR/server.key" -out "$CERT_DIR/server.csr" \
  -subj "/CN=HaoVPN-dev.local"
openssl x509 -req -days $DAYS -in "$CERT_DIR/server.csr" \
  -CA "$CERT_DIR/ca.crt" -CAkey "$CERT_DIR/ca.key" -CAcreateserial \
  -out "$CERT_DIR/server.crt"

rm -f "$CERT_DIR/server.csr" "$CERT_DIR/ca.srl"

echo ""
echo "完成:"
echo "  CA:     $CERT_DIR/ca.crt"
echo "  服务端: $CERT_DIR/server.crt / $CERT_DIR/server.key"
echo ""
echo "client.yaml 中 tls.ca_file 指向 ca.crt"
