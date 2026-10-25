// Package osnotify sends OS notification banners. Per ADR 0035 every backend
// compiles on every OS and only builds an argv; Select picks one at runtime.
package osnotify

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Notification is one banner. Banners sharing a Group replace each other.
// Execute is a shell command run on click; Activate is the bundle ID of the
// app brought to the front on click. Link is a URL: a notifier that can open
// one does so on click unless Execute is set, and one that cannot puts it in
// the body. Empty fields are left out.
type Notification struct {
	Title, Body string
	Group       string
	Execute     string
	Activate    string
	Link        string
}

// Notifier shows a banner.
type Notifier interface {
	Notify(ctx context.Context, note Notification) error
}

// Runner runs a command to completion. Injected so tests never run one.
type Runner func(ctx context.Context, name string, args ...string) error

const sendTimeout = 5 * time.Second

// NewSystemNotifier picks the backend for this machine, looking up
// terminal-notifier once. Inside a test binary it returns a no-op, so tests
// that start a server without injecting a Notifier never show a real banner.
func NewSystemNotifier() Notifier {
	if testing.Testing() {
		return noop{}
	}
	return Select(runtime.GOOS, exec.LookPath, execRunner)
}

// Select returns terminal-notifier on darwin when it is on PATH, osascript on
// darwin without it, notify-send on linux when it is on PATH, and a no-op
// everywhere else.
func Select(goos string, lookPath func(string) (string, error), run Runner) Notifier {
	switch goos {
	case "darwin":
		if path, err := lookPath("terminal-notifier"); err == nil {
			return newTerminalNotifier(path, run)
		}
		return newOSAScript(run)
	case "linux":
		if path, err := lookPath("notify-send"); err == nil {
			return newNotifySend(path, run)
		}
	}
	return noop{}
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
