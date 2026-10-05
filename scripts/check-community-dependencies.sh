#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: scripts/check-community-dependencies.sh [--source-dir DIR]

Verify that the Community command's actual Go dependency closure does not
import packages excluded by scripts/community-export.json.
EOF
}

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
default_root="$(cd "$script_dir/.." && pwd -P)"
source_dir="$default_root"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --source-dir)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      source_dir="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      printf 'Unknown argument: %s\n' "$1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

source_dir="$(cd "$source_dir" && pwd -P)"
backend_dir="$source_dir/client/backend"
policy_file="$source_dir/scripts/community-export.json"

[[ -f "$backend_dir/go.mod" ]] || {
  printf 'Missing Community backend module: %s\n' "$backend_dir" >&2
  exit 1
}
[[ -f "$policy_file" ]] || {
  printf 'Missing Community export policy: %s\n' "$policy_file" >&2
  exit 1
}

forbidden_text="$(jq -er '.forbiddenImportPrefixes | if type == "array" and length > 0 then .[] else error("missing forbidden prefixes") end' "$policy_file")" || {
  echo 'Community dependency policy is invalid' >&2
  exit 1
}
dependencies_text="$(
  cd "$backend_dir"
  go list -buildvcs=false -deps -tags community -f '{{.ImportPath}}' ./cmd
)" || {
  echo 'Community Go dependency listing failed' >&2
  exit 1
}
[[ -n "$dependencies_text" ]] || { echo 'Community Go dependency listing was empty' >&2; exit 1; }
mapfile -t forbidden_prefixes <<<"$forbidden_text"
mapfile -t dependencies <<<"$dependencies_text"

violations=()
for dependency in "${dependencies[@]}"; do
  for forbidden_prefix in "${forbidden_prefixes[@]}"; do
    if [[ "$dependency" == "$forbidden_prefix" || "$dependency" == "$forbidden_prefix"/* ]]; then
      violations+=("$dependency")
      break
    fi
  done
done

if [[ ${#violations[@]} -gt 0 ]]; then
  printf 'Community command imports excluded packages:\n' >&2
  printf '%s\n' "${violations[@]}" | LC_ALL=C sort -u | sed 's/^/- /' >&2
  exit 1
fi

printf 'Community dependency check passed: %s\n' "$backend_dir"
