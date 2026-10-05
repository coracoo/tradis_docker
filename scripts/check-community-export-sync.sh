#!/usr/bin/env bash
set -euo pipefail

usage() {
  printf 'Usage: %s --candidate DIR --public-dir DIR --source-commit SHA\n' "$0" >&2
  exit 2
}

candidate=""
public_dir=""
source_commit=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --candidate) [[ $# -ge 2 ]] || usage; candidate="$2"; shift 2 ;;
    --public-dir) [[ $# -ge 2 ]] || usage; public_dir="$2"; shift 2 ;;
    --source-commit) [[ $# -ge 2 ]] || usage; source_commit="$2"; shift 2 ;;
    *) usage ;;
  esac
done

[[ -d "$candidate" && -d "$public_dir" && "$source_commit" =~ ^[0-9a-f]{40}$ ]] || usage
candidate="$(cd "$candidate" && pwd -P)"
public_dir="$(cd "$public_dir" && pwd -P)"
manifest="$candidate/.community-export-manifest.json"
[[ -f "$manifest" ]] || { echo 'Community manifest is missing' >&2; exit 1; }

jq -e --arg commit "$source_commit" '
  .source.mode == "git-ref" and
  .source.commit == $commit and
  .source.dirty == false and
  .source.gitHistoryIncluded == false and
  (.files | length > 0)
' "$manifest" >/dev/null || { echo 'Community candidate has no approved Git-ref provenance' >&2; exit 1; }

if find "$candidate" "$public_dir" -type l -print -quit | grep -q .; then
  echo 'Community candidate or public tree contains a symlink' >&2
  exit 1
fi

diff -u \
  <(jq -r '.files[].path' "$manifest" | LC_ALL=C sort) \
  <(find "$candidate" -type f -printf '%P\n' | sed '/^\.community-export-manifest\.json$/d' | LC_ALL=C sort) \
  >/dev/null || { echo 'Community manifest does not cover every candidate file' >&2; exit 1; }

while IFS=$'\t' read -r relative_path expected_hash; do
  [[ -f "$candidate/$relative_path" ]] || { echo "Missing candidate file: $relative_path" >&2; exit 1; }
  actual_hash="$(sha256sum "$candidate/$relative_path" | awk '{print $1}')"
  [[ "$actual_hash" == "$expected_hash" ]] || { echo "Candidate hash mismatch: $relative_path" >&2; exit 1; }
done < <(jq -r '.files[] | [.path, .sha256] | @tsv' "$manifest")

diff -qr -x .git "$candidate" "$public_dir" >/dev/null || {
  echo 'Public tree differs from the approved Community candidate' >&2
  exit 1
}

printf 'community-export-sync: PASS (%s)\n' "$source_commit"
