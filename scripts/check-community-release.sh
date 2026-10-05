#!/usr/bin/env bash
# Public Community release gate; no private service or repository dependency.
set -euo pipefail
root="${1:?candidate root required}"
version="${2:?release tag required}"
commit="${3:-$(jq -er '.source.commit' "$root/.community-export-manifest.json")}"
[[ "$version" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || { echo 'Invalid stable release tag' >&2; exit 1; }
[[ "$(jq -er '.version' "$root/client/frontend/package.json")" == "${version#v}" ]] || { echo 'Candidate package version mismatch' >&2; exit 1; }
[[ "$(jq -er '.version' "$root/.community-export-manifest.json")" == "${version#v}" ]] || { echo 'Candidate manifest version mismatch' >&2; exit 1; }
[[ -s "$root/LICENSE" ]] || { echo 'Community LICENSE missing' >&2; exit 1; }
# Actions checks out a Git repository; only its metadata is outside the snapshot.
snapshot="$(mktemp -d)"
trap 'rm -rf "$snapshot"' EXIT
rsync -a --exclude /.git "$root/" "$snapshot/"
bash "$root/scripts/check-community-export-sync.sh" --candidate "$snapshot" --public-dir "$root" --source-commit "$commit"
