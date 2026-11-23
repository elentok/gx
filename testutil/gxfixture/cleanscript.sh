#!/bin/sh
dir=$(dirname "$0")
want=$(cat "$dir/clean" 2>/dev/null)
case "$want" in
'') exit 0 ;;
fail)
	echo "fixture cleanup failed" >&2
	exit 1
	;;
esac
printf '%s\n' "$want" > "$dir/free.tmp" && mv "$dir/free.tmp" "$dir/free"
