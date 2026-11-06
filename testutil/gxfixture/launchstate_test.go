package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/testutil"
)

const (
	launchTicket = "demo:launch-epic/01"
	capThird     = "demo:cap-epic/03"
)

func TestLaunchHeldScenario_TheTicketStaysClaimedAtInstallUntilTheHoldIsReleased(t *testing.T) {
	t.Parallel()
	r := start(t, "launch-held")
	defer r.stop(t)
	started := filepath.Join(r.GateDir, "npm-started")
	if !testutil.Poll(90*time.Second, 50*time.Millisecond, func() bool { _, err := os.Stat(started); return err == nil }) {
		t.Fatalf("the install never started; ticket is %q", r.ticketStatus(t, launchTicket))
	}
	if got := r.ticketStatus(t, launchTicket); got != "claimed" {
		t.Fatalf("ticket = %q while the install is held, want claimed", got)
	}

	if err := os.Remove(filepath.Join(r.GateDir, "npm-hold")); err != nil {
		t.Fatal(err)
	}
	r.waitStatus(t, launchTicket, "done")
}

func TestEpicAtCapScenario_TheHeldTicketIsCappedAndLoweringTheCapNamesTheOverCap(t *testing.T) {
	t.Parallel()
	r := start(t, "epic-at-cap")
	defer r.stop(t)
	explain := func() string { return r.gx(t, "server", "tickets", "explain", capThird) }
	wait := func(want string) {
		t.Helper()
		if !testutil.Poll(90*time.Second, time.Second, func() bool { return strings.Contains(explain(), want) }) {
			t.Fatalf("explain %s = %q, want %q", capThird, explain(), want)
		}
	}
	wait("concurrency cap")
	wait("epic cap 2/2")

	raw, err := os.ReadFile(r.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	cfg["execution-queue"].(map[string]any)["max-agents-per-epic"] = 1
	raw, err = json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.ConfigFile, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	r.gx(t, "server", "reload-config")
	wait("epic cap 2/1")
}
