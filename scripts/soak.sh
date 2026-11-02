#!/usr/bin/env bash
# Runs gx's full test check N times to measure flakes, then lists each
# failing test with the number of rounds it failed.
#
# Usage: scripts/soak.sh [N] [--fail-fast]    (N defaults to 20)
#
# Each round is `gx check --full --kind test`, so a round takes the check
# lock, writes a run record and sets GX_CHECK_RUN, like a real full run. Every
# round runs, also after a failing one, unless --fail-fast is given: then the
# script stops at the first failed round, prints its failing tests and exits 1.
# Each round's output stays in the work dir printed at the end.
#
# The script runs the `gx` on PATH. The caller must have the `gx` built from
# this checkout on PATH (scripts/test.sh builds its own for the same reason).
#
# Failing test names are read from the round's test log, which `gx check`
# prints as `test log: <path>`. They come from `gx brief` failure reports:
# after a `FAIL <tool>: N failed in <time>` line come location lines, then each
# test name indented two spaces (brief/render.go renderUnits).
set -euo pipefail

rounds=20
fail_fast=0
for arg in "$@"; do
	case $arg in
	--fail-fast) fail_fast=1 ;;
	*) rounds=$arg ;;
	esac
done

root=$(git rev-parse --show-toplevel)
work=$(mktemp -d)
cd "$root"

# failing_names <round output>: each failing test name in the round's test
# log, once.
failing_names() {
	local log
	log=$(sed -n 's/^test log: //p' "$1" | head -n 1)
	[[ -n $log && -f $log ]] || return 0
	awk '
		/^FAIL .*: [0-9]+ failed in / { inblock = 1; next }
		/^$/ || /^==/ { inblock = 0 }
		inblock && /^  [^ ]/ { sub(/^  /, ""); print }
	' "$log" | sort -u
}

failed_rounds=()
for ((i = 1; i <= rounds; i++)); do
	out=$work/round-$i.log
	start=$SECONDS
	status=0
	gx check --full --kind test >"$out" 2>&1 || status=$?
	secs=$((SECONDS - start))
	if [[ $status -eq 0 ]]; then
		printf 'round %d: pass %ds\n' "$i" "$secs"
	else
		printf 'round %d: FAIL %ds\n' "$i" "$secs"
		failed_rounds+=("$i")
		failing_names "$out" >"$work/round-$i.failed"
		if [[ $fail_fast -eq 1 ]]; then
			echo
			echo "output files: $work"
			if [[ -s $work/round-$i.failed ]]; then
				cat "$work/round-$i.failed"
			else
				echo "round $i failed, no test names"
			fi
			exit 1
		fi
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
