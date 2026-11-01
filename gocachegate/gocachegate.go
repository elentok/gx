// Package gocachegate is the wait gate between Go builds and Go cache
// cleaners. A build holds a file lock shared; a cleaner holds it exclusive and
// also waits while a `go` process runs, which catches builds that never
// registered. It is a plain flock under the state dir, so it works with the
// server down or out of reach of the sandbox.
package gocachegate

import (
	"context"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	lockName   = "gx-gocache.lock"
	defaultCap = 10 * time.Minute
	defaultGap = 50 * time.Millisecond
)

// Deps holds the hooks tests replace. The zero value is the production setup.
type Deps struct {
	// Scan reports whether a `go` process runs. Nil lists processes with ps.
	Scan func() (bool, error)
	// Cap bounds WaitExclusive. Zero means ten minutes.
	Cap time.Duration
	// Poll is the gap between lock and scan attempts. Zero means 50ms.
	Poll time.Duration
	// Logf receives diagnostics. Nil means the standard logger.
	Logf func(format string, args ...any)
}

func noop() {}

func (d Deps) logf(format string, args ...any) {
	if d.Logf != nil {
		d.Logf(format, args...)
		return
	}
	log.Printf(format, args...)
}

func (d Deps) poll() time.Duration {
	if d.Poll > 0 {
		return d.Poll
	}
	return defaultGap
}

func openLock(stateDir string) (*os.File, error) {
	return os.OpenFile(filepath.Join(stateDir, lockName), os.O_CREATE|os.O_RDWR, 0o600)
}

// Register takes the shared lock for a Go build and returns its release. A
// build must never fail because the gate is unavailable, so an unopenable
// lock logs and returns a no-op release.
func Register(stateDir string) (release func()) {
	return Deps{}.Register(stateDir)
}

// Register is the package-level Register with these hooks.
func (d Deps) Register(stateDir string) (release func()) {
	f, err := openLock(stateDir)
	if err != nil {
		d.logf("gocachegate: build runs unregistered: %v", err)
		return noop
	}
	// A cleaner waits at most its cap, so a shared wait is short; block.
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH); err != nil {
		f.Close()
		d.logf("gocachegate: build runs unregistered: %v", err)
		return noop
	}
	return func() { f.Close() }
}

// WaitExclusive waits for the exclusive lock and for no `go` process. It holds
// the lock while it waits for processes, so new builds queue behind the
// cleaner. It gives up after the cap and reports capPassed; release then
// frees whatever was taken (nothing, if the lock never came).
func (d Deps) WaitExclusive(ctx context.Context, stateDir string) (release func(), capPassed bool, err error) {
	f, err := openLock(stateDir)
	if err != nil {
		d.logf("gocachegate: cleaner runs ungated: %v", err)
		return noop, false, nil
	}
	limit := d.Cap
	if limit <= 0 {
		limit = defaultCap
	}
	scan := d.Scan
	if scan == nil {
		scan = scanGoProcess
	}
	deadline := time.NewTimer(limit)
	defer deadline.Stop()
	tick := time.NewTicker(d.poll())
	defer tick.Stop()

	locked := false
	closeFile := func() { f.Close() }
	for {
		if !locked {
			switch err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err {
			case nil:
				locked = true
			case syscall.EWOULDBLOCK:
			default:
				f.Close()
				return noop, false, err
			}
		}
		if locked {
			busy, err := scan()
			if err != nil {
				d.logf("gocachegate: process scan failed: %v", err)
			}
			if err == nil && !busy {
				return closeFile, false, nil
			}
		}
		select {
		case <-ctx.Done():
			f.Close()
			return noop, false, ctx.Err()
		case <-deadline.C:
			d.logf("gocachegate: cap passed, cleaning anyway")
			return closeFile, true, nil
		case <-tick.C:
		}
	}
}

// scanGoProcess lists processes with ps, which behaves the same on macOS and
// Linux, and looks for an executable named go.
func scanGoProcess() (bool, error) {
	out, err := exec.Command("ps", "-axo", "comm=").Output()
	if err != nil {
		return false, err
	}
	for line := range strings.SplitSeq(string(out), "\n") {
		if filepath.Base(strings.TrimSpace(line)) == "go" {
			return true, nil
		}
	}
	return false, nil
}
