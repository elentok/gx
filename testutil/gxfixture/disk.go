package main

import (
	_ "embed"
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

// cleanScriptBody (cleanscript.sh) reads `clean` next to it: a size string is written to
// `free`, `fail` fails with a message, and an empty or missing file does nothing.
//
//go:embed cleanscript.sh
var cleanScriptBody string

// diskDir sits outside the project repo, so the trunk stays clean.
func (f *fixture) diskDir() string { return filepath.Join(f.root, "disk") }

// rateLimitReleaseFile is outside the repo for the same reason as diskDir.
func (f *fixture) rateLimitReleaseFile() string { return filepath.Join(f.root, "ratelimit-release") }

func (f *fixture) lateFinishScanHold() string { return filepath.Join(f.root, "late-finish-scan-hold") }

func cleanScript(diskDir string) string { return filepath.Join(diskDir, "clean.sh") }

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
	if err := os.WriteFile(cleanScript(f.diskDir()), []byte(cleanScriptBody), 0o755); err != nil {
		return err
	}
	f.env = append(f.env, "GX_DISK_FREE_FILE="+free, "GX_DISK_WATCH_INTERVAL="+diskWatchInterval, "GX_USAGE_POLL_INTERVAL="+diskWatchInterval)
	return nil
}
