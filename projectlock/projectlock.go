// Package projectlock is the per-project migration lock. A migration holds it
// exclusive; a ticket file update or a ticket-log append holds it shared, so
// writes wait while their project migrates. Readers take no lock.
//
// It sits below tickets/schema so schema's writers can take it. Lock order is
// this lock first, then schema.LockTicket.
package projectlock

import (
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/elentok/gx/config"
)

// projectFile marks a directory as a ticket-store project.
const projectFile = "project.json"

// waitHookAfter is a var so tests can shorten it.
var waitHookAfter = time.Second

var (
	hookMu   sync.Mutex
	waitHook func(project string)
)

// SetWaitHook registers fn, called once with the project name when a lock wait
// passes one second. It returns a func restoring the previous hook.
func SetWaitHook(fn func(project string)) (restore func()) {
	hookMu.Lock()
	defer hookMu.Unlock()
	old := waitHook
	waitHook = fn
	return func() {
		hookMu.Lock()
		defer hookMu.Unlock()
		waitHook = old
	}
}

// SetWaitHookDelayForTest shortens the wait before the hook fires.
func SetWaitHookDelayForTest(d time.Duration) (restore func()) {
	old := waitHookAfter
	waitHookAfter = d
	return func() { waitHookAfter = old }
}

// ProjectOf returns the project directory of a ticket path laid out as
// <store>/<project>/<epic>/issues/NN-slug.md. A path outside a store project
// (the old .scratch tree) reports false.
func ProjectOf(ticketPath string) (string, bool) {
	abs, err := filepath.Abs(ticketPath)
	if err != nil {
		return "", false
	}
	dir := filepath.Dir(filepath.Dir(filepath.Dir(abs)))
	if filepath.Base(filepath.Dir(abs)) != "issues" {
		return "", false
	}
	if _, err := os.Stat(filepath.Join(dir, projectFile)); err != nil {
		return "", false
	}
	return dir, true
}

// Shared takes the shared lock of the project holding ticketPath and returns
// its unlock func. A path outside a store project takes no lock.
func Shared(ticketPath string) (unlock func(), err error) {
	dir, ok := ProjectOf(ticketPath)
	if !ok {
		return func() {}, nil
	}
	return acquire(dir, syscall.LOCK_SH)
}

// SharedProject takes the shared lock of projectDir itself, for writers that
// work on a project directory rather than one ticket path (epic creation). A
// directory that is not a store project takes no lock.
func SharedProject(projectDir string) (unlock func(), err error) {
	if _, err := os.Stat(filepath.Join(projectDir, projectFile)); err != nil {
		return func() {}, nil
	}
	return acquire(projectDir, syscall.LOCK_SH)
}

// SharedEpic takes the shared lock of the project holding epicDir, laid out as
// <store>/<project>/<epic>. An epic outside a store project takes no lock.
func SharedEpic(epicDir string) (unlock func(), err error) {
	abs, err := filepath.Abs(epicDir)
	if err != nil {
		return func() {}, nil
	}
	return SharedProject(filepath.Dir(abs))
}

// Exclusive takes projectDir's exclusive lock, waiting without a time limit.
func Exclusive(projectDir string) (unlock func(), err error) {
	return acquire(projectDir, syscall.LOCK_EX)
}

// lockPath keeps the lock file out of the project dir: storecommit runs
// `git add -A` over the store, so a file there would be committed. It lives in
// the state dir, not os.TempDir(): a process with another $TMPDIR would open a
// different file, and macOS deletes old temp files under a live lock.
func lockPath(projectDir string) (string, error) {
	stateDir, err := config.EnsuredStateDir()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(projectDir); err == nil {
		projectDir = resolved
	}
	sum := sha1.Sum([]byte(filepath.Clean(projectDir)))
	return filepath.Join(stateDir, "gx-project-"+hex.EncodeToString(sum[:8])+".lock"), nil
}

// acquire takes a flock, so the kernel drops it when the holder dies.
func acquire(projectDir string, how int) (func(), error) {
	path, err := lockPath(projectDir)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	hooked := false
	for {
		err := syscall.Flock(int(f.Fd()), how|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if err != syscall.EWOULDBLOCK {
			f.Close()
			return nil, err
		}
		if !hooked && time.Since(start) >= waitHookAfter {
			hooked = true
			hookMu.Lock()
			fn := waitHook
			hookMu.Unlock()
			if fn != nil {
				fn(filepath.Base(projectDir))
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	return func() { f.Close() }, nil
}
