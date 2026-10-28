#!/usr/bin/env bash
# Runs gx's full test command N times to measure flakes, then lists each
# failing test with the number of rounds it failed.
#
# Usage: scripts/soak.sh [N]    (N defaults to 20)
#
# The command is `test.full` from .gx/checks.json at the repo root, so the
# soak and `gx check --full` never drift. Every round runs, also after a
# failing one. Each round's output stays in the work dir printed at the end.
#
# Failing test names are read from `gx brief` failure reports: after a
# `FAIL <tool>: N failed in <time>` line come location lines, then each test
# name indented two spaces (brief/render.go renderUnits).
set -euo pipefail

rounds=${1:-20}

command -v jq >/dev/null || {
	echo "soak.sh: jq is required" >&2
	exit 2
}

root=$(git rev-parse --show-toplevel)
cmd=$(jq -r '.test.full // empty' "$root/.gx/checks.json") || {
	echo "soak.sh: cannot read $root/.gx/checks.json" >&2
	exit 2
}
if [[ -z $cmd ]]; then
	echo "soak.sh: $root/.gx/checks.json has no test.full" >&2
	exit 2
fi

work=$(mktemp -d)
cd "$root"

# failing_names <file>: each failing test name in a round's output, once.
failing_names() {
	awk '
		/^FAIL .*: [0-9]+ failed in / { inblock = 1; next }
		/^$/ || /^==/ { inblock = 0 }
		inblock && /^  [^ ]/ { sub(/^  /, ""); print }
	' "$1" | sort -u
}

failed_rounds=()
for ((i = 1; i <= rounds; i++)); do
	out=$work/round-$i.log
	start=$SECONDS
	status=0
	bash -c "$cmd" >"$out" 2>&1 || status=$?
	secs=$((SECONDS - start))
	if [[ $status -eq 0 ]]; then
		printf 'round %d: pass %ds\n' "$i" "$secs"
	else
		printf 'round %d: FAIL %ds\n' "$i" "$secs"
		failed_rounds+=("$i")
		failing_names "$out" >"$work/round-$i.failed"
	fi
done

echo
echo "output files: $work"

if [[ ${#failed_rounds[@]} -eq 0 ]]; then
	echo "all $rounds rounds passed"
	exit 0
fi

echo "${#failed_rounds[@]} of $rounds rounds failed"
for i in "${failed_rounds[@]}"; do
	if [[ ! -s $work/round-$i.failed ]]; then
		echo "round $i failed, no test names"
	fi
done
cat "$work"/round-*.failed | sort | uniq -c | sort -rn |
	while read -r count name; do
		printf '%d× %s\n' "$count" "$name"
	done
exit 1
