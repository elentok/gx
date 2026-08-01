package ralphloop

import (
	"bytes"
	"sync"
	"testing"

	"github.com/elentok/gx/tickets"
)

// recordingSink implements EventSink by appending each call's method name to
// a slice, for asserting the exact event sequence a Run call produces
// (rather than only the text a textEventSink renders from it).
type recordingSink struct {
	mu    sync.Mutex
	calls []string
}

func (s *recordingSink) record(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, name)
}

func (s *recordingSink) snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.calls))
	copy(out, s.calls)
	return out
}

func (s *recordingSink) NoTicketsFound(epicName string) { s.record("NoTicketsFound") }
func (s *recordingSink) AlreadyComplete(epicName string, done, total int) {
	s.record("AlreadyComplete")
}
func (s *recordingSink) TicketReverted(identifier string) { s.record("TicketReverted") }
func (s *recordingSink) TicketReattached(identifier string, label string) {
	s.record("TicketReattached")
}
func (s *recordingSink) TicketStillNeedsAttention(identifier string) {
	s.record("TicketStillNeedsAttention")
}
func (s *recordingSink) TicketClaimed(ticket tickets.Ticket) { s.record("TicketClaimed") }
func (s *recordingSink) IterationStarted(identifier string, label string) {
	s.record("IterationStarted")
}
func (s *recordingSink) IterationPaused(label string, kind PauseKind, reason string) {
	s.record("IterationPaused")
}
func (s *recordingSink) IterationResumed(label string, kind PauseKind) { s.record("IterationResumed") }
func (s *recordingSink) IterationFinished(ticket tickets.Ticket, epicName string) {
	s.record("IterationFinished")
}
func (s *recordingSink) TranscriptLine(label, line string) { s.record("TranscriptLine") }
func (s *recordingSink) TicketCleanupFinished(identifier string) {
	s.record("TicketCleanupFinished")
}
func (s *recordingSink) TicketRecovered(identifier, epicName, branch, landedSHA string) {
	s.record("TicketRecovered")
}
func (s *recordingSink) TicketUnrecoverable(identifier, epicName string) {
	s.record("TicketUnrecoverable")
}
func (s *recordingSink) EpicComplete(epicName string, completed int) { s.record("EpicComplete") }

func TestRun_EventSink_EmitsLifecycleSequenceForASingleTicket(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "# A\n\n**Status:** open\n",
	})
	d, _, _ := fakeDeps()

	sink := &recordingSink{}
	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, sink); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	want := []string{"TicketClaimed", "IterationStarted", "IterationFinished", "EpicComplete"}
	got := sink.snapshot()
	if len(got) != len(want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("events[%d] = %q, want %q (full sequence %v)", i, got[i], w, got)
		}
	}
}

func TestRun_EventSink_NoTicketsFound(t *testing.T) {
	scratchDir := t.TempDir()
	d, _, _ := fakeDeps()

	sink := &recordingSink{}
	if err := Run(RunOptions{EpicName: "missing-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, sink); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if got := sink.snapshot(); len(got) != 1 || got[0] != "NoTicketsFound" {
		t.Errorf("events = %v, want [NoTicketsFound]", got)
	}
}

// TestNewTextEventSink_RendersSameTextAsBeforeTheEventSinkRefactor pins the
// headless CLI's rendered output for a representative slice of events, so a
// future change to textEventSink can't silently break gx ralph-loop's
// stdout output for a piped/CI run.
func TestNewTextEventSink_RendersSameTextAsBeforeTheEventSinkRefactor(t *testing.T) {
	var out bytes.Buffer
	sink := NewTextEventSink(&out)

	sink.NoTicketsFound("epic")
	sink.AlreadyComplete("epic", 2, 2)
	sink.TicketReverted("01")
	sink.TicketReattached("01", "iter-01")
	sink.TicketStillNeedsAttention("01")
	sink.IterationPaused("iter-01", PauseRateLimit, "rate limit detected")
	sink.IterationResumed("iter-01", PauseRateLimit)
	sink.IterationPaused("iter-01", PauseSmartZone, "context occupancy 200000 exceeds --smart-zone 150000")
	sink.IterationResumed("iter-01", PauseSmartZone)
	sink.IterationFinished(tickets.Ticket{Number: 1, Identifier: "01", Title: "First"}, "epic")
	sink.TicketCleanupFinished("01")
	sink.TicketRecovered("01", "epic", "ralph-loop/iter-01", "deadbeef")
	sink.TicketUnrecoverable("01", "epic")
	sink.EpicComplete("epic", 1)

	// These no-op in the headless CLI: no line should be printed for them.
	sink.TicketClaimed(tickets.Ticket{Number: 1})
	sink.IterationStarted("01", "iter-01")
	sink.TranscriptLine("iter-01", "some transcript text")

	want := "" +
		"no tickets found for epic \"epic\"; nothing to do\n" +
		"epic \"epic\" is already complete (2/2 done)\n" +
		"ticket 01: no live iteration found on restart; reverted to open\n" +
		"ticket 01: reattaching to live iteration iter-01\n" +
		"ticket 01 still needs attention; no live iteration found\n" +
		"paused iter-01: rate limit detected; waiting for automatic reset\n" +
		"resumed iter-01 after rate-limit reset\n" +
		"paused iter-01: context occupancy 200000 exceeds --smart-zone 150000; run `gx ralph-loop resume` to continue\n" +
		"resumed iter-01\n" +
		"ticket 01 \"First\" landed on epic\n" +
		"ticket 01: done and commits landed, but leftover iteration state was never cleaned up; finished the interrupted cleanup\n" +
		"ticket 01: done but commits were missing from epic; auto re-cherry-picked from iteration branch ralph-loop/iter-01 and restored (deadbeef)\n" +
		"ticket 01: done but commits missing from epic and no iteration branch left to recover them; marked needs-attention\n" +
		"ralph-loop \"epic\" complete: 1 ticket(s) landed on epic\n"

	if out.String() != want {
		t.Errorf("rendered text =\n%s\nwant\n%s", out.String(), want)
	}
}
