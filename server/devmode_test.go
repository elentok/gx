package server_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

type devRig struct {
	h      *servertest.Harness
	fake   *servertest.FakeVite
	cfg    string
	store  string
	base   string
	cookie *http.Cookie
}

func startDevRig(t *testing.T, checkout string, on bool, opts ...func(*server.Config)) *devRig {
	t.Helper()
	store := t.TempDir()
	cfg := filepath.Join(t.TempDir(), "config.json")
	devConfig(t, cfg, store, map[string]any{"listen": "127.0.0.1:0", "dev-dir": checkout, "dev-frontend": on})
	fake := servertest.NewFakeVite()
	opts = append([]func(*server.Config){servertest.WithConfigFile(cfg), servertest.WithViteLauncher(fake),
		func(c *server.Config) {
			c.WebAddr = "127.0.0.1:0"
			// The harness starts from Config.Settings; the file is read on reload.
			c.Settings.WebDevFrontend, c.Settings.WebDevDir = on, checkout
		}}, opts...)
	h := servertest.StartWithStore(t, store, opts...)
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

func TestDevMode_SettingsWriteStartsViteAndSurvivesARestart(t *testing.T) {
	checkout := servertest.WriteDevCheckout(t, true)
	r := startDevRig(t, checkout, false)
	r.fake.Release()
	res, err := r.h.Client.Admin.SetSettings(context.Background(), wire.SettingsWriteRequest{
		Changes: []wire.SettingChange{{Key: "server.web.dev-frontend", Value: json.RawMessage("true")}},
	})
	if err != nil || res.Refused {
		t.Fatalf("write = %+v, %v", res, err)
	}
	r.waitPage(t, servertest.ViteStubPage)

	data, err := os.ReadFile(r.cfg)
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Server struct {
			Web struct {
				DevFrontend bool `json:"dev-frontend"`
			} `json:"web"`
		} `json:"server"`
	}
	if err := json.Unmarshal(data, &file); err != nil || !file.Server.Web.DevFrontend {
		t.Fatalf("config.json = %s (err %v), want server.web.dev-frontend true", data, err)
	}

	// A restart starts from the saved key: a new server over the same file.
	fake := servertest.NewFakeVite()
	fake.Release()
	servertest.StartWithStore(t, r.store, servertest.WithViteLauncher(fake), func(c *server.Config) {
		c.WebAddr = "127.0.0.1:0"
		c.Settings.WebDevFrontend, c.Settings.WebDevDir = file.Server.Web.DevFrontend, checkout
	})
	servertest.WaitFor(t, 10*time.Second, "the restarted server to start Vite", func() bool { return fake.Running() == 1 })
}

func TestDevMode_HandEditAndReloadBothStartVite(t *testing.T) {
	checkout := servertest.WriteDevCheckout(t, true)
	r := startDevRig(t, checkout, false, func(c *server.Config) {
		c.WatchConfig = true
		c.ConfigWatchDebounce = 20 * time.Millisecond
	})
	r.fake.Release()

	// The watcher picks the hand edit up on its own.
	servertest.WriteConfig(t, r.cfg, r.store, map[string]any{"server": map[string]any{
		"watch-config": true,
		"web":          map[string]any{"listen": "127.0.0.1:0", "dev-dir": checkout, "dev-frontend": true},
	}})
	r.waitPage(t, servertest.ViteStubPage)

	// The reload command (watcher off in the file) stops and starts it the same way.
	r.setKey(t, checkout, false)
	r.waitStopped(t)
	r.setKey(t, checkout, true)
	r.waitPage(t, servertest.ViteStubPage)
}

func TestDevMode_DevDirChangeRestartsFromTheNewCheckoutOrFallsBack(t *testing.T) {
	first := servertest.WriteDevCheckout(t, true)
	second := servertest.WriteDevCheckout(t, true)
	r := startDevRig(t, first, true)
	r.fake.Release()
	r.waitPage(t, servertest.ViteStubPage)

	r.setKey(t, second, true)
	servertest.WaitFor(t, 10*time.Second, "Vite to restart from the new checkout", func() bool {
		_, starts := r.fake.Counts()
		return starts == 2 && r.fake.Running() == 1
	})
	dirs := r.fake.Dirs()
	if want := filepath.Join(second, "webui"); dirs[1] != want {
		t.Fatalf("second start dir = %q, want %q", dirs[1], want)
	}

	r.setKey(t, filepath.Join(t.TempDir(), "missing"), true)
	r.waitStopped(t)
	built := ""
	servertest.WaitFor(t, 10*time.Second, "the built files", func() bool {
		built = r.page(t)
		return built != servertest.ViteStubPage
	})
	if dev := devSnapshot(t, r.h); dev.HasCheckout || dev.State != wire.DevStateOff {
		t.Fatalf("dev = %+v, want off without a checkout", dev)
	}

	r.setKey(t, first, true)
	if dev := devSnapshot(t, r.h); !dev.HasCheckout {
		t.Fatalf("dev = %+v, want a checkout", dev)
	}
	r.waitPage(t, servertest.ViteStubPage)
}

func TestDevMode_DevDirChangeServesBuiltFilesUntilTheNewViteIsReady(t *testing.T) {
	first := servertest.WriteDevCheckout(t, true)
	second := servertest.WriteDevCheckout(t, true)
	r := startDevRig(t, first, true)
	r.fake.Release()
	r.waitPage(t, servertest.ViteStubPage)
	r.fake.Hold()

	r.setKey(t, second, true)
	if got := r.page(t); got == servertest.ViteStubPage {
		t.Fatal("page right after the dev-dir change is the old Vite, want the built files")
	}
	servertest.WaitFor(t, 10*time.Second, "the new Vite to start", func() bool {
		_, starts := r.fake.Counts()
		return starts == 2
	})
	if got := r.page(t); got == servertest.ViteStubPage {
		t.Fatal("page while the new Vite starts is the stub, want the built files")
	}
	r.fake.Release()
	r.waitPage(t, servertest.ViteStubPage)
}

func TestSnapshotDev_InstallingAndStartingCarryTheirDetails(t *testing.T) {
	checkout := servertest.WriteDevCheckout(t, false)
	r := startDevRig(t, checkout, false)
	release := r.fake.HoldInstall()
	defer release()

	r.setKey(t, checkout, true)
	var dev wire.DevStatus
	servertest.WaitFor(t, 10*time.Second, "the installing state", func() bool {
		dev = devSnapshot(t, r.h)
		return dev.State == wire.DevStateInstalling
	})
	if dev.Checkout != checkout || !dev.HasCheckout || len(dev.LogTail) == 0 || dev.Reason == "" {
		t.Fatalf("installing dev = %+v, want checkout, reason and log lines", dev)
	}

	release()
	servertest.WaitFor(t, 10*time.Second, "the starting state", func() bool {
		dev = devSnapshot(t, r.h)
		return dev.State == wire.DevStateStarting
	})
	if dev.Checkout != checkout || !dev.HasCheckout || len(dev.LogTail) == 0 || dev.Reason == "" {
		t.Fatalf("starting dev = %+v, want checkout, reason and log lines", dev)
	}
}

func TestDevMode_NoWebListenerNeverStartsVite(t *testing.T) {
	checkout := servertest.WriteDevCheckout(t, true)
	store := t.TempDir()
	cfg := filepath.Join(t.TempDir(), "config.json")
	devConfig(t, cfg, store, map[string]any{"dev-dir": checkout, "dev-frontend": true})
	fake := servertest.NewFakeVite()
	fake.Release()
	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(cfg), servertest.WithViteLauncher(fake),
		func(c *server.Config) { c.Settings.WebDevFrontend, c.Settings.WebDevDir = true, checkout })
	servertest.Reload(t, h)
	if _, starts := fake.Counts(); starts != 0 {
		t.Fatalf("starts = %d, want 0 without a web listener", starts)
	}
	dev := devSnapshot(t, h)
	if dev.State != wire.DevStateOff || !dev.HasCheckout || !strings.Contains(dev.Reason, "listener") {
		t.Fatalf("dev = %+v, want off with a listener reason", dev)
	}
}

func TestDevMode_WebListenerThatFailedToBindNeverStartsVite(t *testing.T) {
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()
	checkout := servertest.WriteDevCheckout(t, true)
	fake := servertest.NewFakeVite()
	fake.Release()
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithViteLauncher(fake), func(c *server.Config) {
		c.WebAddr = taken.Addr().String()
		c.Settings.WebDevFrontend, c.Settings.WebDevDir = true, checkout
	})
	if _, starts := fake.Counts(); starts != 0 {
		t.Fatalf("starts = %d, want 0 when the web listener failed to bind", starts)
	}
	dev := devSnapshot(t, h)
	if dev.State != wire.DevStateOff || dev.Reason == "" || strings.Contains(dev.Reason, "listener is off") {
		t.Fatalf("dev = %+v, want off with the bind failure as the reason", dev)
	}
}

func TestDevMode_ModulesNeedNoCookieOverHTTPS(t *testing.T) {
	checkout := servertest.WriteDevCheckout(t, true)
	r := startDevRig(t, checkout, true)
	r.fake.Release()
	r.waitPage(t, servertest.ViteStubPage)

	if code, body := servertest.WebDoCookie(t, http.MethodGet, r.base+"/src/main.tsx", nil); code != http.StatusOK || body != servertest.ViteStubPage {
		t.Errorf("module without a cookie = %d %q, want the Vite response", code, body)
	}
	if code, _ := servertest.WebDoCookie(t, http.MethodGet, r.base+"/v1/snapshot", nil); code != http.StatusUnauthorized {
		t.Errorf("API without a cookie = %d, want 401", code)
	}
	if code, body := servertest.WebDoCookie(t, http.MethodGet, r.base+"/v1/snapshot", r.cookie); code != http.StatusOK || body == servertest.ViteStubPage {
		t.Errorf("API with a cookie = %d, want the local snapshot", code)
	}
}

func TestDevMode_SW_SelfRemovingWorkerOnlyWhileViteRuns(t *testing.T) {
	checkout := servertest.WriteDevCheckout(t, true)
	r := startDevRig(t, checkout, false)
	sw := func() string {
		_, body := servertest.WebDoCookie(t, http.MethodGet, r.base+"/sw.js", r.cookie)
		return body
	}
	off := sw()
	if strings.Contains(off, "unregister") {
		t.Fatal("the worker with Dev mode off is the self-removing one")
	}

	r.setKey(t, checkout, true)
	r.fake.Release()
	r.waitPage(t, servertest.ViteStubPage)
	on := sw()
	if !strings.Contains(on, "gx-shell-") || !strings.Contains(on, "unregister") {
		t.Fatalf("worker while Vite runs = %q, want the self-removing worker", on)
	}

	r.fake.Crash()
	r.waitDown(t, "exited")
	if got := sw(); got != off {
		t.Fatalf("worker while Vite is down = %q, want the built worker %q", got, off)
	}
}
