package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/agentrunner"
)

// Dirs resolves gx's directories from env entries (KEY=VALUE, like
// exec.Cmd.Env) instead of the process env. An env with no HOME is an error,
// never a fallback to os.UserHomeDir.
type Dirs struct {
	env []string
}

// DirsFrom builds Dirs over env.
func DirsFrom(env []string) Dirs { return Dirs{env: env} }

func (d Dirs) lookup(key string) string {
	value := ""
	for _, kv := range d.env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == key {
			value = v
		}
	}
	return value
}

// Home is the env's HOME. An env with no HOME is an error.
func (d Dirs) Home() (string, error) {
	home := d.lookup("HOME")
	if home == "" {
		return "", errors.New("no HOME in env")
	}
	return home, nil
}

// CodexHome is the env's CODEX_HOME, else ~/.codex; "" when neither is known.
func (d Dirs) CodexHome() string {
	if v := d.lookup("CODEX_HOME"); v != "" {
		return v
	}
	if home, err := d.Home(); err == nil {
		return filepath.Join(home, ".codex")
	}
	return ""
}

// Environ is the env entries d resolves from, for a child process's Env.
func (d Dirs) Environ() []string { return d.env }

// Getenv is the env's value for key, or "" when it is unset.
func (d Dirs) Getenv(key string) string { return d.lookup(key) }

// xdgBase resolves an XDG base directory: the env var when it holds an
// absolute path (the XDG spec says relative values are invalid and must be
// ignored), otherwise ~/<fallback...>. Same rule on macOS and Linux.
func (d Dirs) xdgBase(envKey string, fallback ...string) (string, error) {
	if v := d.lookup(envKey); filepath.IsAbs(v) {
		return v, nil
	}
	home, err := d.Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(append([]string{home}, fallback...)...), nil
}

// DataDir is gx's durable-data directory (~/.local/share/gx, honoring
// XDG_DATA_HOME). The ticket store and scratch workspace live beside each
// other under it.
func (d Dirs) DataDir() (string, error) {
	base, err := d.xdgBase("XDG_DATA_HOME", ".local", "share")
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "gx"), nil
}

// StateDir is gx's runtime-state directory (~/.local/state/gx, honoring
// XDG_STATE_HOME).
func (d Dirs) StateDir() (string, error) {
	base, err := d.UserStateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "gx"), nil
}

// UserStateDir is the runtime-state base directory: ~/.local/state on every
// platform, or XDG_STATE_HOME when set. Runtime state (notifications-state.json)
// lives here rather than under UserConfigDir,
// which is reserved for user-edited config (config.json).
func (d Dirs) UserStateDir() (string, error) {
	return d.xdgBase("XDG_STATE_HOME", ".local", "state")
}

// UserConfigDir hardcodes ~/.config as gx's config base directory on every
// platform, deliberately bypassing os.UserConfigDir's per-OS/XDG resolution
// (e.g. ~/Library/Application Support on macOS, or $XDG_CONFIG_HOME when
// set). It's the single source of truth other gx packages call into for
// this decision.
func (d Dirs) UserConfigDir() (string, error) {
	home, err := d.Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config"), nil
}

// UserCacheDir hardcodes ~/.cache as gx's cache base directory on every
// platform, mirroring UserConfigDir's deliberate bypass of per-OS/XDG
// resolution.
func (d Dirs) UserCacheDir() (string, error) {
	home, err := d.Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cache"), nil
}

// FilePath returns the config file path, typically ~/.config/gx/config.json.
func (d Dirs) FilePath() (string, error) {
	return configFilePath(d.UserConfigDir)
}

// Load reads user config from disk. Missing file returns defaults.
func (d Dirs) Load() (Config, error) {
	return loadFrom(d.FilePath, d.DataDir)
}

// LoadRunnerWait is N from the config file, or the default when the file is
// missing or unreadable.
func (d Dirs) LoadRunnerWait() time.Duration {
	cfg, _ := d.Load()
	if cfg.RunnerWait <= 0 {
		return agentrunner.DefaultN
	}
	return cfg.RunnerWait
}

// Init writes the default config file and returns its path.
// It returns an error if the file already exists.
func (d Dirs) Init() (string, error) {
	return initAt(d.FilePath)
}

// The package functions read the process env, so production behaves as before.

// DataDir is DirsFrom(os.Environ()).DataDir.
func DataDir() (string, error) { return DirsFrom(os.Environ()).DataDir() }

// StateDir is DirsFrom(os.Environ()).StateDir.
func StateDir() (string, error) { return DirsFrom(os.Environ()).StateDir() }
