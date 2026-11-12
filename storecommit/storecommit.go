// Package storecommit commits the ticket store (a git repo) as tickets change:
// debounced after the last change, immediately on done/cancelled/pause.
// The in-process loop runs it, or the server when the orchestrator switch says
// "server". The S2
// read-only server never starts one.
package storecommit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	gitpkg "github.com/elentok/gx/git"
)

// EnsureRepo makes dir a git repo, initializing it if missing. env is the
// environment of every git child; nil means git.NonInteractiveEnv().
func EnsureRepo(dir string, env []string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return nil
	}
	_, err := runGit(dir, env, "init", "-q")
	return err
}

func runGit(dir string, env []string, args ...string) (string, error) {
	if env == nil {
		env = gitpkg.NonInteractiveEnv()
	}
	stdout, _, err := gitpkg.Run("", env, append([]string{"-C", dir,
		"-c", "user.name=gx", "-c", "user.email=gx@localhost", "-c", "commit.gpgsign=false"}, args...)...)
	if err != nil {
		return "", err
	}
	return stdout, nil
}

func (l *Loop) git(args ...string) (string, error) { return runGit(l.dir, l.env, args...) }

// countDirty returns how many files differ from HEAD and a signature of them
// that changes on every edit: the status text alone would not, since a file
// already shown as modified stays " M" however often it is rewritten.
func (l *Loop) countDirty() (int, string, error) {
	dir := l.dir
	out, err := l.git("status", "--porcelain", "-uall")
	if err != nil {
		return 0, "", err
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return 0, "", nil
	}
	var sig strings.Builder
	for _, line := range lines {
		sig.WriteString(line)
		if len(line) > 3 {
			if fi, err := os.Stat(filepath.Join(dir, line[3:])); err == nil {
				fmt.Fprintf(&sig, "@%d/%d", fi.ModTime().UnixNano(), fi.Size())
			}
		}
		sig.WriteByte('\n')
	}
	return len(lines), sig.String(), nil
}

// Loop is a running store commit loop.
type Loop struct {
	dir  string
	env  []string   // environment of every git child; nil means git.NonInteractiveEnv()
	refs int        // guarded by regMu
	mu   sync.Mutex // serializes commits
	stop chan struct{}
	done chan struct{}
	kick chan struct{} // Touch signals here; coalesces
	// fallback is how often the loop checks for changes nobody reported.
	fallback time.Duration

	// settingsMu guards the fields SetSettings changes while the loop runs.
	settingsMu sync.Mutex
	debounce   time.Duration
	// pushRemote is empty when pushing is off. Pushes run on their own
	// goroutine so a slow or dead remote never delays a commit.
	pushRemote string

	onError  func(error)
	pushKick chan struct{}
	pushStop chan struct{}
	pushDone chan struct{}
}

// Options configures StartWith.
type Options struct {
	Debounce time.Duration
	// PushRemote, when set, is pushed to (best effort) after each commit.
	PushRemote string
	// OnError receives push failures; they never block or fail a commit.
	OnError func(error)
	// Env is the environment of every git child. Nil means git.NonInteractiveEnv().
	Env []string
}

var (
	regMu sync.Mutex
	reg   = map[string]*Loop{}
)

// Start makes dir a git repo and starts the debounced commit loop on it. The
// loop is also registered so Immediate can find it from any path inside dir.
func Start(dir string, debounce time.Duration) (*Loop, error) {
	return StartWith(dir, Options{Debounce: debounce})
}

// StartWith is Start with a push remote. When a loop already runs for dir its
// options are kept.
func StartWith(dir string, o Options) (*Loop, error) {
	return startWith(dir, o, fallbackPoll)
}

// fallbackPoll is the safety net for a change nobody reported through Touch.
// It only checks; the commit still waits for a quiet debounce.
const fallbackPoll = 60 * time.Second

func start(dir string, debounce, poll time.Duration) (*Loop, error) {
	return startWith(dir, Options{Debounce: debounce}, poll)
}

func startWith(dir string, o Options, fallback time.Duration) (*Loop, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := EnsureRepo(dir, o.Env); err != nil {
		return nil, err
	}
	regMu.Lock()
	defer regMu.Unlock()
	// Concurrent epic runs share one loop per store: two committers would race
	// on git's index lock.
	if l, ok := reg[dir]; ok {
		l.refs++
		return l, nil
	}
	l := &Loop{dir: dir, env: o.Env, debounce: o.Debounce, fallback: fallback, kick: make(chan struct{}, 1), refs: 1, stop: make(chan struct{}), done: make(chan struct{}),
		pushRemote: o.PushRemote, onError: o.OnError, pushKick: make(chan struct{}, 1), pushStop: make(chan struct{}), pushDone: make(chan struct{})}
	reg[dir] = l
	go l.run()
	go l.runPush()
	return l, nil
}

// Stop releases one Start; the last one ends the loop, flushing any pending
// change as a final sync commit.
func (l *Loop) Stop() {
	regMu.Lock()
	l.refs--
	if l.refs > 0 {
		regMu.Unlock()
		return
	}
	delete(reg, l.dir)
	regMu.Unlock()
	close(l.stop)
	<-l.done
	_ = l.commit("")
	close(l.pushStop)
	<-l.pushDone
}

// SetSettings changes the debounce and push remote of the running loop, for a
// config reload. The loop keeps running, so Immediate commits are never lost
// to a restart; the new debounce applies from the next change.
func (l *Loop) SetSettings(debounce time.Duration, pushRemote string) {
	l.settingsMu.Lock()
	defer l.settingsMu.Unlock()
	l.debounce, l.pushRemote = debounce, pushRemote
}

func (l *Loop) settings() (debounce time.Duration, pushRemote string) {
	l.settingsMu.Lock()
	defer l.settingsMu.Unlock()
	return l.debounce, l.pushRemote
}

// Touch tells the loop the store changed. Each touch restarts the quiet
// period; no git runs until it ends.
func (l *Loop) Touch() {
	select {
	case l.kick <- struct{}{}:
	default:
	}
}

// Flush commits any pending change now. A server calls it on start to commit
// edits made while it was down.
func (l *Loop) Flush() error { return l.commit("") }

// runPush pushes once per burst of commits; kicks arriving mid-push coalesce.
// After Stop it pushes once more if a kick is pending, then exits.
func (l *Loop) runPush() {
	defer close(l.pushDone)
	push := func() {
		_, remote := l.settings()
		if remote == "" {
			return
		}
		if _, err := l.git("push", remote, "HEAD"); err != nil && l.onError != nil {
			l.onError(err)
		}
	}
	for {
		select {
		case <-l.pushKick:
			push()
		case <-l.pushStop:
			select {
			case <-l.pushKick:
				push()
			default:
			}
			return
		}
	}
}

func (l *Loop) run() {
	defer close(l.done)
	var lastSig string
	var quiet <-chan time.Time // nil while nothing is pending
	restart := func() {
		debounce, _ := l.settings()
		quiet = time.After(debounce)
	}
	fallback := time.NewTicker(l.fallback)
	defer fallback.Stop()
	for {
		select {
		case <-l.stop:
			return
		case <-l.kick:
			restart()
		case <-fallback.C:
			n, sig, err := l.countDirty()
			switch {
			case err != nil || n == 0:
				lastSig, quiet = "", nil
			case sig != lastSig:
				lastSig = sig
				restart()
			}
		case <-quiet:
			quiet, lastSig = nil, ""
			_ = l.commit("")
		}
	}
}

// commit stages everything and commits it. An empty msg means a sync commit.
func (l *Loop) commit(msg string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	n, _, err := l.countDirty()
	if err != nil || n == 0 {
		return err
	}
	if msg == "" {
		msg = fmt.Sprintf("sync: %d files", n)
	}
	if _, err := l.git("add", "-A"); err != nil {
		return err
	}
	if _, err = l.git("commit", "-q", "-m", msg); err != nil {
		return err
	}
	select {
	case l.pushKick <- struct{}{}:
	default:
	}
	return nil
}

// Immediate commits now with msg if a loop is running for the store holding
// path; otherwise it does nothing (S2 server, CLI, tests).
func Immediate(path, msg string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	regMu.Lock()
	var found *Loop
	for dir, l := range reg {
		if strings.HasPrefix(abs, dir+string(filepath.Separator)) {
			found = l
			break
		}
	}
	regMu.Unlock()
	if found == nil {
		return nil
	}
	return found.commit(msg)
}
