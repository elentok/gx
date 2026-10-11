// Package osnotify sends OS notification banners. Per ADR 0035 every backend
// compiles on every OS and only builds an argv; Select picks one at runtime.
package osnotify

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Notifier shows a banner with a title and a body.
type Notifier interface {
	Notify(ctx context.Context, title, body string) error
}

// Runner runs a command to completion. Injected so tests never run one.
type Runner func(ctx context.Context, name string, args ...string) error

const sendTimeout = 5 * time.Second

// NewSystemNotifier picks the backend for this machine, looking up
// terminal-notifier once.
func NewSystemNotifier() Notifier {
	return Select(runtime.GOOS, exec.LookPath, execRunner)
}

// Select returns terminal-notifier on darwin when it is on PATH, osascript on
// darwin without it, and a no-op everywhere else.
func Select(goos string, lookPath func(string) (string, error), run Runner) Notifier {
	if goos != "darwin" {
		return noop{}
	}
	if path, err := lookPath("terminal-notifier"); err == nil {
		return newTerminalNotifier(path, run)
	}
	return newOSAScript(run)
}

func execRunner(ctx context.Context, name string, args ...string) error {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return fmt.Errorf("%s: %w: %s", name, err, msg)
		}
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func runWithTimeout(ctx context.Context, run Runner, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	return run(ctx, name, args...)
}
