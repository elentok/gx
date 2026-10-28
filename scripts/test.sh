#!/usr/bin/env bash
# Runs every non-e2e test step side by side, then prints one line per step and
# the failing steps with their `gx brief` reports.
#
# Usage: [GX_TEST_CPUS=<n>] scripts/test.sh
#
# GX_TEST_CPUS is the CPU budget for all steps together. It defaults to half the
# cores (all of them on a machine with 4 cores or fewer).
#
# The steps are separate processes, each with its own output file:
#   race    the PTY-runner packages under -race (a vt emulator shared with a
#           background goroutine, and herdrrunner's use of the herdr fake)
#   go      every other package except ./server and ./e2e
#   server  ./server
#   web     make web-test (install, tsc, vitest)
#
# -timeout=10m per package: CI runners' lower core counts give
# concurrency-sensitive tests more room to run long. -count=1 keeps the test
# cache out of the timings.
set -euo pipefail

cd "$(dirname "$0")/.."

# A commit's detached auto maintenance can outlive a test and break its
# t.TempDir cleanup. Repos made by testutil carry the setting in their config;
# this covers packages that make repos with their own `git init`.
export GIT_CONFIG_COUNT=1
export GIT_CONFIG_KEY_0=maintenance.auto
export GIT_CONFIG_VALUE_0=false

module=github.com/elentok/gx
race_pkgs=(
	./agentrunner/screen
	./agentrunner/claudeui
	./nativerunner
	./claudedoctor
	./agentrunner/herdrrunner
	./testutil/herdrfake
)

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# Build gx from the checkout: CI has no installed gx, and an installed one can
# be older than the brief index fields `gx check` relies on.
go build -o "$work/gx" .
gx=$work/gx

# Package paths (not dirs) of everything the "go" step runs.
skip=$(printf '%s\n' "$module/server" "$module/e2e" "${race_pkgs[@]/#.\//$module/}")
other_pkgs=()
while IFS= read -r pkg; do
	other_pkgs+=("$pkg")
done < <(go list ./... | grep -Fvx -f <(echo "$skip"))

names=(race go server web)

# run_step <name> <command...>: writes the command's output to $work/<name>.log
# and its exit status and wall time to $work/<name>.status.
run_step() {
	local name=$1 start=$SECONDS status=0
	shift
	"$@" >"$work/$name.log" 2>&1 || status=$?
	echo "$status $((SECONDS - start))" >"$work/$name.status"
}

# The steps share one CPU budget so the machine does not saturate. GOMAXPROCS
# caps a Go step's packages in flight (-p) and its parallel tests (-parallel).
# server and go get two fifths each, race and web get one worker each, so the
# total is about the budget. Machines with 4 cores or fewer get no cap.
cores=$(getconf _NPROCESSORS_ONLN)
if ((cores > 4)); then budget=$((cores / 2)); else budget=$cores; fi
budget=${GX_TEST_CPUS:-$budget}
big=$((budget * 2 / 5))
((big >= 1)) || big=1

run_step race env GOMAXPROCS=1 "$gx" brief go-test -race -timeout=10m -count=1 "${race_pkgs[@]}" &
pids=("$!")
run_step go env GOMAXPROCS="$big" "$gx" brief go-test -timeout=10m -count=1 "${other_pkgs[@]}" &
pids+=("$!")
run_step server env GOMAXPROCS="$big" "$gx" brief go-test -timeout=10m -count=1 ./server &
pids+=("$!")
run_step web make web-test GX="$gx" WEB_TEST_WORKERS=1 &
pids+=("$!")
for pid in "${pids[@]}"; do
	wait "$pid" || true
done

failed=()
for name in "${names[@]}"; do
	read -r status secs <"$work/$name.status"
	if [[ $status -eq 0 ]]; then
		printf 'PASS %-6s %4ss\n' "$name" "$secs"
	else
		printf 'FAIL %-6s %4ss\n' "$name" "$secs"
		failed+=("$name")
	fi
done

for name in "${failed[@]}"; do
	echo
	echo "== $name =="
	cat "$work/$name.log"
done

[[ ${#failed[@]} -eq 0 ]]
