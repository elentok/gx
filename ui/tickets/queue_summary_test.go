package tickets

import (
	"github.com/elentok/gx/server/wire"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/server"
	gxtickets "github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
)

func TestEpicNameLine_NoTotalShowsNoCounterOrBar(t *testing.T) {
	t.Parallel()
	sum := wire.EpicSummary{Name: "one-offs", State: wire.EpicStateWaiting}
	got := ansi.Strip(epicNameLine("one-offs", sum, lipgloss.NewStyle(), epicLayoutWide, 40))
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
	if style.Render("x") != epicStatusProblemStyle.Render("x") {
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
		{"paused (repair)", repair, epicStatusPausedRepairStyle.Render(icons.Warning)},
		{"paused (answer)", answer, epicStatusPausedAnswerStyle.Render(icons.Warning)},
		{"deadlocked", wire.EpicSummary{State: wire.EpicStatePaused, Reason: wire.EpicReasonDeadlock}, epicStatusPausedRepairStyle.Render(icons.Warning)},
	} {
		icon, _, style := epicStatusLine(icons, tt.sum, "")
		if got := style.Render(icon); got != tt.want {
			t.Errorf("%s: icon rendered %q, want %q", tt.name, got, tt.want)
		}
	}
}

// fixtureSummaries stands in for the server's epic summaries in fixtures that
// only build tickets: it counts done tickets, lists the claimed ones that have
// a claim time as active, and the paused (answer) and paused (repair) ones as paused.
func fixtureSummaries(epics []gxtickets.Epic, claimedAt map[string]time.Time) []wire.EpicSummary {
	var out []wire.EpicSummary
	for _, epic := range epics {
		sum := wire.EpicSummary{
			Address: epic.Name, Name: epic.Name, State: wire.EpicStateWaiting, Total: len(epic.Tickets),
			Active: []wire.SummaryTicket{}, Paused: []wire.SummaryTicket{},
		}
		for _, t := range epic.Tickets {
			info := wire.SummaryTicket{Address: t.Path, Title: t.Title}
			switch status := epic.RenderedStatus(t); status {
			case gxtickets.StatusDone:
				sum.Done++
			case gxtickets.StatusClaimed:
				if at, ok := claimedAt[t.Path]; ok {
					info.ClaimedAt = at
					sum.Active = append(sum.Active, info)
					sum.State = wire.EpicStateRunning
				}
			case gxtickets.StatusPausedAnswer, gxtickets.StatusPausedRepair:
				info.Status, info.Reason = status.Word(), gxtickets.PauseDetail(epic, t)
				sum.Paused = append(sum.Paused, info)
			}
		}
		if sum.Total > 0 && sum.Done == sum.Total {
			sum.State = wire.EpicStateAwaitingMerge
		}
		out = append(out, sum)
	}
	return out
}

// summaryQueue delivers one epic with the given ticket rows and server summary.
func summaryQueue(t *testing.T, sum wire.EpicSummary, infos ...server.TicketInfo) QueueModel {
	t.Helper()
	items := make([]server.QueueItem, len(infos))
	for i, info := range infos {
		items[i] = server.QueueItem{Address: info.Address}
	}
	m, _ := deliverQueue(t, server.Snapshot{Seq: 1, Tickets: infos, Epics: []wire.EpicSummary{sum}}, items)
	return m
}

func TestQueueServerMode_HeaderReadsSummary(t *testing.T) {
	m := summaryQueue(t,
		wire.EpicSummary{Address: "gx:alpha", Name: "alpha", State: wire.EpicStateWaiting, Done: 3, Total: 7},
		server.TicketInfo{Address: "gx:alpha/01", Title: "First", Status: "open"},
	)

	body := ansi.Strip(m.View().Content)
	if !strings.Contains(body, "(3/7)") {
		t.Errorf("view lacks the summary counter (3/7):\n%s", body)
	}
	if !strings.Contains(body, "Waiting for a slot") {
		t.Errorf("view lacks the summary state label:\n%s", body)
	}
}

func TestQueueServerMode_ShowsPauseReasonFromSummary(t *testing.T) {
	m := summaryQueue(t,
		wire.EpicSummary{
			Address: "gx:alpha", Name: "alpha", State: wire.EpicStatePaused, Done: 0, Total: 1,
			Paused: []wire.SummaryTicket{{
				Address: "gx:alpha/01", Title: "First", Status: "paused", Reason: "Which approach, A or B?",
			}},
		},
		pausedAnswerInfo("gx:alpha/01", "First"),
	)

	body := ansi.Strip(m.View().Content)
	if !strings.Contains(body, "Which approach, A or B?") {
		t.Errorf("view lacks the pause reason line:\n%s", body)
	}
	if !strings.Contains(body, "Paused") {
		t.Errorf("view lacks the paused state label:\n%s", body)
	}
}

func TestQueueServerMode_DeadlockedWord(t *testing.T) {
	m := summaryQueue(t,
		wire.EpicSummary{Address: "gx:alpha", Name: "alpha", State: wire.EpicStatePaused, Total: 1, Reason: wire.EpicReasonDeadlock, Detail: "01 waits on itself"},
		server.TicketInfo{Address: "gx:alpha/01", Title: "First", Status: "open"},
	)

	if body := ansi.Strip(m.View().Content); !strings.Contains(body, "Paused: Dependency deadlock") {
		t.Errorf("view lacks the paused label and deadlock reason:\n%s", body)
	}
}
