package config

import (
	"os"
	"path/filepath"
)

// legacyUserConfigDirFn resolves gx's pre-hardcoding config base directory
// (os.UserConfigDir's per-OS/XDG-aware result), overridden in tests.
var legacyUserConfigDirFn = os.UserConfigDir

// MigrateDir performs gx's one-time move from the old, platform/XDG-resolved
// config directory to the new hardcoded ~/.config/gx, on first launch after
// the hardcoding change. It never overwrites an already-populated new
// directory, and is a no-op when there's nothing to migrate.
func MigrateDir() error {
	legacyBase, err := legacyUserConfigDirFn()
	if err != nil {
		return nil
	}
	newBase, err := userConfigDirFn()
	if err != nil {
		return nil
	}
	return migrateDir(filepath.Join(legacyBase, "gx"), filepath.Join(newBase, "gx"))
}

// migrateDir moves oldPath to newPath. It's a no-op if oldPath doesn't
// exist, if newPath already exists (never overwrites), or if the two
// resolve to the same path.
func migrateDir(oldPath, newPath string) error {
	if oldPath == newPath {
		return nil
	}
	if _, err := os.Stat(newPath); err == nil {
		return nil
	}
	if _, err := os.Stat(oldPath); err != nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(newPath), 0755); err != nil {
		return err
	}
	return os.Rename(oldPath, newPath)
}
