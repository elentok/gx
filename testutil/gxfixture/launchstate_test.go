package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
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

// Story 29: a failed launch pauses the ticket as iteration-error with the
// launch error, and no surface still reads it as launching.
func TestLaunchFailScenario_TheTicketPausesWithTheErrorAndNothingReadsLaunching(t *testing.T) {
	t.Parallel()
	r := start(t, "launch-fail")
	defer r.stop(t)
	// The status moves on from paused once recovery escalates it, so wait on the reason.
	var d wire.TicketDetail
	if !testutil.Poll(90*time.Second, time.Second, func() bool {
		d = r.detail(t, launchTicket)
		return d.PauseReason == "iteration-error"
	}) {
		t.Fatalf("pause reason = %q, want iteration-error", d.PauseReason)
	}
	if !strings.Contains(d.PauseText, "launch failed") || !strings.Contains(d.PauseText, "installing dependencies") {
		t.Errorf("pause text = %q, want the launch error", d.PauseText)
	}
	if d.Stats != nil && d.Stats.LaunchStep != (wire.LaunchStep{}) {
		t.Errorf("ticket page launch step = %+v, want none", d.Stats.LaunchStep)
	}

	var snap wire.Snapshot
	if err := json.Unmarshal([]byte(r.gx(t, "server", "snapshot", "--json")), &snap); err != nil {
		t.Fatal(err)
	}
	for _, tk := range snap.Tickets {
		if tk.Address == launchTicket && tk.LaunchStep != (wire.LaunchStep{}) {
			t.Errorf("queue row launch step = %+v, want none", tk.LaunchStep)
		}
	}
	for _, c := range r.cardsOf(t) {
		if c.LaunchStep != (wire.LaunchStep{}) {
			t.Errorf("epic card %s launch step = %+v, want none", c.Name, c.LaunchStep)
		}
	}
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
