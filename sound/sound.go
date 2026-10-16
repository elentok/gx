// Package sound plays the Agents tab's sounds. Like osnotify (ADR 0035) it
// only builds an argv and runs it through an injected Runner; goos picks the
// backend at runtime, with no build tags.
package sound

import (
	"bytes"
	"context"
	"embed"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/config"
)

// Name is a sound slot.
type Name string

const (
	Attention Name = "attention"
	Done      Name = "done"
)

// The built-in sounds are CC0 from Kenney's Interface Sounds pack, converted
// to AAC because afplay cannot play Ogg Vorbis.
//
//go:embed sounds/*.m4a
var builtins embed.FS

// Player plays a sound. It runs synchronously and never fails: problems are
// logged, so callers that do not want to wait run it on a goroutine.
type Player interface {
	Play(ctx context.Context, cfg config.AgentsSoundsConfig, name Name)
}

// Runner runs a command to completion. Injected so tests never run one.
type Runner func(ctx context.Context, name string, args ...string) error

const playTimeout = 10 * time.Second

// NewSystemPlayer returns the player for this machine. Inside a test binary
// it returns a no-op, so tests never make a real sound.
func NewSystemPlayer(log *slog.Logger) Player {
	if testing.Testing() {
		return noop{}
	}
	cfgPath, err := config.FilePath()
	if err != nil {
		log.Warn("sound: resolve config dir", "err", err)
		return noop{}
	}
	dataDir, err := config.DataDir()
	if err != nil {
		log.Warn("sound: resolve data dir", "err", err)
		return noop{}
	}
	return New(runtime.GOOS, execRunner, filepath.Dir(cfgPath), filepath.Join(dataDir, "sounds"), log)
}

// New returns afplay on darwin and a no-op elsewhere. Custom sound paths are
// relative to configDir; built-in sounds are copied into builtinDir.
func New(goos string, run Runner, configDir, builtinDir string, log *slog.Logger) Player {
	if goos != "darwin" {
		return noop{}
	}
	return &afplay{run: run, configDir: configDir, builtinDir: builtinDir, log: log}
}

type noop struct{}

func (noop) Play(context.Context, config.AgentsSoundsConfig, Name) {}

type afplay struct {
	run        Runner
	configDir  string
	builtinDir string
	log        *slog.Logger
	copyOnce   sync.Once
}

func (a *afplay) Play(ctx context.Context, cfg config.AgentsSoundsConfig, name Name) {
	if !cfg.Enabled {
		return
	}
	if custom := customPath(cfg, name); custom != "" {
		if !filepath.IsAbs(custom) {
			custom = filepath.Join(a.configDir, custom)
		}
		if _, err := os.Stat(custom); err != nil {
			a.log.Warn("sound: custom sound missing, using built-in", "sound", name, "path", custom, "err", err)
		} else if err := a.play(ctx, custom); err != nil {
			a.log.Warn("sound: custom sound failed, using built-in", "sound", name, "path", custom, "err", err)
		} else {
			return
		}
	}
	a.copyOnce.Do(a.copyBuiltins)
	if err := a.play(ctx, a.builtinPath(name)); err != nil {
		a.log.Warn("sound: built-in sound failed", "sound", name, "err", err)
	}
}

func (a *afplay) play(ctx context.Context, path string) error {
	ctx, cancel := context.WithTimeout(ctx, playTimeout)
	defer cancel()
	return a.run(ctx, "afplay", path)
}

func (a *afplay) builtinPath(name Name) string {
	return filepath.Join(a.builtinDir, string(name)+".m4a")
}

// copyBuiltins writes each embedded sound to builtinDir unless the file
// already holds the same bytes.
func (a *afplay) copyBuiltins() {
	if err := os.MkdirAll(a.builtinDir, 0o755); err != nil {
		a.log.Warn("sound: create built-in dir", "path", a.builtinDir, "err", err)
		return
	}
	for _, name := range []Name{Attention, Done} {
		data, _ := builtins.ReadFile("sounds/" + string(name) + ".m4a")
		path := a.builtinPath(name)
		if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, data) {
			continue
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			a.log.Warn("sound: write built-in sound", "path", path, "err", err)
		}
	}
}

func customPath(cfg config.AgentsSoundsConfig, name Name) string {
	if name == Done {
		return cfg.Done
	}
	return cfg.Attention
}

func execRunner(ctx context.Context, name string, args ...string) error {
	return exec.CommandContext(ctx, name, args...).Run()
}
