package main

import (
	"os"
	"path/filepath"
)

const (
	// plentyFree is well above both default thresholds, so a scenario that does
	// not steer the disk never shows a strip or holds dispatch.
	plentyFree = "100G"
	// diskWatchInterval is short so a flow sees a change fast.
	diskWatchInterval = "250ms"
)

// cleanScriptBody reads `clean` next to it: a size string is written to `free`, `fail`
// fails with a message, and an empty or missing file does nothing. `free` is
// replaced by a rename, so the server never reads a half-written file.
const cleanScriptBody = `#!/bin/sh
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
`

// diskDir sits outside the project repo, so the trunk stays clean.
func (f *fixture) diskDir() string { return filepath.Join(f.root, "disk") }

func (f *fixture) cleanScript() string { return filepath.Join(f.diskDir(), "clean.sh") }

// setupDisk gives every scenario the fake disk: the server reads its free space
// from <disk dir>/free, which starts at plentyFree.
func (f *fixture) setupDisk() error {
	if err := os.MkdirAll(f.diskDir(), 0o755); err != nil {
		return err
	}
	free := filepath.Join(f.diskDir(), "free")
	if err := os.WriteFile(free, []byte(plentyFree+"\n"), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(f.cleanScript(), []byte(cleanScriptBody), 0o755); err != nil {
		return err
	}
	f.env = append(f.env, "GX_DISK_FREE_FILE="+free, "GX_DISK_WATCH_INTERVAL="+diskWatchInterval)
	return nil
}
