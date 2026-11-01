package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/migratefail"
)

// migrating is the handshake's view of the start-up migration.
func (r *running) migrating(t *testing.T) bool {
	t.Helper()
	var st struct {
		Running   bool `json:"running"`
		Migrating bool `json:"migrating"`
	}
	if err := json.Unmarshal([]byte(r.gx(t, "server", "status", "--json")), &st); err != nil {
		t.Fatal(err)
	}
	if !st.Running {
		t.Fatal("server is not running")
	}
	return st.Migrating
}

// up reports whether a server answers; it never fails the test.
func (r *running) up() bool {
	cmd := exec.Command(gxBin, "server", "status", "--json")
	cmd.Env = r.Env
	out, err := cmd.Output()
	var st struct {
		Running bool `json:"running"`
	}
	return err == nil && json.Unmarshal(out, &st) == nil && st.Running
}

func (r *running) waitMigrated(t *testing.T) {
	t.Helper()
	waitUntil(t, "the migration to end", func() bool { return !r.migrating(t) })
}

func waitUntil(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if ok() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

func (r *running) failures(t *testing.T) map[string]string {
	t.Helper()
	got, err := migratefail.Read(r.StateDir, []string{"demo", "broken"})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range got {
		out[f.Project] = f.Error
	}
	return out
}

// wantMigratedFile checks the old-shape ticket in demo/<epic> was rewritten. It
// reads the file, not the snapshot: the index is not refreshed after the
// migration rewrites a file.
func (r *running) wantMigratedFile(t *testing.T, epic string) {
	t.Helper()
	path := filepath.Join(r.StateDir, "..", "..", "data", "gx", "tickets", "demo", epic, "issues", "01-broken-shape.md")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "status: paused") {
		t.Fatalf("ticket was not migrated:\n%s", body)
	}
}

// serverCtl runs the fixture's bare-server control script.
func (r *running) serverCtl(t *testing.T, verb string) string {
	t.Helper()
	out, err := exec.Command(filepath.Join(r.MigrateDir, "server"), verb).CombinedOutput()
	if err != nil {
		t.Fatalf("server %s: %v\n%s", verb, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestMigratePending_RealServerMigratesTheOldShapeTicketWithNoFlag(t *testing.T) {
	t.Parallel()
	r := start(t, "migrate-pending")
	r.waitMigrated(t)
	r.wantMigratedFile(t, "demo-epic")
	if got := r.failures(t); len(got) != 0 {
		t.Fatalf("failures = %v", got)
	}
}

func TestMigrateHeld_StaysMigratingUntilTheHoldFileIsRemoved(t *testing.T) {
	t.Parallel()
	r := start(t, "migrate-held")
	if !r.migrating(t) {
		t.Fatal("server is not migrating while the hold file exists")
	}
	time.Sleep(500 * time.Millisecond)
	if !r.migrating(t) {
		t.Fatal("migration ended while the hold file exists")
	}
	if err := os.Remove(filepath.Join(r.MigrateDir, "hold")); err != nil {
		t.Fatal(err)
	}
	r.waitMigrated(t)
	r.wantMigratedFile(t, "old-epic")
	waitUntil(t, "the ready ticket to leave open", func() bool { return r.ticketStatus(t, "demo:ready-epic/01") != "open" })
}

func TestMigrateBroken_BrokenKeepsASavedFailureAndDemoRuns(t *testing.T) {
	t.Parallel()
	r := start(t, "migrate-broken")
	r.waitMigrated(t)
	got := r.failures(t)
	if got["broken"] == "" || got["demo"] != "" {
		t.Fatalf("failures = %v, want one for broken only", got)
	}
	waitUntil(t, "demo's ticket to be claimed", func() bool { return r.ticketStatus(t, "demo:demo-epic/01") != "open" })
}

func TestMigrateServerControl_KillStartAndInjectedFailingStep(t *testing.T) {
	t.Parallel()
	r := start(t, "migrate-pending")
	r.waitMigrated(t)
	pid := r.serverCtl(t, "pid")
	inject := `[{"name":"test-fail","fail":"injected failure"}]`
	if err := os.WriteFile(filepath.Join(r.MigrateDir, "inject.json"), []byte(inject), 0o644); err != nil {
		t.Fatal(err)
	}
	r.serverCtl(t, "kill")
	r.serverCtl(t, "start")
	if got := r.serverCtl(t, "pid"); got == pid {
		t.Fatalf("pid %s did not change after a restart", got)
	}
	waitUntil(t, "the restarted server to answer", r.up)
	r.waitMigrated(t)
	if got := r.failures(t)["demo"]; !strings.Contains(got, "injected failure") {
		t.Fatalf("demo failure = %q, want the injected one", got)
	}
}

func TestMigrateScenarios_ServerStopsWithTheFixtureAfterARestart(t *testing.T) {
	t.Parallel()
	r := start(t, "migrate-pending")
	r.waitMigrated(t)
	r.serverCtl(t, "kill")
	r.serverCtl(t, "start")
	waitUntil(t, "the restarted server to answer", r.up)
	pid := r.serverCtl(t, "pid")
	r.stop(t)
	waitUntil(t, "the restarted server to stop", func() bool {
		return exec.Command("kill", "-0", pid).Run() != nil
	})
}
