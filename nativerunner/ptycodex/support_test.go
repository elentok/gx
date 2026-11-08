// Package ptycodex tests the pty runner against the built fakecodex binary. It
// lives apart from nativerunner so neither package's tests outgrow the size
// guard; the helpers below are the few it shares in spirit with that package.
package ptycodex

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/testutil/agentfake"
)

func TestMain(m *testing.M) {
	agentfake.RegisterHarness()
	if holder.ServeTestBinary() {
		return
	}
	builtGx.build()
	builtFakecodex.build()
	os.Exit(m.Run())
}

// builtBinary builds one Go package once per test run, into a directory of its
// own so the directory can go on PATH. Tests must not modify the result.
type builtBinary struct {
	pkg, name string
	once      sync.Once
	path      string
	err       error
}

var (
	builtGx        = &builtBinary{pkg: "github.com/elentok/gx", name: "gx"}
	builtFakecodex = &builtBinary{pkg: "github.com/elentok/gx/testutil/agentfake/cmd/fakecodex", name: "codex"}
)

func (b *builtBinary) get(t *testing.T) string {
	t.Helper()
	b.build()
	if b.err != nil {
		t.Fatal(b.err)
	}
	return b.path
}

func (b *builtBinary) build() {
	b.once.Do(func() {
		dir, err := os.MkdirTemp("", "ptycodex-"+b.name)
		if err != nil {
			b.err = err
			return
		}
		b.path = filepath.Join(dir, b.name)
		if out, err := exec.Command("go", "build", "-o", b.path, b.pkg).CombinedOutput(); err != nil {
			b.err = &buildError{b.pkg, err, string(out)}
		}
	})
}

type buildError struct {
	pkg string
	err error
	out string
}

func (e *buildError) Error() string { return "build " + e.pkg + ": " + e.err.Error() + "\n" + e.out }

// shortTempDir keeps holder.sock under the unix socket path limit, which a
// t.TempDir path can exceed on macOS.
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "gxh")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// ptyTestEnv is the env a test pty gives its holder: claude's home and no
// config dir, plus the marker entries.
func ptyTestEnv(home string, markers ...string) []string {
	env := []string{"HOME=" + home, "CLAUDE_CONFIG_DIR=", holder.HolderOwnerEnv + "=" + strconv.Itoa(os.Getpid())}
	return append(env, markers...)
}

// ptyOn returns a fresh pty runner over root, as after a server restart.
func ptyOn(root, exe string) *nativerunner.Pty {
	return &nativerunner.Pty{
		Root:         root,
		Holder:       []string{exe},
		Claude:       exe,
		PollInterval: 10 * time.Millisecond,
		StartTimeout: 10 * time.Second,
		// Long: a delivered prompt never waits for it.
		PromptGrace: 10 * time.Second,
		N:           300 * time.Millisecond,
	}
}

func killAgents(root string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(root, e.Name(), nativerunner.MetaFile))
		if err != nil {
			continue
		}
		var meta nativerunner.Meta
		if json.Unmarshal(data, &meta) != nil {
			continue
		}
		for _, pid := range []int{meta.PID, meta.HolderPID} {
			if pid > 0 {
				_ = syscall.Kill(-pid, syscall.SIGKILL)
			}
		}
	}
}
