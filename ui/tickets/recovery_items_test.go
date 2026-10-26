package tickets

import (
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui/contextmenu"
)

// resumeLabel is the pause-to-verb table's label for resume.
var resumeLabel = schema.VerbResume.Label()

// detailWith is a detail whose actions carry the table's label and tooltip, as
// the server sends them.
func detailWith(actions ...string) server.TicketDetail {
	d := server.TicketDetail{Actions: actions, ActionInfo: map[string]server.ActionInfo{}}
	for _, a := range actions {
		v, ok := schema.VerbOfWord(a)
		if !ok {
			continue
		}
		d.ActionInfo[a] = server.ActionInfo{Label: v.Label(), Tooltip: v.Tooltip()}
	}
	return d
}

var answerDetail = detailWith(server.ActionAnswer, server.ActionResume)

func recoveryMenu(t *testing.T, status schema.Status, reason schema.PauseReason, d server.TicketDetail) []contextmenu.Item[ticketTarget] {
	t.Helper()
	ticket := tickets.Ticket{Status: string(status), PauseReason: reason}
	return ticketMenuItems(ticketMenuInput{rendered: ticket.RawStatus(), ticket: ticket, detail: &d})
}

func TestRecoveryItems_MatchDetailActions(t *testing.T) {
	cases := []struct {
		name    string
		status  schema.Status
		reason  schema.PauseReason
		actions []string
	}{
		{"repair pause", schema.StatusPaused, schema.PauseIterationError, []string{server.ActionRetry, server.ActionStartOver}},
		{"answer pause", schema.StatusPaused, schema.PauseSelfReported, []string{server.ActionAnswer, server.ActionResume}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := detailWith(tc.actions...)
			items := recoveryMenu(t, tc.status, tc.reason, d)
			for _, a := range tc.actions {
				if it := itemByLabel(t, items, d.ActionInfo[a].Label); it.Disabled != "" && it.Disabled != serverNotRunning {
					t.Errorf("%s disabled: %q", a, it.Disabled)
				}
			}
			for _, label := range []string{"Cancel", "Investigate", "Set open"} {
				itemByLabel(t, items, label)
			}
			if slices.ContainsFunc(items, func(it contextmenu.Item[ticketTarget]) bool { return it.Label == "Relaunch" }) {
				t.Error("menu still offers Relaunch")
			}
			// A verb the detail does not list is not offered.
			unlisted := schema.VerbResume.Label()
			if tc.name == "answer pause" {
				unlisted = schema.VerbRetry.Label()
			}
			if slices.ContainsFunc(items, func(it contextmenu.Item[ticketTarget]) bool { return it.Label == unlisted }) {
				t.Errorf("menu offers %q, which detail.actions lacks", unlisted)
			}
		})
	}
}

func TestRecoveryItems_NoDetailOffersNone(t *testing.T) {
	ticket := tickets.Ticket{Status: string(schema.StatusPaused), PauseReason: schema.PauseIterationError}
	items := ticketMenuItems(ticketMenuInput{rendered: ticket.RawStatus(), ticket: ticket})
	if slices.ContainsFunc(items, func(it contextmenu.Item[ticketTarget]) bool { return it.Label == schema.VerbRetry.Label() }) {
		t.Error("menu offers Retry without a detail")
	}
}

func TestRecoveryItems_ShowTheTooltipAsHelp(t *testing.T) {
	d := detailWith(server.ActionRetry)
	it := itemByLabel(t, recoveryMenu(t, schema.StatusPaused, schema.PauseIterationError, d), schema.VerbRetry.Label())
	if it.Help == "" || it.Help != schema.VerbRetry.Tooltip() {
		t.Errorf("help = %q, want %q", it.Help, schema.VerbRetry.Tooltip())
	}
}

func TestRecoveryItems_MarkTheRecommendedVerb(t *testing.T) {
	d := detailWith(server.ActionRetry, server.ActionStartOver)
	d.Recommendation = &server.DetailRecommendation{Verb: server.ActionStartOver, Diagnosis: "bad branch"}
	items := recoveryMenu(t, schema.StatusPaused, schema.PauseIterationError, d)

	rec := itemByLabel(t, items, schema.VerbStartOver.Label()+recommendedMark)
	if rec.Group != contextmenu.Suggested {
		t.Errorf("recommended group = %v, want Suggested", rec.Group)
	}
	itemByLabel(t, items, schema.VerbRetry.Label())
}

// Seam D: "enter" on a repair pause runs the first verb of detail.actions.
func TestServerMode_EnterRunsTheFirstDetailAction(t *testing.T) {
	var calls []string
	api := fakeServerAPI{calls: &calls, detail: detailWith(server.ActionRetry, server.ActionStartOver)}
	m := newServerModel(t).WithServer(api)
	m = withState(m, server.Snapshot{Tickets: []server.TicketInfo{
		{Address: "gx:alpha/02", Title: "Second", Status: string(schema.StatusLegacyRepair), RenderedStatus: tickets.StatusPausedRepair.Word()},
	}})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = selectTicketRow(t, updated.(Model))

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter on a paused row issued nothing")
	}
	cmd()
	if want := []string{"relaunch gx:alpha/02"}; !slices.Equal(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestServerMode_EnterWithNoActionsDoesNothing(t *testing.T) {
	var calls []string
	m := newServerModel(t).WithServer(fakeServerAPI{calls: &calls, detail: detailWith()})
	m = withState(m, server.Snapshot{Tickets: []server.TicketInfo{
		{Address: "gx:alpha/02", Title: "Second", Status: string(schema.StatusLegacyRepair), RenderedStatus: tickets.StatusPausedRepair.Word()},
	}})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = selectTicketRow(t, updated.(Model))

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		if msg := cmd(); msg != nil {
			t.Fatalf("enter with no actions produced %T", msg)
		}
	}
	if len(calls) != 0 {
		t.Fatalf("calls = %v, want none", calls)
	}
}
