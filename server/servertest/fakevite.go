package servertest

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/devvite"
)

// ViteStubPage is what the fake Vite serves on every path.
const ViteStubPage = "vite-stub"

// FakeVite is a scripted devvite.Launcher: Install and Start touch no pnpm,
// and Start serves ViteStubPage on the claimed port. It holds readiness until
// Release, so a test can observe the starting state.
type FakeVite struct {
	mu       sync.Mutex
	installs int
	starts   int
	running  int
	ports    []int
	ready    chan struct{}
	once     sync.Once
}

// NewFakeVite returns a fake that is not ready yet.
func NewFakeVite() *FakeVite { return &FakeVite{ready: make(chan struct{})} }

// WithViteLauncher makes the server's Dev mode use l instead of pnpm.
func WithViteLauncher(l devvite.Launcher) func(*server.Config) {
	return func(c *server.Config) { c.ViteLauncher = l }
}

// Release lets the fake answer its readiness probe.
func (f *FakeVite) Release() { f.once.Do(func() { close(f.ready) }) }

// Counts returns how many installs and starts happened.
func (f *FakeVite) Counts() (installs, starts int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.installs, f.starts
}

// Running is how many started fakes have not been stopped.
func (f *FakeVite) Running() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.running
}

// Ports lists every port Start was given.
func (f *FakeVite) Ports() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int(nil), f.ports...)
}

func (f *FakeVite) FreePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

func (f *FakeVite) Install(_ context.Context, webuiDir string, _ io.Writer) error {
	f.mu.Lock()
	f.installs++
	f.mu.Unlock()
	return os.MkdirAll(filepath.Join(webuiDir, "node_modules"), 0o755)
}

func (f *FakeVite) Start(_ context.Context, _ string, port int, _ io.Writer) (devvite.Process, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return nil, err
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, ViteStubPage)
	})}
	p := &fakeViteProc{f: f, srv: srv, done: make(chan struct{})}
	go func() { _ = srv.Serve(ln); close(p.done) }()
	f.mu.Lock()
	f.starts++
	f.running++
	f.ports = append(f.ports, port)
	f.mu.Unlock()
	return p, nil
}

func (f *FakeVite) Probe(ctx context.Context, _ int) error {
	select {
	case <-f.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return context.DeadlineExceeded
	}
}

type fakeViteProc struct {
	f    *FakeVite
	srv  *http.Server
	done chan struct{}
	once sync.Once
}

func (p *fakeViteProc) Wait() error { <-p.done; return nil }

func (p *fakeViteProc) Stop() {
	p.once.Do(func() {
		_ = p.srv.Close()
		p.f.mu.Lock()
		p.f.running--
		p.f.mu.Unlock()
	})
}

// WriteDevCheckout makes a Dev checkout under t.TempDir() that has
// webui/package.json, and returns its root. A node_modules folder is created
// when fresh is true, so the supervisor needs no install.
func WriteDevCheckout(t *testing.T, fresh bool) string {
	t.Helper()
	root := t.TempDir()
	webui := filepath.Join(root, "webui")
	if err := os.MkdirAll(webui, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webui, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if fresh {
		if err := os.MkdirAll(filepath.Join(webui, "node_modules"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}
