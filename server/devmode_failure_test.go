package server_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/devvite"
	"github.com/elentok/gx/server/devvite/vitetest"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

func (r *devRig) restart(t *testing.T) wire.DevRestartResult {
	t.Helper()
	code, body := servertest.WebPostJSON(t, r.base+"/v1/dev/restart", "{}", r.cookie, "", servertest.JSONPost(r.base))
	if code != http.StatusOK {
		t.Fatalf("POST /v1/dev/restart = %d %s", code, body)
	}
	var res wire.DevRestartResult
	if err := json.Unmarshal([]byte(body), &res); err != nil {
		t.Fatal(err)
	}
	return res
}

func (r *devRig) waitDown(t *testing.T, reason string) wire.DevStatus {
	t.Helper()
	var dev wire.DevStatus
	servertest.WaitFor(t, 10*time.Second, "Dev mode to be down", func() bool {
		dev = devSnapshot(t, r.h)
		return dev.State == wire.DevStateDown
	})
	if !strings.Contains(dev.Reason, reason) {
		t.Fatalf("reason = %q, want it to contain %q", dev.Reason, reason)
	}
	return dev
}

func TestDevMode_FailureCausesServeTheBuiltFilesWithAReason(t *testing.T) {
	for _, tc := range []struct {
		name   string
		fresh  bool
		script func(*vitetest.FakeVite)
		reason string
	}{
		{"start fails", true, func(f *vitetest.FakeVite) { f.FailStart(errors.New("boom")) }, "boom"},
		{"install fails", false, func(f *vitetest.FakeVite) { f.FailInstall(errors.New("exit status 1")) }, "exit status 1"},
		{"pnpm missing at start", true, func(f *vitetest.FakeVite) { f.FailStart(devvite.ErrPnpmNotFound) }, "pnpm"},
		{"pnpm missing at install", false, func(f *vitetest.FakeVite) { f.FailInstall(devvite.ErrPnpmNotFound) }, "pnpm"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkout := vitetest.WriteDevCheckout(t, tc.fresh)
			r := startDevRig(t, checkout, false)
			built := r.page(t)
			tc.script(r.fake)
			r.fake.Release()
			r.setKey(t, checkout, true)
			dev := r.waitDown(t, tc.reason)
			if got := r.page(t); got != built {
				t.Fatalf("page = %q, want the built index", got)
			}
			if !tc.fresh && !strings.Contains(strings.Join(dev.LogTail, "\n"), "fake install output") {
				t.Fatalf("dev = %+v, want the install output in the log tail", dev)
			}
		})
	}
}

func TestDevMode_InstallThatLeavesNoNodeModulesIsDown(t *testing.T) {
	checkout := vitetest.WriteDevCheckout(t, false)
	r := startDevRig(t, checkout, false)
	r.fake.NoNodeModules()
	r.fake.Release()
	r.setKey(t, checkout, true)
	r.waitDown(t, "node_modules")
	if _, starts := r.fake.Counts(); starts != 0 {
		t.Fatalf("starts = %d, want 0", starts)
	}
}

func TestDevMode_CrashServesBuiltFilesAndRestartsOnlyOnRequest(t *testing.T) {
	checkout := vitetest.WriteDevCheckout(t, true)
	r := startDevRig(t, checkout, false)
	built := r.page(t)
	r.fake.Release()
	if res := r.restart(t); !res.Refused || res.Reason != wire.ReasonDevNotDown {
		t.Fatalf("restart while off = %+v, want a refusal", res)
	}
	r.setKey(t, checkout, true)
	r.waitPage(t, vitetest.ViteStubPage)
	if res := r.restart(t); !res.Refused {
		t.Fatalf("restart while running = %+v, want a refusal", res)
	}

	r.fake.Crash()
	r.waitPage(t, built)
	r.waitDown(t, "exited")
	time.Sleep(200 * time.Millisecond) // a wrongly automatic restart would have happened by now
	if _, starts := r.fake.Counts(); starts != 1 {
		t.Fatalf("starts = %d after a crash, want 1: no automatic restart", starts)
	}

	if res := r.restart(t); res.Refused {
		t.Fatalf("restart while down = %+v", res)
	}
	r.waitPage(t, vitetest.ViteStubPage)
	if _, starts := r.fake.Counts(); starts != 2 {
		t.Fatalf("starts = %d after Restart Vite, want 2", starts)
	}

	// Turning the key off and on retries too.
	r.fake.Crash()
	r.waitDown(t, "exited")
	r.setKey(t, checkout, false)
	r.setKey(t, checkout, true)
	r.waitPage(t, vitetest.ViteStubPage)
	if _, starts := r.fake.Counts(); starts != 3 {
		t.Fatalf("starts = %d after flipping the key, want 3", starts)
	}
}

func TestDevMode_ViteOutputReachesALogFile(t *testing.T) {
	checkout := vitetest.WriteDevCheckout(t, true)
	r := startDevRig(t, checkout, true)
	r.fake.Release()
	r.waitPage(t, vitetest.ViteStubPage)
	data, err := os.ReadFile(filepath.Join(r.h.StateDir, "logs", "vite", "dev-vite.log"))
	if err != nil || !strings.Contains(string(data), "fake vite output") {
		t.Fatalf("log = %q, %v, want the fake's output", data, err)
	}
}
