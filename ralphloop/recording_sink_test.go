package ralphloop

import (
	"sync"
	"testing"

	"github.com/elentok/gx/tickets"
)

// recordingSink implements EventSink by appending each call's method name to
// a slice, for asserting the exact event sequence a Run call produces.
type recordingSink struct {
	mu                     sync.Mutex
	calls                  []string
	lastIterationStats     IterationStats
	lastEpicElapsedSeconds int
	// iterationStatsByTicket records every IterationFinished call's stats
	// keyed by ticket identifier, for scenarios (unlike
	// lastIterationStats) that need to inspect more than the most recent
	// call, e.g. asserting a scripted scheduling scenario's per-ticket
	// progress counts.
	iterationStatsByTicket map[string]IterationStats
	// onIterationFinished, if set, runs synchronously after recording each
	// IterationFinished call, letting a test unblock a still-running
	// iteration's fake AgentWait exactly once this ticket's stats have been
	// captured, deterministically rather than via a polling loop.
	onIterationFinished func(ticket tickets.Ticket)
	// parkedStalled records the stalled-ticket list of every EpicParked
	// call, so a test can assert both how many times a run parked and what
	// it named as blocking each park.
	parkedStalled [][]StalledTicket
	// ticketNeedsHumanCalls records every TicketNeedsHuman call's
	// (identifier, epicName, status, reason) tuple, for asserting both the
	// count and the identity/status a parking-write transition reports.
	ticketNeedsHumanCalls [][4]string
	// reattachedCalls records every TicketReattached call's
	// (identifier, label, cwd, sessionID) tuple, for asserting the live
	// session identity a reattach reports.
	reattachedCalls [][4]string
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

func (s *recordingSink) EpicStarted(epicName string, done, total int) { s.record("EpicStarted") }
func (s *recordingSink) TicketReverted(identifier string)             { s.record("TicketReverted") }
func (s *recordingSink) TicketReattached(identifier, label, cwd, sessionID string) {
	s.mu.Lock()
	s.reattachedCalls = append(s.reattachedCalls, [4]string{identifier, label, cwd, sessionID})
	s.mu.Unlock()
	s.record("TicketReattached")
}
func (s *recordingSink) TicketNeedsHuman(identifier, epicName, status, reason string) {
	s.mu.Lock()
	s.ticketNeedsHumanCalls = append(s.ticketNeedsHumanCalls, [4]string{identifier, epicName, status, reason})
	s.mu.Unlock()
	s.record("TicketNeedsHuman")
}
func (s *recordingSink) TicketClaimed(ticket tickets.Ticket) { s.record("TicketClaimed") }
func (s *recordingSink) IterationStarted(ticket tickets.Ticket, label, cwd, sessionID string) {
	s.record("IterationStarted")
}
func (s *recordingSink) IterationPaused(identifier, label string, kind PauseKind, reason string) {
	s.record("IterationPaused")
}
func (s *recordingSink) IterationResumed(identifier, label string, kind PauseKind) {
	s.record("IterationResumed")
}
func (s *recordingSink) IterationFinished(ticket tickets.Ticket, epicName string, stats IterationStats) {
	s.mu.Lock()
	s.lastIterationStats = stats
	if s.iterationStatsByTicket == nil {
		s.iterationStatsByTicket = map[string]IterationStats{}
	}
	s.iterationStatsByTicket[ticket.Identifier] = stats
	hook := s.onIterationFinished
	s.mu.Unlock()
	s.record("IterationFinished")
	if hook != nil {
		hook(ticket)
	}
}
func (s *recordingSink) TranscriptLine(label, line string) { s.record("TranscriptLine") }
func (s *recordingSink) ContextOccupancy(identifier string, tokens int) {
	s.record("ContextOccupancy")
}
func (s *recordingSink) TicketCleanupFinished(identifier string) {
	s.record("TicketCleanupFinished")
}
func (s *recordingSink) TicketRecovering(identifier string) {
	s.record("TicketRecovering")
}
func (s *recordingSink) TicketRecovered(identifier, epicName, branch, landedSHA string) {
	s.record("TicketRecovered")
}
func (s *recordingSink) TicketUnrecoverable(identifier, epicName string) {
	s.record("TicketUnrecoverable")
}
func (s *recordingSink) EpicParked(epicName string, stalled []StalledTicket) {
	s.mu.Lock()
	s.parkedStalled = append(s.parkedStalled, stalled)
	s.mu.Unlock()
	s.record("EpicParked")
}
func (s *recordingSink) EpicComplete(epicName string, completed int, elapsedSeconds int) {
	s.mu.Lock()
	s.lastEpicElapsedSeconds = elapsedSeconds
	s.mu.Unlock()
	s.record("EpicComplete")
}
func (s *recordingSink) EpicFailed(epicName string, err error) {
	s.record("EpicFailed")
}
func (s *recordingSink) NotificationFailed(channel, reason string) {
	s.record("NotificationFailed")
}
func (s *recordingSink) CherryPickStarted(identifier string) { s.record("CherryPickStarted") }
func (s *recordingSink) ConflictResolutionStarted(identifier string) {
	s.record("ConflictResolutionStarted")
}
func (s *recordingSink) SmartZoneCompactStarted(identifier string) {
	s.record("SmartZoneCompactStarted")
}
func (s *recordingSink) SmartZoneFinishingUp(identifier string) { s.record("SmartZoneFinishingUp") }
func (s *recordingSink) SmartZoneRecovered(identifier string)   { s.record("SmartZoneRecovered") }

func TestRun_EventSink_EmitsLifecycleSequenceForASingleTicket(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
	})
	d, _, _ := fakeDeps()

	sink := &recordingSink{}
	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, sink); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	want := []string{"EpicStarted", "TicketClaimed", "IterationStarted", "CherryPickStarted", "IterationFinished", "EpicComplete"}
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

func TestRecordingSink_CapturesIterationFinishedStatsAndEpicCompleteElapsedSeconds(t *testing.T) {
	t.Parallel()
	sink := &recordingSink{}

	stats := IterationStats{ElapsedSeconds: 42, PeakContextTokens: 12345, InProgress: 2, Completed: 3, Total: 5}
	sink.IterationFinished(tickets.Ticket{Identifier: "01"}, "epic", stats)
	if sink.lastIterationStats != stats {
		t.Errorf("lastIterationStats = %+v, want %+v", sink.lastIterationStats, stats)
	}

	sink.EpicComplete("epic", 5, 300)
	if sink.lastEpicElapsedSeconds != 300 {
		t.Errorf("lastEpicElapsedSeconds = %d, want 300", sink.lastEpicElapsedSeconds)
	}
}

func TestRun_EventSink_NoTicketsFound(t *testing.T) {
	t.Parallel()
	scratchDir := t.TempDir()
	d, _, _ := fakeDeps()

	sink := &recordingSink{}
	if err := Run(RunOptions{EpicName: "missing-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, sink); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if got := sink.snapshot(); len(got) != 1 || got[0] != "EpicStarted" {
		t.Errorf("events = %v, want [EpicStarted]", got)
	}
}

// TestRun_EventSink_AlreadyCompleteEpic_EmitsExactlyOneEpicStarted pins the
// other half of the fold: an epic whose every ticket is already done
// produces the same single EpicStarted event as a fresh run or an empty
// epic, not a separate "already complete" event.
func TestRun_EventSink_AlreadyCompleteEpic_EmitsExactlyOneEpicStarted(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: done\ntype: task\n---\n# A\n",
	})
	d, _, _ := fakeDeps()

	sink := &recordingSink{}
	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, sink); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if got := sink.snapshot(); len(got) != 1 || got[0] != "EpicStarted" {
		t.Errorf("events = %v, want [EpicStarted]", got)
	}
}
