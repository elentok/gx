package deps

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// installFlights serializes installs that share a key. Launches that start
// together each ran the same frozen install on a cold package-manager store;
// the first one now warms the store and the rest run their own install on it.
var installFlights = &flights{held: map[string]*flight{}}

type flights struct {
	mu   sync.Mutex
	held map[string]*flight
}

type flight struct {
	token chan struct{} // holds one value while nobody is installing
	users int
}

// acquire waits until no other install holds key, or ctx ends. A failed first
// install needs no special hand-over: its release lets the next waiter go.
func (f *flights) acquire(ctx context.Context, key string) (release func(), err error) {
	f.mu.Lock()
	fl := f.held[key]
	if fl == nil {
		fl = &flight{token: make(chan struct{}, 1)}
		fl.token <- struct{}{}
		f.held[key] = fl
	}
	fl.users++
	f.mu.Unlock()

	done := func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		if fl.users--; fl.users == 0 {
			delete(f.held, key)
		}
	}
	select {
	case <-fl.token:
		return func() { fl.token <- struct{}{}; done() }, nil
	case <-ctx.Done():
		done()
		return nil, ctx.Err()
	}
}

// installKey identifies installs that would fill the store with the same
// packages: the directory within its worktree, the files the manager reads, the
// manager's pinned version and the node version. Unpinned manager versions are
// not probed (a probe is a process per install); node's is, once per pathEnv.
func installKey(ctx context.Context, worktree, dir, packageManager, pathEnv string) string {
	h := sha256.New()
	rel, err := filepath.Rel(worktree, dir)
	if err != nil {
		rel = dir
	}
	h.Write([]byte(rel + "\x00" + packageManager + "\x00" + nodeVersion(ctx, pathEnv)))
	files := []string{"package.json", "pnpm-workspace.yaml", ".npmrc"}
	for name := range lockfileManagers {
		files = append(files, name)
	}
	sort.Strings(files)
	for _, name := range files {
		raw, _ := os.ReadFile(filepath.Join(dir, name))
		h.Write([]byte("\x00" + name + "\x00"))
		h.Write(raw)
	}
	return hex.EncodeToString(h.Sum(nil))
}

var nodeVersions sync.Map // pathEnv -> version

func nodeVersion(ctx context.Context, pathEnv string) string {
	if v, ok := nodeVersions.Load(pathEnv); ok {
		return v.(string)
	}
	name := "node"
	if pathEnv != "" {
		var err error
		if name, err = lookPathIn(name, pathEnv); err != nil {
			return ""
		}
	}
	out, err := exec.CommandContext(ctx, name, "--version").Output()
	if err != nil {
		return ""
	}
	v := strings.TrimSpace(string(out))
	nodeVersions.Store(pathEnv, v)
	return v
}
