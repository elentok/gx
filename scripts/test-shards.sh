#!/usr/bin/env bash
# Runs one package's tests split across N concurrent processes.
#
# Usage: scripts/test-shards.sh <package-dir> [shards]
#
# The server tests change process-wide env (PATH, HOME) through t.Setenv, so
# they can't use t.Parallel and run one at a time in a single process. Separate
# processes have separate env: build the test binary once, deal its tests out
# round-robin by index, and run the groups side by side.
set -euo pipefail

pkg=${1:?usage: test-shards.sh <package-dir> [shards]}
shards=${2:-${TEST_SHARDS:-5}}
timeout=${TEST_TIMEOUT:-10m}

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
root=$(pwd)

go test -c -o "$work/pkg.test" "$pkg"

# Tests read testdata relative to their package dir, as under `go test`.
cd "$pkg"

"$work/pkg.test" -test.list '^Test' | grep '^Test' >"$work/tests.txt" || true
if [[ ! -s $work/tests.txt ]]; then
	exit 0
fi

pids=()
for ((i = 0; i < shards; i++)); do
	re=$(awk -v n="$shards" -v i="$i" 'NR % n == i { printf "%s^%s$", sep, $0; sep = "|" }' "$work/tests.txt")
	[[ -n $re ]] || continue
	"$work/pkg.test" -test.run "$re" -test.timeout="$timeout" >"$work/shard$i.out" 2>&1 &
	pids+=("$!")
done

status=0
for idx in "${!pids[@]}"; do
	if ! wait "${pids[$idx]}"; then
		status=1
	fi
done

for f in "$work"/shard*.out; do
	# Passing shards only print PASS; keep their output out of the way.
	if [[ $status -ne 0 ]] || ! grep -qx PASS "$f"; then
		cat "$f"
	fi
done

if [[ $status -eq 0 ]]; then
	echo "ok  	$pkg (sharded x$shards)"
else
	echo "FAIL	$pkg (sharded x$shards)" >&2
fi
cd "$root"
exit "$status"
