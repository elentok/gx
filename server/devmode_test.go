package server_test

import (
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

type devRig struct {
	h      *servertest.Harness
	fake   *servertest.FakeVite
	cfg    string
	store  string
	base   string
	cookie *http.Cookie
}

func startDevRig(t *testing.T, checkout string, on bool) *devRig {
	t.Helper()
	store := t.TempDir()
	cfg := filepath.Join(t.TempDir(), "config.json")
	devConfig(t, cfg, store, map[string]any{"listen": "127.0.0.1:0", "dev-dir": checkout, "dev-frontend": on})
	fake := servertest.NewFakeVite()
	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(cfg), servertest.WithViteLauncher(fake),
		func(c *server.Config) {
			c.WebAddr = "127.0.0.1:0"
			// The harness starts from Config.Settings; the file is read on reload.
			c.Settings.WebDevFrontend, c.Settings.WebDevDir = on, checkout
		})
	base := h.Server.WebURL()
	if base == "" {
		t.Fatal("no web listener")
	}
	return &devRig{h: h, fake: fake, cfg: cfg, store: store, base: base, cookie: servertest.Login(t, h.StateDir, base)}
}

func (r *devRig) setKey(t *testing.T, checkout string, on bool) {
	t.Helper()
	devConfig(t, r.cfg, r.store, map[string]any{"listen": "127.0.0.1:0", "dev-dir": checkout, "dev-frontend": on})
	servertest.Reload(t, r.h)
}

func (r *devRig) page(t *testing.T) string {
	t.Helper()
	code, body := servertest.WebDoCookie(t, http.MethodGet, r.base+"/", r.cookie)
	if code != http.StatusOK {
		t.Fatalf("GET / = %d", code)
	}
	return body
}

func (r *devRig) waitPage(t *testing.T, want string) {
	t.Helper()
	servertest.WaitFor(t, 10*time.Second, "the page to become "+want, func() bool { return r.page(t) == want })
}

func (r *devRig) waitStopped(t *testing.T) {
	t.Helper()
	servertest.WaitFor(t, 10*time.Second, "the fake Vite to stop", func() bool { return r.fake.Running() == 0 })
}

func TestDevMode_KeyFlipSwapsTheUIWithoutRestart(t *testing.T) {
	checkout := servertest.WriteDevCheckout(t, true)
	r := startDevRig(t, checkout, false)
	built := r.page(t)
	if built == servertest.ViteStubPage {
		t.Fatal("the built index is the stub")
	}

	r.setKey(t, checkout, true) // returns while the fake is still starting
	if got := r.page(t); got != built {
		t.Fatalf("page while starting = %q, want the built index", got)
	}
	r.fake.Release()
	r.waitPage(t, servertest.ViteStubPage)
	if got := r.h.Server.WebURL(); got != r.base {
		t.Fatalf("web URL = %q, want %q: the listener must not restart", got, r.base)
	}
	if installs, starts := r.fake.Counts(); installs != 0 || starts != 1 {
		t.Fatalf("installs, starts = %d, %d, want 0, 1 for a fresh node_modules", installs, starts)
	}

	r.setKey(t, checkout, false)
	r.waitPage(t, built)
	r.waitStopped(t)
}

func TestDevMode_StaleNodeModulesInstallsFirst(t *testing.T) {
	checkout := servertest.WriteDevCheckout(t, false)
	r := startDevRig(t, checkout, false)
	r.fake.Release()
	r.setKey(t, checkout, true)
	r.waitPage(t, servertest.ViteStubPage)
	if installs, starts := r.fake.Counts(); installs != 1 || starts != 1 {
		t.Fatalf("installs, starts = %d, %d, want 1, 1", installs, starts)
	}
}

func TestDevMode_KeyOnAtStartRunsAndStopsWithTheServer(t *testing.T) {
	checkout := servertest.WriteDevCheckout(t, true)
	r := startDevRig(t, checkout, true)
	r.fake.Release()
	r.waitPage(t, servertest.ViteStubPage)

	if err := r.h.Stop(); err != nil {
		t.Fatal(err)
	}
	if n := r.fake.Running(); n != 0 {
		t.Fatalf("%d fake Vite still running after a graceful stop", n)
	}
}

func TestDevMode_KeyOnWithoutACheckoutStartsNothing(t *testing.T) {
	r := startDevRig(t, filepath.Join(t.TempDir(), "missing"), true)
	r.fake.Release()
	if _, starts := r.fake.Counts(); starts != 0 {
		t.Fatalf("starts = %d, want 0 without a Dev checkout", starts)
	}
}

func TestDevMode_TwoServersGetDifferentProxyPorts(t *testing.T) {
	checkout := servertest.WriteDevCheckout(t, true)
	a := startDevRig(t, checkout, true)
	b := startDevRig(t, checkout, true)
	a.fake.Release()
	b.fake.Release()
	a.waitPage(t, servertest.ViteStubPage)
	b.waitPage(t, servertest.ViteStubPage)
	if pa, pb := a.fake.Ports(), b.fake.Ports(); len(pa) != 1 || len(pb) != 1 || pa[0] == pb[0] {
		t.Fatalf("ports = %v and %v, want one distinct port each", pa, pb)
	}
}
