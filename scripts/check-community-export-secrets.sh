#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: scripts/check-community-export-secrets.sh [--source-dir DIR]

Reject runtime configuration files, private-key files, and high-confidence
credential markers from a Community export candidate. This is a release gate,
not a replacement for manual review of template/media redistribution rights.
EOF
}

source_dir=""
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

command -v rg >/dev/null 2>&1 || {
  printf 'community export secret check: ripgrep (rg) is required; scan not performed\n' >&2
  exit 2
}

source_dir="${source_dir:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)}"
source_dir="$(cd "$source_dir" && pwd -P)"

violations=0

report() {
  printf 'community export secret check: %s\n' "$*" >&2
  violations=1
}

# ripgrep uses 1 for no matches; every other failure must block the gate.
scan_matches() {
  local status
  if rg --quiet "$@"; then
    return 0
  else
    status=$?
  fi
  if [[ "$status" != 1 ]]; then
    printf 'community export secret check: scan failed (rg exit %s)\n' "$status" >&2
    exit 2
  fi
  return 1
}

while IFS= read -r -d '' file; do
  relative_path="${file#"$source_dir"/}"
  case "$relative_path" in
    client/.env.community.example|client/.env.example)
      continue
      ;;
  esac
  report "runtime configuration file is not exportable: $relative_path"
done < <(find "$source_dir" -type f \( \
  -name '.env*' -o -name '*.env' -o -name '*.pem' -o -name '*.key' -o -name '*.p12' -o -name '*.pfx' \
  -o -name 'id_rsa' -o -name 'id_ed25519' -o -name 'identity.json' \
\) -print0)

if [[ -f "$source_dir/client/.env.example" ]] &&
  scan_matches '^[[:space:]]*(export[[:space:]]+)?(JWT_SECRET|ADMIN_PASSWORD)[[:space:]]*=[[:space:]]*[^[:space:]]' "$source_dir/client/.env.example"; then
  report 'installation example must contain blank credentials'
fi

check_pattern() {
  local label="$1"
  local pattern="$2"
  if scan_matches --pcre2 --hidden --text \
    --glob '!**/.git/**' \
    --glob '!**/node_modules/**' \
    --glob '!**/coverage/**' \
    -e "$pattern" "$source_dir" >&2; then
    report "$label detected"
  fi
}

check_pattern 'private key header' '-----BEGIN (?:[A-Z0-9 ]+ )?PRIVATE KEY-----'
check_pattern 'GitHub token' 'gh[pousr]_[A-Za-z0-9_]{20,}|github_pat_[A-Za-z0-9_]{20,}'
check_pattern 'GitLab token' 'glpat-[A-Za-z0-9_-]{20,}'
check_pattern 'AWS access key' '(?:AKIA|ASIA)[0-9A-Z]{16}'
check_pattern 'OpenAI API key' 'sk-(?:proj-)?[A-Za-z0-9_-]{20,}'
check_pattern 'Slack token' 'xox[baprs]-[A-Za-z0-9-]{20,}'
check_pattern 'TRADIS Agent setup code' 'TRADIS_AGENT_SETUP_CODE=[^[:space:]#]{20,}'

if (( violations )); then
  exit 1
fi

printf 'Community export secret check passed: %s\n' "$source_dir"
