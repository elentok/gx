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
	mu        sync.Mutex
	installs  int
	starts    int
	running   int
	ports     []int
	dirs      []string
	procs     []*fakeViteProc
	noModules bool
	installE  error
	startE    error
	ready     chan struct{}
	once      sync.Once

	installGate chan struct{}
}

// FailInstall makes every Install return err (nil clears it).
func (f *FakeVite) FailInstall(err error) { f.mu.Lock(); f.installE = err; f.mu.Unlock() }

// FailStart makes every Start return err (nil clears it).
func (f *FakeVite) FailStart(err error) { f.mu.Lock(); f.startE = err; f.mu.Unlock() }

// NoNodeModules makes Install succeed without creating node_modules.
func (f *FakeVite) NoNodeModules() { f.mu.Lock(); f.noModules = true; f.mu.Unlock() }

// Crash ends every running fake the way a Vite crash would: Wait returns
// without anyone calling Stop.
func (f *FakeVite) Crash() {
	f.mu.Lock()
	procs := append([]*fakeViteProc(nil), f.procs...)
	f.mu.Unlock()
	for _, p := range procs {
		p.Stop()
	}
}

// Log makes every running fake print a line, as Vite does while it runs.
func (f *FakeVite) Log(line string) {
	f.mu.Lock()
	procs := append([]*fakeViteProc(nil), f.procs...)
	f.mu.Unlock()
	for _, p := range procs {
		_, _ = io.WriteString(p.out, line+"\n")
	}
}

// NewFakeVite returns a fake that is not ready yet.
func NewFakeVite() *FakeVite { return &FakeVite{ready: make(chan struct{})} }

// WithViteLauncher makes the server's Dev mode use l instead of pnpm.
func WithViteLauncher(l devvite.Launcher) func(*server.Config) {
	return func(c *server.Config) { c.ViteLauncher = l }
}

// Release lets the fake answer its readiness probe.
func (f *FakeVite) Release() {
	f.mu.Lock()
	ready, once := f.ready, &f.once
	f.mu.Unlock()
	once.Do(func() { close(ready) })
}

// Hold makes the fakes started from now on not ready again, after a Release.
func (f *FakeVite) Hold() {
	f.mu.Lock()
	f.ready, f.once = make(chan struct{}), sync.Once{}
	f.mu.Unlock()
}

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

// Dirs lists the webui directory every Start was given.
func (f *FakeVite) Dirs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.dirs...)
}

func (f *FakeVite) FreePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

// HoldInstall makes Install wait until the returned release func is called, so
// a test can observe the installing state.
func (f *FakeVite) HoldInstall() (release func()) {
	gate := make(chan struct{})
	f.mu.Lock()
	f.installGate = gate
	f.mu.Unlock()
	var once sync.Once
	return func() { once.Do(func() { close(gate) }) }
}

func (f *FakeVite) Install(ctx context.Context, webuiDir string, out io.Writer) error {
	f.mu.Lock()
	f.installs++
	err, skip, gate := f.installE, f.noModules, f.installGate
	f.mu.Unlock()
	_, _ = io.WriteString(out, "fake install output\n")
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err != nil || skip {
		return err
	}
	return os.MkdirAll(filepath.Join(webuiDir, "node_modules"), 0o755)
}

func (f *FakeVite) Start(_ context.Context, webuiDir string, port int, out io.Writer) (devvite.Process, error) {
	f.mu.Lock()
	failure := f.startE
	f.mu.Unlock()
	if failure != nil {
		return nil, failure
	}
	_, _ = io.WriteString(out, "fake vite output\n")
	ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return nil, err
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, ViteStubPage)
	})}
	p := &fakeViteProc{f: f, srv: srv, out: out, done: make(chan struct{})}
	go func() { _ = srv.Serve(ln); close(p.done) }()
	f.mu.Lock()
	f.starts++
	f.running++
	f.ports = append(f.ports, port)
	f.dirs = append(f.dirs, webuiDir)
	f.procs = append(f.procs, p)
	f.mu.Unlock()
	return p, nil
}

func (f *FakeVite) Probe(ctx context.Context, _ int) error {
	f.mu.Lock()
	ready := f.ready
	f.mu.Unlock()
	select {
	case <-ready:
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
	out  io.Writer
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
