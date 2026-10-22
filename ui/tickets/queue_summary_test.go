package tickets

import (
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
	sum := server.EpicSummary{Name: "one-offs", State: server.EpicStateWaiting}
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
		state    server.EpicState
		wantIcon string
		wantText string
	}{
		{server.EpicStateAwaitingMerge, icons.TicketDone, "Awaiting merge"},
		{server.EpicStateRunning, "S", "Running"},
		{server.EpicStateWaiting, icons.EpicQueued, "Waiting for a slot"},
		{server.EpicStatePaused, icons.Warning, "Paused"},
		{server.EpicStateMerging, "S", "Merging"},
	}
	for _, tt := range tests {
		icon, text, _ := epicStatusLine(icons, server.EpicSummary{State: tt.state}, "S")
		if icon != tt.wantIcon || text != tt.wantText {
			t.Errorf("state %q: got icon=%q text=%q, want icon=%q text=%q", tt.state, icon, text, tt.wantIcon, tt.wantText)
		}
	}
}

func TestEpicStatusLine_PausedShowsReasonAndMore(t *testing.T) {
	t.Parallel()
	sum := server.EpicSummary{State: server.EpicStatePaused, Reason: "Waiting for your answer", More: 2}
	_, text, _ := epicStatusLine(ui.Icons(false), sum, "")
	if text != "Paused: Waiting for your answer +2 more" {
		t.Errorf("text = %q, want the label, the reason and the count of the others", text)
	}
}

func TestEpicStatusLine_MergeBlockedShowsReason(t *testing.T) {
	t.Parallel()
	_, text, style := epicStatusLine(ui.Icons(false), server.EpicSummary{State: server.EpicStatePaused, Reason: server.EpicReasonMergeBlocked, MergeBlocked: true}, "")
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
	repair := server.EpicSummary{State: server.EpicStatePaused, HasRepair: true, Paused: []server.SummaryTicket{
		{Status: "needs-answer"}, {Status: "needs-repair"},
	}}
	answer := server.EpicSummary{State: server.EpicStatePaused, Paused: []server.SummaryTicket{{Status: "needs-answer"}}}
	for _, tt := range []struct {
		name string
		sum  server.EpicSummary
		want string
	}{
		{"awaiting-merge", server.EpicSummary{State: server.EpicStateAwaitingMerge}, epicStatusDoneStyle.Render(icons.TicketDone)},
		{"parked needs-repair", repair, epicStatusParkedRepairStyle.Render(icons.Warning)},
		{"parked needs-answer", answer, epicStatusParkedAnswerStyle.Render(icons.Warning)},
		{"deadlocked", server.EpicSummary{State: server.EpicStatePaused, Reason: server.EpicReasonDeadlock}, epicStatusParkedRepairStyle.Render(icons.Warning)},
	} {
		icon, _, style := epicStatusLine(icons, tt.sum, "")
		if got := style.Render(icon); got != tt.want {
			t.Errorf("%s: icon rendered %q, want %q", tt.name, got, tt.want)
		}
	}
}

// fixtureSummaries stands in for the server's epic summaries in fixtures that
// only build tickets: it counts done tickets, lists the claimed ones that have
// a claim time as active, and the needs-answer/needs-repair ones as parked.
func fixtureSummaries(epics []gxtickets.Epic, claimedAt map[string]time.Time) []server.EpicSummary {
	var out []server.EpicSummary
	for _, epic := range epics {
		sum := server.EpicSummary{
			Address: epic.Name, Name: epic.Name, State: server.EpicStateWaiting, Total: len(epic.Tickets),
			Active: []server.SummaryTicket{}, Paused: []server.SummaryTicket{},
		}
		for _, t := range epic.Tickets {
			info := server.SummaryTicket{Address: t.Path, Title: t.Title}
			switch status := epic.RenderedStatus(t); status {
			case gxtickets.StatusDone:
				sum.Done++
			case gxtickets.StatusClaimed:
				if at, ok := claimedAt[t.Path]; ok {
					info.ClaimedAt = at
					sum.Active = append(sum.Active, info)
					sum.State = server.EpicStateRunning
				}
			case gxtickets.StatusPausedAnswer, gxtickets.StatusPausedRepair:
				info.Status, info.Reason = status.Word(), gxtickets.PauseDetail(epic, t)
				sum.Paused = append(sum.Paused, info)
			}
		}
		if sum.Total > 0 && sum.Done == sum.Total {
			sum.State = server.EpicStateAwaitingMerge
		}
		out = append(out, sum)
	}
	return out
}

// summaryQueue delivers one epic with the given ticket rows and server summary.
func summaryQueue(t *testing.T, sum server.EpicSummary, infos ...server.TicketInfo) QueueModel {
	t.Helper()
	items := make([]server.QueueItem, len(infos))
	for i, info := range infos {
		items[i] = server.QueueItem{Address: info.Address}
	}
	m, _ := deliverQueue(t, server.Snapshot{Seq: 1, Tickets: infos, Epics: []server.EpicSummary{sum}}, items)
	return m
}

func TestQueueServerMode_HeaderReadsSummary(t *testing.T) {
	m := summaryQueue(t,
		server.EpicSummary{Address: "gx:alpha", Name: "alpha", State: server.EpicStateWaiting, Done: 3, Total: 7},
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

func TestQueueServerMode_ShowsParkReasonFromSummary(t *testing.T) {
	m := summaryQueue(t,
		server.EpicSummary{
			Address: "gx:alpha", Name: "alpha", State: server.EpicStatePaused, Done: 0, Total: 1,
			Paused: []server.SummaryTicket{{
				Address: "gx:alpha/01", Title: "First", Status: "needs-answer", Reason: "Which approach, A or B?",
			}},
		},
		server.TicketInfo{Address: "gx:alpha/01", Title: "First", Status: "needs-answer"},
	)

	body := ansi.Strip(m.View().Content)
	if !strings.Contains(body, "Which approach, A or B?") {
		t.Errorf("view lacks the Park reason line:\n%s", body)
	}
	if !strings.Contains(body, "Paused") {
		t.Errorf("view lacks the paused state label:\n%s", body)
	}
}

func TestQueueServerMode_DeadlockedWord(t *testing.T) {
	m := summaryQueue(t,
		server.EpicSummary{Address: "gx:alpha", Name: "alpha", State: server.EpicStatePaused, Total: 1, Reason: server.EpicReasonDeadlock, Detail: "01 waits on itself"},
		server.TicketInfo{Address: "gx:alpha/01", Title: "First", Status: "open"},
	)

	if body := ansi.Strip(m.View().Content); !strings.Contains(body, "Paused: Dependency deadlock") {
		t.Errorf("view lacks the paused label and deadlock reason:\n%s", body)
	}
}
