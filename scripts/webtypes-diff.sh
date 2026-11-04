#!/usr/bin/env bash
# Compares the generated web types (server.ts plus wire.ts) with server.ts at a
# base commit: the sorted declaration lines and the count of `any` must match.
# Run `make web-types` first. Temporary: removed once the wire split is done.
#
# Usage: scripts/webtypes-diff.sh <base-sha>
set -euo pipefail

cd "$(dirname "$0")/.."

base=${1:?usage: scripts/webtypes-diff.sh <base-sha>}
gen=webui/src/generated

# Header, import and re-export lines differ by design; everything else is a
# declaration line.
decls() {
	grep -Ev '^// Code generated|^//////////|^// source:|^import |^export \* ' | LC_ALL=C sort
}
anys() {
	grep -Ec '\bany\b' || true
}

before=$(mktemp)
after=$(mktemp)
trap 'rm -f "$before" "$after"' EXIT

git show "$base:$gen/server.ts" | decls >"$before"
cat "$gen/server.ts" "$gen/wire.ts" | decls >"$after"

status=0
diff "$before" "$after" || status=1

before_any=$(git show "$base:$gen/server.ts" | anys)
after_any=$(cat "$gen/server.ts" "$gen/wire.ts" | anys)
if [[ $before_any != "$after_any" ]]; then
	echo "any count: $before_any at $base, $after_any now"
	status=1
fi

[[ $status -eq 0 ]] && echo "web types match $base"
exit "$status"
