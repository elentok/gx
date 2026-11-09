package designflows

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/transcript"
)

const (
	launchTicket = "demo:launch-epic/01"
	capThird     = "demo:cap-epic/03"
)

func TestLaunchHeldScenario_TheTicketStaysClaimedAtInstallUntilTheHoldIsReleased(t *testing.T) {
	t.Parallel()
	r := start(t, "launch-held")
	defer r.Stop(t)
	started := filepath.Join(r.GateDir, "npm-started")
	if !testutil.Poll(90*time.Second, 50*time.Millisecond, func() bool { _, err := os.Stat(started); return err == nil }) {
		t.Fatalf("the install never started; ticket is %q", r.TicketStatus(t, launchTicket))
	}
	if got := r.TicketStatus(t, launchTicket); got != "claimed" {
		t.Fatalf("ticket = %q while the install is held, want claimed", got)
	}

	if err := os.Remove(filepath.Join(r.GateDir, "npm-hold")); err != nil {
		t.Fatal(err)
	}
	r.WaitStatus(t, launchTicket, "done")
}

// Story 29: a failed launch pauses the ticket as iteration-error with the
// launch error, and no surface still reads it as launching.
func TestLaunchFailScenario_TheTicketPausesWithTheErrorAndNothingReadsLaunching(t *testing.T) {
	t.Parallel()
	r := start(t, "launch-fail")
	defer r.Stop(t)
	// The status moves on from paused once recovery escalates it, so wait on the reason.
	var d wire.TicketDetail
	if !testutil.Poll(90*time.Second, time.Second, func() bool {
		d = r.Detail(t, launchTicket)
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
	if err := json.Unmarshal([]byte(r.Gx(t, "server", "snapshot", "--json")), &snap); err != nil {
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
	defer r.Stop(t)
	explain := func() string { return r.Gx(t, "server", "tickets", "explain", capThird) }
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
	r.Gx(t, "server", "reload-config")
	wait("epic cap 2/1")
}

const (
	vimLaunchTicket = "demo:vim-launch-epic/01"
	// vimCheckLogFragment is the stable part of the launch-failed log line.
	vimCheckLogFragment = "vim editor mode despite the editorMode normal launch override"
)

func TestVimLaunchScenario_TheServerLogShowsTheVimCheckAndTheTicketStillFinishes(t *testing.T) {
	t.Parallel()
	r := start(t, "vim-launch")
	r.WaitStatus(t, vimLaunchTicket, "done")
	var log string
	// A persistent footer fails the check on the launch and on its one relaunch.
	if !testutil.Poll(30*time.Second, 200*time.Millisecond, func() bool {
		data, err := os.ReadFile(r.LogFile)
		log = string(data)
		return err == nil && strings.Count(log, vimCheckLogFragment) >= 2
	}) {
		t.Fatalf("want at least two vim-check lines in %s, got:\n%s", r.LogFile, log)
	}
	r.Stop(t)
}

func TestToolsScenario_ChatListsTheCallsInFlight(t *testing.T) {
	t.Parallel()
	r := start(t, "agents-tools")
	list := r.seededSessions(t)
	if len(list) != 3 {
		t.Fatalf("%d sessions, want 3", len(list))
	}
	for i, s := range list {
		if s.Status.State != wire.SessionWorking {
			t.Errorf("session %d state = %s, want working", i, s.Status.State)
		}
	}
	if !list[2].Design {
		t.Errorf("third session is not a design session: %+v", list[2])
	}

	// A single call is Current alone; the newest of several is Current and the
	// older ones follow newest first.
	for i, wantOthers := range []int{0, 2, 0} {
		chat := r.waitChat(t, list[i].ID, func(c wire.SessionChatResponse) bool { return c.Current != nil })
		if chat.Current.Kind != transcript.InFlightShell && wantOthers == 0 {
			t.Errorf("session %d current = %+v, want the shell call", i, chat.Current)
		}
		if len(chat.Others) != wantOthers {
			t.Errorf("session %d others = %+v, want %d", i, chat.Others, wantOthers)
		}
		if wantOthers > 0 && (chat.Current.Kind != transcript.InFlightSearch || chat.Others[0].Kind != transcript.InFlightAgent || chat.Others[1].Kind != transcript.InFlightShell) {
			t.Errorf("session %d current %+v others %+v, want search, then agent, shell", i, chat.Current, chat.Others)
		}
	}
}
