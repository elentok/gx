#!/usr/bin/env bash
# Reports which vendored skills have upstream changes. See docs/specs/vendored-skills.md.
#
# Usage: sync-vendored-skills.sh --check [--manifest FILE]
#
# GX_UPSTREAM_BASE replaces https://github.com as the place upstream repos are fetched from
# (<base>/<owner>/<repo>), so tests never touch the network.
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
manifest="$script_dir/vendored-skills.manifest"
mode=""

while [ $# -gt 0 ]; do
	case "$1" in
	--check) mode=check ;;
	--manifest)
		manifest="${2:?--manifest needs a file}"
		shift
		;;
	*)
		echo "unknown argument: $1" >&2
		exit 2
		;;
	esac
	shift
done

if [ "$mode" != check ]; then
	echo "usage: $0 --check [--manifest FILE]" >&2
	exit 2
fi

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

upstream_url() {
	if [ -n "${GX_UPSTREAM_BASE:-}" ]; then
		echo "$GX_UPSTREAM_BASE/$1"
	else
		echo "https://github.com/$1.git"
	fi
}

# Full clone, so every pin is reachable. One clone per upstream repo per run.
fetch() {
	local dir="$work/${1//\//__}"
	[ -d "$dir" ] || git clone --quiet --bare "$(upstream_url "$1")" "$dir"
	echo "$dir"
}

found=0
while read -r skill repo path _license pin _strategy; do
	case "$skill" in '' | '#'*) continue ;; esac
	dir=$(fetch "$repo")
	count=$(git -C "$dir" rev-list --count "$pin..HEAD" -- "$path")
	if [ "$count" -gt 0 ]; then
		echo "$skill: $count new upstream commits since $pin"
		found=1
	fi
done <"$manifest"

[ "$found" = 1 ] || echo "All vendored skills are up to date."
