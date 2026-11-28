#!/usr/bin/env bash
# 把本仓库的后台二进制、管理端静态资源和 App 接口文档拷到 tata-deploy。
# 不覆盖生产 config.yaml，不动 uploads，不重启任何进程。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DEPLOY="${TATA_DEPLOY:-$ROOT/../tata-deploy}"
if [[ -d "$DEPLOY" ]]; then
  DEPLOY="$(cd "$DEPLOY" && pwd)"
fi
GOPROXY_VALUE="${GOPROXY:-https://proxy.golang.org,direct}"

DO_SERVER=0
DO_ADMIN=0
DO_DOCS=0
HAVE_TARGET=0

usage() {
  cat <<EOF
用法: $(basename "$0") [选项]

默认拷到: $DEPLOY
可用环境变量 TATA_DEPLOY 改目标目录。

选项:
  --all       后台 + 管理端 + App 接口文档（默认）
  --server    只编 Linux amd64 二进制
  --admin     只编管理端并同步静态资源
  --docs      只拷 docs/openapi-app.yaml
  --deploy DIR
              指定目标目录
  -h, --help  看这一段

不会做的事:
  不改 $DEPLOY/server/manifest/config/config.yaml
  不碰 $DEPLOY/server/uploads/
  不重启本机 :8080，也不往远端发
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --all) DO_SERVER=1; DO_ADMIN=1; DO_DOCS=1; HAVE_TARGET=1; shift ;;
    --server) DO_SERVER=1; HAVE_TARGET=1; shift ;;
    --admin) DO_ADMIN=1; HAVE_TARGET=1; shift ;;
    --docs) DO_DOCS=1; HAVE_TARGET=1; shift ;;
    --deploy)
      DEPLOY="${2:?--deploy 后面要跟目录}"
      shift 2
      ;;
    -h|--help) usage; exit 0 ;;
    *)
      echo "未知参数: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ $HAVE_TARGET -eq 0 ]]; then
  DO_SERVER=1
  DO_ADMIN=1
  DO_DOCS=1
fi

if [[ ! -d "$DEPLOY" ]]; then
  echo "目标目录不存在: $DEPLOY" >&2
  echo "确认机器上已有 tata-deploy，或用 --deploy / TATA_DEPLOY 指定。" >&2
  exit 1
fi

DEPLOY="$(cd "$DEPLOY" && pwd)"
echo "仓库: $ROOT"
echo "目标: $DEPLOY"

package_server() {
  local tmp bin
  command -v go >/dev/null || { echo "需要 go" >&2; exit 1; }
  mkdir -p "$DEPLOY/server"
  tmp="$(mktemp -t tata-linux-amd64.XXXXXX)"
  bin="$DEPLOY/server/tata-linux-amd64"
  echo "编译 Linux amd64 二进制…"
  (
    cd "$ROOT/server"
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOPROXY="$GOPROXY_VALUE" \
      go build -trimpath -ldflags "-s -w" -o "$tmp" ./cmd/tata
  )
  mv "$tmp" "$bin"
  chmod +x "$bin"
  echo "二进制: $bin"
  ls -lh "$bin"
  file "$bin"
}

package_admin() {
  local dist
  command -v pnpm >/dev/null || { echo "需要 pnpm" >&2; exit 1; }
  dist="$ROOT/admin/apps/web-antd/dist"
  echo "编译管理端 @vben/web-antd…"
  (
    cd "$ROOT/admin"
    pnpm run build:antd
  )
  if [[ ! -f "$dist/index.html" ]]; then
    echo "管理端产物缺少 index.html: $dist" >&2
    exit 1
  fi
  mkdir -p "$DEPLOY/admin"
  echo "同步管理端静态资源（会删掉目标里多出来的旧文件）…"
  rsync -a --delete --exclude '*.zip' "$dist/" "$DEPLOY/admin/"
  echo "管理端: $DEPLOY/admin/index.html"
}

package_docs() {
  local src dest
  src="$ROOT/docs/openapi-app.yaml"
  dest="$DEPLOY/docs/openapi-app.yaml"
  if [[ ! -f "$src" ]]; then
    echo "缺少 $src" >&2
    exit 1
  fi
  mkdir -p "$DEPLOY/docs"
  cp "$src" "$dest"
  echo "App 接口文档: $dest"
}

[[ $DO_SERVER -eq 1 ]] && package_server
[[ $DO_ADMIN -eq 1 ]] && package_admin
[[ $DO_DOCS -eq 1 ]] && package_docs

echo
echo "完成。config.yaml 和 uploads 都没动。"
echo "本机 :8080 也没重启。把 $DEPLOY 里的文件拷到线上后再自己重启远端进程。"
