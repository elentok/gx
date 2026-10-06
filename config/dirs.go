package config

import (
	"os"
	"path/filepath"
)

// xdgBase resolves an XDG base directory: the env var when it holds an
// absolute path (the XDG spec says relative values are invalid and must be
// ignored), otherwise ~/<fallback...>. Same rule on macOS and Linux.
func xdgBase(envKey string, fallback ...string) (string, error) {
	if v := os.Getenv(envKey); filepath.IsAbs(v) {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(append([]string{home}, fallback...)...), nil
}

// DataDir is gx's durable-data directory (~/.local/share/gx, honoring
// XDG_DATA_HOME). The ticket store and scratch workspace live beside each
// other under it.
func DataDir() (string, error) {
	base, err := xdgBase("XDG_DATA_HOME", ".local", "share")
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "gx"), nil
}

// StateDir is gx's runtime-state directory (~/.local/state/gx, honoring
// XDG_STATE_HOME).
func StateDir() (string, error) {
	base, err := UserStateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "gx"), nil
}
