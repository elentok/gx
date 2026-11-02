package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
)

const (
	stallVimTicket      = "demo:stall-vim-epic/01"
	stallITicket        = "demo:stall-i-epic/01"
	lateRegistryTicket  = "demo:late-registry-epic/01"
	noUserRecordTicket  = "demo:no-record-epic/01"
	skillPromptSnippet  = "/gx-implement"
	nudgeEventWaitLimit = 60 * time.Second
)

// events is the ticket's run-log events of one type.
func (r *running) events(t *testing.T, addr, typ string) []ralphloop.Event {
	t.Helper()
	var h server.History
	if err := json.Unmarshal([]byte(r.gx(t, "server", "tickets", "history", addr, "--json")), &h); err != nil {
		t.Fatal(err)
	}
	var out []ralphloop.Event
	for _, ev := range h.Events {
		if ev.Type == typ {
			out = append(out, ev)
		}
	}
	return out
}

func (r *running) waitEvents(t *testing.T, addr, typ string) []ralphloop.Event {
	t.Helper()
	var got []ralphloop.Event
	if !testutil.Poll(nudgeEventWaitLimit, 500*time.Millisecond, func() bool {
		got = r.events(t, addr, typ)
		return len(got) > 0
	}) {
		t.Fatalf("no %s event for %s", typ, addr)
	}
	return got
}

// paused fails the test when the ticket is in a paused state.
func (r *running) requireNotPaused(t *testing.T, addr string) {
	t.Helper()
	if s := r.ticketStatus(t, addr); strings.HasPrefix(s, "needs-") {
		t.Fatalf("ticket %s paused as %q:\n%s", addr, s, r.gx(t, "server", "tickets", "history", addr))
	}
}

// agentInput is what the fake agent read off its terminal.
func (r *running) agentInput(t *testing.T) string {
	t.Helper()
	home := filepath.Join(filepath.Dir(filepath.Dir(r.StateDir)), "home")
	paths, _ := filepath.Glob(agentfake.TUIInputPath(home, "*"))
	var all strings.Builder
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		all.Write(data)
	}
	return all.String()
}

func (r *running) toggleVim(t *testing.T) {
	t.Helper()
	home := filepath.Join(filepath.Dir(filepath.Dir(r.StateDir)), "home")
	var ctl string
	if !testutil.Poll(30*time.Second, 200*time.Millisecond, func() bool {
		paths, _ := filepath.Glob(agentfake.TUIControlPath(home, "*"))
		if len(paths) == 1 {
			ctl = paths[0]
		}
		return ctl != ""
	}) {
		t.Fatal("no fake agent control file")
	}
	if err := agentfake.SendControl(ctl, agentfake.Control{Action: agentfake.ActionVimToggle}); err != nil {
		t.Fatal(err)
	}
}

func TestStallVimScenario_AToggleToVimGetsTheNudgeWithNoStrayCharacter(t *testing.T) {
	t.Parallel()
	r := start(t, "stall-vim")
	// The agent's first turn is empty, so the stall nudge is the next prompt.
	r.toggleVim(t)
	r.waitStatus(t, stallVimTicket, "done")
	nudges := r.events(t, stallVimTicket, string(events.StallNudged))
	if len(nudges) == 0 || nudges[0].Text != ralphloop.StallNudgeText {
		t.Fatalf("stall-nudged events = %+v, want one with the nudge text", nudges)
	}
	in := r.agentInput(t)
	if strings.Contains(in, "i"+ralphloop.StallNudgeText) {
		t.Errorf("a stray i reached the input before the nudge:\n%q", in)
	}
	if !strings.Contains(in, ralphloop.StallNudgeText) {
		t.Errorf("the nudge text never reached the agent:\n%q", in)
	}
	r.stop(t)
}

func TestStallLeavesIScenario_TheNudgeRetrySucceedsAndTheTicketIsNotPaused(t *testing.T) {
	t.Parallel()
	r := start(t, "stall-leaves-i")
	r.waitEvents(t, stallITicket, string(events.StallNudged))
	r.waitStatus(t, stallITicket, "done")
	r.requireNotPaused(t, stallITicket)
	r.stop(t)
}

func TestLateRegistryScenario_TheLaunchPromptIsNudgedUntilATurnStarts(t *testing.T) {
	t.Parallel()
	r := start(t, "late-registry")
	r.waitEvents(t, lateRegistryTicket, string(events.PromptNudged))
	// The ticket finishes only when a turn started once the registry filled.
	r.waitStatus(t, lateRegistryTicket, "done")
	r.stop(t)
}

func TestNoUserRecordScenario_TheStallNudgeCarriesTheOriginalSkillPrompt(t *testing.T) {
	t.Parallel()
	r := start(t, "no-user-record")
	text := r.waitEvents(t, noUserRecordTicket, string(events.StallNudged))[0].Text
	if !strings.HasPrefix(text, skillPromptSnippet) || !strings.Contains(text, noUserRecordTicket) {
		t.Errorf("stall-nudged text = %q, want the original skill prompt for %s", text, noUserRecordTicket)
	}
	r.stop(t)
}
