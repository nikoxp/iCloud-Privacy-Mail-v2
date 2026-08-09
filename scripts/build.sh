#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "正在构建 Vue 前端……"
(
  cd "$PROJECT_ROOT/frontend"
  npm ci
  npm run build
)

"$PROJECT_ROOT/scripts/sync-web.sh"

echo "正在构建 Go 服务……"
mkdir -p "$PROJECT_ROOT/bin"
(

  cd "$PROJECT_ROOT"
  go build -o "$PROJECT_ROOT/bin/ipm-server" .
  go build -o "$PROJECT_ROOT/bin/ipm-migrate" ./cmd/migrate
)

echo "构建完成，服务程序位于 bin/ipm-server。"
