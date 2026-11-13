#!/usr/bin/env bash
# Regenerates the web types on a base commit and on the working tree, then
# compares every webui/src/generated/*.ts as one set: the sorted declaration
# lines and the count of `any` must match. Only the re-export line and the
# declaration order may differ.
#
# Usage: scripts/webtypes-diff.sh <base-sha>
set -euo pipefail

if [[ $# -lt 1 ]]; then
	echo "usage: scripts/webtypes-diff.sh <base-sha>" >&2
	exit 2
fi

cd "$(dirname "$0")/.."

base=$1
gen=webui/src/generated
tygo=github.com/gzuidhof/tygo@v0.2.21

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

mkdir "$tmp/base"
git archive "$base" | tar -x -C "$tmp/base"
(cd "$tmp/base" && go run "$tygo" generate >/dev/null)
go run "$tygo" generate >/dev/null

# Header, import and re-export lines differ by design; everything else is a
# declaration line. Blank lines are layout, not declarations.
decls() {
	cat "$1"/*.ts | grep -Ev '^// Code generated|^//////////|^// source:|^import |^export \* |^$' | LC_ALL=C sort
}
anys() {
	cat "$1"/*.ts | grep -Ec '\bany\b' || true
}

decls "$tmp/base/$gen" >"$tmp/before"
decls "$gen" >"$tmp/after"

status=0
diff "$tmp/before" "$tmp/after" || status=1

before_any=$(anys "$tmp/base/$gen")
after_any=$(anys "$gen")
if [[ $before_any != "$after_any" ]]; then
	echo "any count: $before_any at $base, $after_any now"
	status=1
fi

[[ $status -eq 0 ]] && echo "web types match $base"
exit "$status"
