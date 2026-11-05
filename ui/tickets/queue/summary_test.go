package queue

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/ui"
)

func TestEpicNameLine_NoTotalShowsNoCounterOrBar(t *testing.T) {
	t.Parallel()
	sum := wire.EpicSummary{Name: "one-offs", State: wire.EpicStateWaiting}
	got := ansi.Strip(EpicNameLine("one-offs", sum, lipgloss.NewStyle(), EpicLayoutWide, 40))
	if strings.TrimSpace(got) != "one-offs" {
		t.Errorf("name line = %q, want the name alone", got)
	}
	_, text, _ := epicStatusLine(ui.Icons(false), sum, "")
	if strings.Contains(text, "merge") {
		t.Errorf("status = %q, must not read awaiting merge", text)
	}
}

func TestEpicStatusLine_UsesStateLabels(t *testing.T) {
	t.Parallel()
	icons := ui.Icons(false)
	tests := []struct {
		state    wire.EpicState
		wantIcon string
		wantText string
	}{
		{wire.EpicStateAwaitingMerge, icons.TicketDone, "Awaiting merge"},
		{wire.EpicStateRunning, "S", "Running"},
		{wire.EpicStateWaiting, icons.EpicQueued, "Waiting for a slot"},
		{wire.EpicStatePaused, icons.Warning, "Paused"},
		{wire.EpicStateMerging, "S", "Merging"},
		{wire.EpicStateFinished, icons.TicketDone, "Merged"},
	}
	for _, tt := range tests {
		icon, text, _ := epicStatusLine(icons, wire.EpicSummary{State: tt.state}, "S")
		if icon != tt.wantIcon || text != tt.wantText {
			t.Errorf("state %q: got icon=%q text=%q, want icon=%q text=%q", tt.state, icon, text, tt.wantIcon, tt.wantText)
		}
	}
}

func TestEpicStatusLine_PausedShowsReasonAndMore(t *testing.T) {
	t.Parallel()
	sum := wire.EpicSummary{State: wire.EpicStatePaused, Reason: "Waiting for your answer", More: 2}
	_, text, _ := epicStatusLine(ui.Icons(false), sum, "")
	if text != "Paused: Waiting for your answer +2 more" {
		t.Errorf("text = %q, want the label, the reason and the count of the others", text)
	}
}

func TestEpicStatusLine_MergeBlockedShowsReason(t *testing.T) {
	t.Parallel()
	_, text, style := epicStatusLine(ui.Icons(false), wire.EpicSummary{State: wire.EpicStatePaused, Reason: wire.EpicReasonMergeBlocked, MergeBlocked: true}, "")
	if text != "Paused: Merge blocked" {
		t.Errorf("text = %q, want the label followed by the reason", text)
	}
	if style.Render("x") != EpicStatusProblemStyle.Render("x") {
		t.Errorf("merge blocked should use the warning style")
	}
}

func TestEpicStatusLine_Colors(t *testing.T) {
	t.Parallel()
	icons := ui.Icons(false)
	repair := wire.EpicSummary{State: wire.EpicStatePaused, HasRepair: true, Paused: []wire.SummaryTicket{
		{Status: "paused"}, {Status: "paused"},
	}}
	answer := wire.EpicSummary{State: wire.EpicStatePaused, Paused: []wire.SummaryTicket{{Status: "paused"}}}
	for _, tt := range []struct {
		name string
		sum  wire.EpicSummary
		want string
	}{
		{"awaiting-merge", wire.EpicSummary{State: wire.EpicStateAwaitingMerge}, epicStatusDoneStyle.Render(icons.TicketDone)},
		{"paused (repair)", repair, EpicStatusPausedRepairStyle.Render(icons.Warning)},
		{"paused (answer)", answer, EpicStatusPausedAnswerStyle.Render(icons.Warning)},
		{"deadlocked", wire.EpicSummary{State: wire.EpicStatePaused, Reason: wire.EpicReasonDeadlock}, EpicStatusPausedRepairStyle.Render(icons.Warning)},
	} {
		icon, _, style := epicStatusLine(icons, tt.sum, "")
		if got := style.Render(icon); got != tt.want {
			t.Errorf("%s: icon rendered %q, want %q", tt.name, got, tt.want)
		}
	}
}
