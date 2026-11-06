package ticketwrite

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/tickets"
)

func TestPauseChatDetail_Families(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "01-x.md")
	if err := os.WriteFile(path, []byte("---\nid: \"01\"\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	withQuestion := filepath.Join(t.TempDir(), "02-x.md")
	if err := os.WriteFile(withQuestion, []byte("---\nid: \"02\"\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := tickets.AppendLog(withQuestion, tickets.LogEvent{
		Type: tickets.EventPauseQuestion, By: "agent", Summary: "Per-project cache?", Handoff: "h",
	}); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		path   string
		kind   events.Kind
		reason string
		want   string
	}{
		{"self-reported with question", withQuestion, events.SelfReported, "r", "Per-project cache?"},
		{"self-reported no question", path, events.SelfReported, "r", selfReportedNoQuestion},
		{"self-reported no path", "", events.SelfReported, "r", selfReportedNoQuestion},
		{"zero-commit", "", events.ZeroCommit, "no result reported", zeroCommitSentence},
		{"blocked prompt", "", events.BlockedPane, "x", blockedPaneSentence},
		{"stall kind", "", events.AgentPromptStalled, "agent_prompt_stalled: whatever", stalledSentence},
		{"stall nudge marker", "", events.IterationError, events.StallNudgeMarker.Reason("stall nudge for x: not delivered"), stalledSentence},
		{"stall nudge text changed", "", events.IterationError, events.StallNudgeMarker.Reason("reworded"), stalledSentence},
		{"session died", "", events.IterationError, events.SessionDiedMarker.Reason("holder socket /tmp/x.sock timeout"), "The agent session ended unexpectedly."},
		{"herdr lost", "", events.IterationError, events.HerdrLostMarker.Reason("herdr agent get: {\"error\":1}"), "herdr lost track of the agent."},
		{"unmatched", "", events.BudgetKilled, "daily budget hard limit reached", "daily budget hard limit reached"},
		{"empty reason", "", events.BudgetKilled, "", ""},
		{"multi-line", "", events.AmbiguousBase, "first\nsecond", "first"},
	}
	for _, c := range cases {
		if got := PauseChatDetail(c.path, c.kind, c.reason); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestPauseChatDetail_FallbackCutsAt100(t *testing.T) {
	t.Parallel()
	for _, n := range []int{99, 100, 101} {
		reason := strings.Repeat("a", n)
		got := PauseChatDetail("", events.BudgetKilled, reason)
		want := reason
		if n > 100 {
			want = strings.Repeat("a", 100) + "…"
		}
		if got != want {
			t.Errorf("len %d: got %q, want %q", n, got, want)
		}
	}
}

func TestPauseChatDetail_MarkerIgnoresErrorText(t *testing.T) {
	t.Parallel()
	const m events.Marker = "test-marker"
	table := []markedPause{{events.IterationError, m, "Marked sentence."}}
	for _, detail := range []string{"", "boom", "a completely different error"} {
		if got := pauseSentence("", events.IterationError, m.Reason(detail), table); got != "Marked sentence." {
			t.Errorf("detail %q: got %q", detail, got)
		}
	}
	if got := pauseSentence("", events.IterationError, "test-markerless", table); got == "Marked sentence." {
		t.Errorf("marker matched without its separator: %q", got)
	}
	if got := pauseSentence("", events.BudgetKilled, m.Reason("x"), table); got == "Marked sentence." {
		t.Errorf("marker matched under another kind: %q", got)
	}
}
