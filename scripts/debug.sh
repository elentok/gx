#!/usr/bin/env bash
#
# Debugs why worktree statuses may not be showing correctly.
# Run from inside any git repo or bare repo.
#
# Usage:
#   scripts/debug.sh           # diagnose only
#   scripts/debug.sh --fix     # diagnose and fix issues automatically

set -euo pipefail

FIX=false
if [[ "${1:-}" == "--fix" ]]; then
	FIX=true
fi

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
DIM='\033[2m'
BOLD='\033[1m'
RESET='\033[0m'

ok() { echo -e "  ${GREEN}✓${RESET} $*"; }
warn() { echo -e "  ${YELLOW}!${RESET} $*"; }
fail() { echo -e "  ${RED}✗${RESET} $*"; }
dim() { echo -e "  ${DIM}$*${RESET}"; }

# ── repo root ─────────────────────────────────────────────────────────────────

REPO_ROOT=$(git rev-parse --show-toplevel 2>/dev/null || git rev-parse --git-dir 2>/dev/null)
if [[ -z "$REPO_ROOT" ]]; then
	echo "Not inside a git repository."
	exit 1
fi
# For bare repos, git-dir IS the root
if [[ "$REPO_ROOT" == "." ]]; then
	REPO_ROOT="$PWD"
fi

echo ""
echo -e "${BOLD}Repo:${RESET} $REPO_ROOT"
echo ""

# ── remotes ───────────────────────────────────────────────────────────────────

echo -e "${BOLD}Remotes${RESET}"
REMOTES=$(git -C "$REPO_ROOT" remote)
if [[ -z "$REMOTES" ]]; then
	fail "No remotes configured — status cannot be determined"
else
	while IFS= read -r remote; do
		url=$(git -C "$REPO_ROOT" remote get-url "$remote" 2>/dev/null || echo "(unknown)")
		ok "$remote  ${DIM}$url${RESET}"
	done <<<"$REMOTES"
fi
echo ""

# ── fetch refspec ─────────────────────────────────────────────────────────────

echo -e "${BOLD}Fetch refspec (origin)${RESET}"
REFSPEC=$(git -C "$REPO_ROOT" config remote.origin.fetch 2>/dev/null || true)
EXPECTED_REFSPEC="+refs/heads/*:refs/remotes/origin/*"
if [[ -z "$REFSPEC" ]]; then
	fail "No fetch refspec for origin"
elif [[ "$REFSPEC" == "$EXPECTED_REFSPEC" ]]; then
	ok "$REFSPEC"
	dim "Remote tracking refs will be stored under refs/remotes/origin/*"
else
	warn "$REFSPEC"
	dim "Expected $EXPECTED_REFSPEC for remote tracking refs to work"
	if [[ "$FIX" == true ]]; then
		git -C "$REPO_ROOT" config remote.origin.fetch "$EXPECTED_REFSPEC"
		ok "Fixed: set fetch refspec to $EXPECTED_REFSPEC"
	else
		dim "Fix: git config remote.origin.fetch '$EXPECTED_REFSPEC'  (or re-run with --fix)"
	fi
fi
echo ""

# ── remote tracking refs ──────────────────────────────────────────────────────

echo -e "${BOLD}Remote tracking refs (refs/remotes/origin/*)${RESET}"
REMOTE_REFS=$(git -C "$REPO_ROOT" for-each-ref --format='%(refname:short)' refs/remotes/origin/ 2>/dev/null || true)
if [[ -z "$REMOTE_REFS" ]]; then
	fail "No remote tracking refs found"
	if [[ "$FIX" == true ]]; then
		echo ""
		git -C "$REPO_ROOT" fetch origin
		ok "Fetched origin"
		REMOTE_REFS=$(git -C "$REPO_ROOT" for-each-ref --format='%(refname:short)' refs/remotes/origin/ 2>/dev/null || true)
	else
		dim "Fix: git fetch origin  (or re-run with --fix)"
	fi
fi
if [[ -n "$REMOTE_REFS" ]]; then
	while IFS= read -r ref; do
		hash=$(git -C "$REPO_ROOT" rev-parse --short "$ref")
		dim "$ref  ($hash)"
	done <<<"$REMOTE_REFS"
fi
echo ""

# ── worktrees ─────────────────────────────────────────────────────────────────

echo -e "${BOLD}Worktrees${RESET}"
while IFS= read -r line; do
	if [[ "$line" == worktree* ]]; then
		wt_path="${line#worktree }"
	elif [[ "$line" == branch* ]]; then
		full_branch="${line#branch }"
		branch="${full_branch#refs/heads/}"

		echo ""
		echo -e "  ${BOLD}$branch${RESET}  ${DIM}($wt_path)${RESET}"

		# configured upstream
		configured=$(git -C "$REPO_ROOT" for-each-ref --format='%(upstream:short)' "refs/heads/$branch" 2>/dev/null || true)
		if [[ -n "$configured" ]]; then
			# verify the ref actually resolves
			if git -C "$REPO_ROOT" rev-parse --verify "$configured" &>/dev/null; then
				ok "configured upstream: $configured"
			else
				fail "configured upstream: $configured  (ref does not exist — run: git fetch origin)"
			fi
		else
			warn "no upstream configured"
			# check fallback
			candidate="origin/$branch"
			if git -C "$REPO_ROOT" rev-parse --verify "$candidate" &>/dev/null; then
				ok "fallback ref exists: $candidate  (will be used automatically)"
			else
				fail "fallback ref missing: $candidate  (run: git fetch origin, then press t to track)"
			fi
		fi
	fi
done < <(git -C "$REPO_ROOT" worktree list --porcelain)

echo ""
