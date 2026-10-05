#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
dockerfile="$root_dir/client/Dockerfile.community"
compose_file="$root_dir/client/docker-compose.community.yml"
build_compose="$root_dir/client/docker-compose.community.build.yml"
env_example="$root_dir/client/.env.community.example"
start_script="$root_dir/client/start_community.sh"
dockerignore="$root_dir/client/.dockerignore"
install_compose="$root_dir/client/docker-compose.yml"
install_env="$root_dir/client/.env.example"

for file in "$dockerfile" "$compose_file" "$build_compose" "$env_example" "$start_script" "$dockerignore"; do
  if [[ ! -f "$file" ]]; then
    echo "missing Community image artifact: ${file#$root_dir/}" >&2
    exit 1
  fi
done
test -f "$install_compose" && test -f "$install_env" || { echo 'Missing default installation files' >&2; exit 1; }
grep -Fq 'image: coracoo/tradis:latest' "$install_compose"
grep -Fq '# 社区版' "$install_compose"
if grep -Eq '^    build:|CLIENT_VERSION:' "$install_compose"; then
  echo 'Default installation must pull the released Full image without a local build or version override' >&2
  exit 1
fi
if grep -Eq '^    build:|CLIENT_VERSION:' "$compose_file" || ! grep -Fq '${TRADIS_COMMUNITY_IMAGE:?' "$compose_file"; then
  echo 'Released installation must require an explicit image, without rebuilding or overriding its version' >&2
  exit 1
fi
grep -Fq 'Dockerfile.community' "$build_compose"

if ! grep -Fxq 'data/' "$dockerignore"; then
  echo 'Community Docker build context must exclude the root data directory' >&2
  exit 1
fi

if ! grep -Fq 'go build -tags community' "$dockerfile"; then
  echo 'Community Dockerfile must compile the backend with -tags community' >&2
  exit 1
fi
if ! grep -Fq 'VITE_TRADIS_EDITION=community' "$dockerfile"; then
  echo 'Community Dockerfile must build the Community frontend entry' >&2
  exit 1
fi
runtime_workdir="$(awk '/^FROM alpine:/{in_runtime=1; next} in_runtime && /^WORKDIR /{workdir=$2} END{print workdir}' "$dockerfile")"
if [[ "$runtime_workdir" != '/app/client/backend' ]]; then
  echo 'Community runtime workdir must match the backend static asset root' >&2
  exit 1
fi
if grep -Eqi 'tradis-agent|LICENSE_|GITHUB_APP|APPSTORE_SERVER_URL|TRADIS_REMOTE_AGENT' "$dockerfile" "$compose_file" "$env_example" "$start_script"; then
  echo 'Community image artifacts must not contain commercial runtime configuration' >&2
  exit 1
fi
if ! grep -Fq 'PROJECT_ROOT' "$compose_file" || ! grep -Fq 'APPSTORE_CDN_URL' "$compose_file"; then
  echo 'Community Compose must retain local project and CDN configuration' >&2
  exit 1
fi
if ! grep -Fq '${TRADIS_COMMUNITY_ENV_FILE:-.env}' "$compose_file"; then
  echo 'Community Compose must allow an explicit environment file for validation and automation' >&2
  exit 1
fi

echo 'community-image-contract: PASS'
