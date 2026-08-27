#!/usr/bin/env bash
# 全平台交叉编译 release 包 → dist/
# 读取 VERSION 与 scripts/platforms.txt（与 build-release.ps1 行为一致）
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
OUT="dist"

PLATFORM_FILTER=()
SERVER_ONLY=0
CLIENT_ONLY=0
NO_ZIP=0

usage() {
  cat <<'EOF'
用法: ./scripts/build-release.sh [选项]

选项:
  -p, --platform GOOS/GOARCH   仅构建指定平台（可多次）
  --server-only                只构建 server
  --client-only                只构建 client
  --no-zip                     不生成 zip
  -h, --help                   帮助

平台列表见 scripts/platforms.txt（默认全部）:
  linux/amd64 linux/arm64
  windows/amd64 windows/arm64
  darwin/amd64 darwin/arm64
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -p|--platform) PLATFORM_FILTER+=("$2"); shift 2 ;;
    --server-only) SERVER_ONLY=1; shift ;;
    --client-only) CLIENT_ONLY=1; shift ;;
    --no-zip) NO_ZIP=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "未知参数: $1"; usage; exit 1 ;;
  esac
done

if [[ ! -f go.mod ]]; then
  echo "错误: 未找到 go.mod"
  exit 1
fi

if [[ ! -f VERSION ]]; then
  echo "错误: 未找到 VERSION（仅开发者维护）"
  exit 1
fi

VERSION="$(tr -d ' \r\n' < VERSION)"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
BUILD_TIME="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
LDFLAGS="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildTime=${BUILD_TIME}"

read_platforms() {
  local line goos goarch
  while IFS= read -r line || [[ -n "$line" ]]; do
    line="${line%%#*}"
    line="$(echo "$line" | xargs)"
    [[ -z "$line" ]] && continue
    if [[ "$line" =~ ^([^/]+)/([^/]+)$ ]]; then
      echo "${BASH_REMATCH[1]}/${BASH_REMATCH[2]}"
    fi
  done < "$SCRIPT_DIR/platforms.txt"
}

mapfile -t ALL_PLATFORMS < <(read_platforms)

if [[ ${#PLATFORM_FILTER[@]} -gt 0 ]]; then
  PLATFORMS=("${PLATFORM_FILTER[@]}")
else
  PLATFORMS=("${ALL_PLATFORMS[@]}")
fi

BUILD_SERVER=1
BUILD_CLIENT=1
[[ $SERVER_ONLY -eq 1 ]] && BUILD_CLIENT=0
[[ $CLIENT_ONLY -eq 1 ]] && BUILD_SERVER=0

echo "==> HaoVPN release build"
echo "    version: $VERSION"
echo "    commit:  $COMMIT"
echo "    time:    $BUILD_TIME"
echo "    targets: ${#PLATFORMS[@]} platform(s)"
echo ""

rm -rf "$OUT"
mkdir -p "$OUT"
cp VERSION "$OUT/VERSION"

ARTIFACT_LINES=()

for p in "${PLATFORMS[@]}"; do
  GOOS="${p%/*}"
  GOARCH="${p#*/}"
  DIR="$OUT/${GOOS}-${GOARCH}"
  mkdir -p "$DIR"
  EXT=""
  [[ "$GOOS" == "windows" ]] && EXT=".exe"

  if [[ $BUILD_SERVER -eq 1 ]]; then
    echo "==> server  $GOOS/$GOARCH"
    GOOS=$GOOS GOARCH=$GOARCH CGO_ENABLED=0 \
      go build -trimpath -ldflags "$LDFLAGS" -o "$DIR/haovpn-server${EXT}" ./cmd/server
    ARTIFACT_LINES+=("server,$GOOS/$GOARCH,$DIR/haovpn-server${EXT}")
  fi

  if [[ $BUILD_CLIENT -eq 1 ]]; then
    echo "==> client  $GOOS/$GOARCH"
    GOOS=$GOOS GOARCH=$GOARCH CGO_ENABLED=0 \
      go build -trimpath -ldflags "$LDFLAGS" -o "$DIR/haovpn-client${EXT}" ./cmd/client
    ARTIFACT_LINES+=("client,$GOOS/$GOARCH,$DIR/haovpn-client${EXT}")
  fi

  if [[ $NO_ZIP -eq 0 ]]; then
    ZIP="$OUT/HaoVPN-${VERSION}-${GOOS}-${GOARCH}.zip"
    (cd "$DIR" && zip -qr "../$(basename "$ZIP")" .)
    echo "    zip: $(basename "$ZIP")"
  fi
done

# 简易 manifest（无 jq 依赖）
{
  echo "{"
  echo "  \"version\": \"$VERSION\","
  echo "  \"commit\": \"$COMMIT\","
  echo "  \"buildTime\": \"$BUILD_TIME\","
  echo "  \"goVersion\": \"$(go version | sed 's/"/\\"/g')\","
  echo "  \"artifacts\": ["
  for i in "${!ARTIFACT_LINES[@]}"; do
    IFS=',' read -r typ plat path <<< "${ARTIFACT_LINES[$i]}"
    comma=","
    [[ $i -eq $((${#ARTIFACT_LINES[@]} - 1)) ]] && comma=""
    echo "    {\"type\": \"$typ\", \"platform\": \"$plat\", \"path\": \"$path\"}$comma"
  done
  echo "  ]"
  echo "}"
} > "$OUT/manifest.json"

echo ""
echo "完成。产物: $OUT/"
find "$OUT" -type f | sort
