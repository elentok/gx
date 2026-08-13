package config

import (
	"errors"
	"fmt"
	"io"
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

// WarnOnMigrateFailure runs MigrateDir and MigrateStateFiles and, for either
// that fails, writes a non-fatal warning to w instead of returning the error
// - callers should proceed with startup regardless of migration outcome.
func WarnOnMigrateFailure(w io.Writer) {
	if err := MigrateDir(); err != nil {
		fmt.Fprintf(w, "warning: failed to migrate config directory: %v\n", err)
	}
	if err := MigrateStateFiles(); err != nil {
		fmt.Fprintf(w, "warning: failed to migrate state files: %v\n", err)
	}
}

// stateFileNames are gx's per-machine runtime state files, moved out of
// ~/.config/gx/ into ~/.local/state/gx/ (see UserStateDir) on first launch
// after the split - config.json and other user-edited config stay under
// UserConfigDir.
var stateFileNames = []string{"queue-state.json", "notifications-state.json"}

// MigrateStateFiles moves each of stateFileNames from the old
// ~/.config/gx/ location to the new ~/.local/state/gx/ one, individually
// (never the enclosing directory, since config.json must stay behind). Like
// migrateDir, it never overwrites an already-migrated file and is a no-op
// when there's nothing to migrate.
func MigrateStateFiles() error {
	oldBase, err := userConfigDirFn()
	if err != nil {
		return nil
	}
	newBase, err := userStateDirFn()
	if err != nil {
		return nil
	}
	var errs []error
	for _, name := range stateFileNames {
		if err := migrateDir(filepath.Join(oldBase, "gx", name), filepath.Join(newBase, "gx", name)); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
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
