#!/usr/bin/env bash
# Reports which vendored skills have upstream changes and, with --merge, merges them into the gx
# copies. See docs/specs/vendored-skills.md.
#
# Usage: sync-vendored-skills.sh --check|--merge [--accept-license SKILL]... [--manifest FILE]
#        [--skills-dir DIR]
#
# --check lists skills with upstream changes. --merge 3-way merges `merge` skills into the gx copies
# under --skills-dir (default: ../skills relative to this script's own directory, not the current
# directory) and reports; `port` skills only get the upstream diff. The
# pin does not move. A changed upstream LICENSE blocks its skill until a reviewer re-runs with
# --accept-license SKILL, which copies the new LICENSE. README.md is never touched.
#
# GX_UPSTREAM_BASE replaces https://github.com as the place upstream repos are fetched from
# (<base>/<owner>/<repo>), so tests never touch the network.
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
manifest="$script_dir/vendored-skills.manifest"
skills_dir="$script_dir/../skills"
mode=""
accepted=""

while [ $# -gt 0 ]; do
	case "$1" in
	--check) mode=check ;;
	--merge) mode=merge ;;
	--accept-license)
		accepted="$accepted ${2:?--accept-license needs a skill name}"
		shift
		;;
	--manifest)
		manifest="${2:?--manifest needs a file}"
		shift
		;;
	--skills-dir)
		skills_dir="${2:?--skills-dir needs a directory}"
		shift
		;;
	*)
		echo "unknown argument: $1" >&2
		exit 2
		;;
	esac
	shift
done

if [ -z "$mode" ]; then
	echo "usage: $0 --check|--merge [--accept-license SKILL]... [--manifest FILE] [--skills-dir DIR]" >&2
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

# Prints the files under <path> at <rev>, relative to <path>.
upstream_files() {
	git -C "$1" ls-tree -r --name-only "$2:$3"
}

# 3-way merges one skill into gx's copy: base = upstream at the pin, ours = gx, theirs = latest.
# The pin does not move.
merge_skill() {
	local skill=$1 dir=$2 path=$3 pin=$4 ours_dir="$skills_dir/$1"
	local clean=() conflicted=() f b o t rc
	local tmp="$work/merge"
	rm -rf "$tmp"
	mkdir -p "$tmp"

	while read -r f; do
		case "$f" in LICENSE | README.md) continue ;; esac
		b=0 o=0 t=0
		git -C "$dir" cat-file -e "$pin:$path/$f" 2>/dev/null && b=1
		[ -f "$ours_dir/$f" ] && o=1
		git -C "$dir" cat-file -e "$latest:$path/$f" 2>/dev/null && t=1
		[ $b = 1 ] && git -C "$dir" show "$pin:$path/$f" >"$tmp/base"
		[ $t = 1 ] && git -C "$dir" show "$latest:$path/$f" >"$tmp/theirs"

		# The code is base, ours, theirs presence (1 = file exists). Codes 000, 010 and 100 need no
		# action: gone everywhere, added only in gx, or deleted on both sides.
		case "$b$o$t" in
		# 111: in all three, so 3-way merge unless upstream did not change it.
		111)
			if cmp -s "$tmp/base" "$tmp/theirs"; then continue; fi
			rc=0
			git merge-file "$ours_dir/$f" "$tmp/base" "$tmp/theirs" || rc=$?
			if [ "$rc" -eq 0 ]; then clean+=("$f"); else conflicted+=("$f"); fi
			;;
		# 001: added upstream, so copy it in.
		001)
			mkdir -p "$(dirname "$ours_dir/$f")"
			cp "$tmp/theirs" "$ours_dir/$f"
			clean+=("$f")
			;;
		# 011: added by both gx and upstream, a conflict unless the contents match.
		011)
			cmp -s "$ours_dir/$f" "$tmp/theirs" || conflicted+=("$f")
			;;
		# 110: deleted upstream, so delete it here too unless gx changed it (then a conflict).
		110)
			if cmp -s "$ours_dir/$f" "$tmp/base"; then
				rm "$ours_dir/$f"
				clean+=("$f")
			else
				conflicted+=("$f")
			fi
			;;
		# 101: deleted in gx, a conflict if upstream changed the file since the pin.
		101)
			cmp -s "$tmp/base" "$tmp/theirs" || conflicted+=("$f")
			;;
		esac
	done < <({
		upstream_files "$dir" "$pin" "$path"
		upstream_files "$dir" "$latest" "$path"
		[ -d "$ours_dir" ] && (cd "$ours_dir" && find . -type f | sed 's|^\./||')
		true
	} | sort -u)

	echo "clean files:"
	[ ${#clean[@]} -eq 0 ] || printf '  %s\n' "${clean[@]}"
	echo "conflicted files:"
	[ ${#conflicted[@]} -eq 0 ] || printf '  %s\n' "${conflicted[@]}"
}

# Returns 0 when the skill may be synced. A changed upstream LICENSE blocks it unless the skill was
# named with --accept-license; an accepted one is copied word for word.
license_gate() {
	local skill=$1 dir=$2 license=$3 pin=$4
	local old="$work/license.old" new="$work/license.new"
	git -C "$dir" show "$pin:$license" >"$old" 2>/dev/null || : >"$old"
	git -C "$dir" show "$latest:$license" >"$new" 2>/dev/null || : >"$new"
	cmp -s "$old" "$new" && return 0
	case " $accepted " in
	*" $skill "*)
		mkdir -p "$skills_dir/$skill"
		cp "$new" "$skills_dir/$skill/LICENSE"
		echo "$skill: license change accepted, LICENSE updated"
		found=1
		return 0
		;;
	esac
	found=1
	echo "$skill: license changed; nothing written for this skill"
	diff -u "$old" "$new" || true
	return 1
}

found=0
while read -r skill repo path license pin strategy; do
	case "$skill" in '' | '#'*) continue ;; esac
	dir=$(fetch "$repo")
	latest=$(git -C "$dir" rev-parse HEAD)
	if ! git -C "$dir" cat-file -e "$latest:$path" 2>/dev/null; then
		found=1
		echo "$skill: upstream path $path no longer exists at $latest; nothing written for this skill"
		continue
	fi
	if [ "$mode" = merge ]; then
		license_gate "$skill" "$dir" "$license" "$pin" || continue
	fi
	count=$(git -C "$dir" rev-list --count "$pin..$latest" -- "$path")
	if [ "$count" -eq 0 ]; then
		continue
	fi
	found=1
	if [ "$mode" = check ]; then
		echo "$skill: $count new upstream commits since $pin"
		continue
	fi
	echo "== $skill: $count new upstream commits since $pin =="
	git -C "$dir" log --oneline "$pin..$latest" -- "$path"
	echo "upstream diff:"
	git -C "$dir" diff "$pin" "$latest" -- "$path"
	if [ "${strategy:-merge}" = merge ]; then
		merge_skill "$skill" "$dir" "$path" "$pin"
	fi
done <"$manifest"

[ "$found" = 1 ] || echo "All vendored skills are up to date."
