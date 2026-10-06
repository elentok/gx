// Package storecommit commits the ticket store (a git repo) as tickets change:
// debounced after the last change, immediately on done/cancelled/park.
// S1 runs it in the in-process loop; S3 moves it into the server. The S2
// read-only server never starts one.
package storecommit

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// EnsureRepo makes dir a git repo, initializing it if missing.
func EnsureRepo(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return nil
	}
	_, err := git(dir, "init", "-q")
	return err
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir,
		"-c", "user.name=gx", "-c", "user.email=gx@localhost", "-c", "commit.gpgsign=false"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, out)
	}
	return string(out), nil
}

// countDirty returns how many files differ from HEAD and a signature of them
// that changes on every edit: the status text alone would not, since a file
// already shown as modified stays " M" however often it is rewritten.
func countDirty(dir string) (int, string, error) {
	out, err := git(dir, "status", "--porcelain", "-uall")
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
	dir      string
	debounce time.Duration
	poll     time.Duration
	refs     int        // guarded by regMu
	mu       sync.Mutex // serializes commits
	stop     chan struct{}
	done     chan struct{}
}

var (
	regMu sync.Mutex
	reg   = map[string]*Loop{}
)

// Start makes dir a git repo and starts the debounced commit loop on it. The
// loop is also registered so Immediate can find it from any path inside dir.
func Start(dir string, debounce time.Duration) (*Loop, error) {
	return start(dir, debounce, pollFor(debounce))
}

func pollFor(debounce time.Duration) time.Duration {
	return min(max(debounce/4, 10*time.Millisecond), time.Second)
}

func start(dir string, debounce, poll time.Duration) (*Loop, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := EnsureRepo(dir); err != nil {
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
	l := &Loop{dir: dir, debounce: debounce, poll: poll, refs: 1, stop: make(chan struct{}), done: make(chan struct{})}
	reg[dir] = l
	go l.run()
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
}

func (l *Loop) run() {
	defer close(l.done)
	var lastSig string
	var quietSince time.Time
	t := time.NewTicker(l.poll)
	defer t.Stop()
	for {
		select {
		case <-l.stop:
			return
		case <-t.C:
		}
		n, sig, err := countDirty(l.dir)
		if err != nil || n == 0 {
			lastSig = ""
			continue
		}
		now := time.Now()
		if sig != lastSig {
			lastSig, quietSince = sig, now
			continue
		}
		if now.Sub(quietSince) >= l.debounce {
			_ = l.commit("")
			lastSig = ""
		}
	}
}

// commit stages everything and commits it. An empty msg means a sync commit.
func (l *Loop) commit(msg string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	n, _, err := countDirty(l.dir)
	if err != nil || n == 0 {
		return err
	}
	if msg == "" {
		msg = fmt.Sprintf("sync: %d files", n)
	}
	if _, err := git(l.dir, "add", "-A"); err != nil {
		return err
	}
	_, err = git(l.dir, "commit", "-q", "-m", msg)
	return err
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
