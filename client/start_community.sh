#!/bin/sh
set -eu

BACKEND_PORT="${BACKEND_PORT:-8080}"
PROJECT_ROOT="${PROJECT_ROOT:-}"
# 镜像默认生产模式：未提供 ADMIN_PASSWORD 时后端拒绝启动，不回落开发默认口令。
GIN_MODE="${GIN_MODE:-release}"
export BACKEND_PORT PROJECT_ROOT GIN_MODE

if [ -z "$PROJECT_ROOT" ]; then
  echo "FATAL: PROJECT_ROOT 未配置。请设置宿主机 Compose 项目绝对路径，并以相同路径挂载到容器。" >&2
  exit 1
fi

case "$PROJECT_ROOT" in
  /*) ;;
  *)
    echo "FATAL: PROJECT_ROOT 必须是宿主机绝对路径，当前值: $PROJECT_ROOT" >&2
    exit 1
    ;;
esac

mkdir -p /app/client/backend/data
mkdir -p /app/client/backend/dist
mkdir -p "$PROJECT_ROOT"

cat > /app/client/backend/dist/env.js <<EOF
window.__ENV__ = { MANAGEMENT_MODE: "CS" };
EOF

echo "Starting TRADIS Community with BACKEND_PORT=$BACKEND_PORT"
exec /app/backend/backend
