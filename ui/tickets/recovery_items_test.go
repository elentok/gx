package tickets

import (
	"errors"
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/config"
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

// The TUI has no input for a reply or a base, and the server has no repair verb
// for reopen-and-retry, so these verbs must never reach the server as a resume.
func TestServerWrite_WebOnlyVerbsNeverResume(t *testing.T) {
	for _, verb := range []string{server.ActionAnswer, server.ActionChooseBase, server.ActionReopenAndRetry, "no-such-verb"} {
		t.Run(verb, func(t *testing.T) {
			var calls []string
			cmd := cmdServerWrite(fakeServerAPI{calls: &calls}, verb, "gx:alpha/02")
			if cmd == nil {
				t.Fatal("no toast for a web-only verb")
			}
			cmd()
			if len(calls) != 0 {
				t.Fatalf("calls = %v, want none", calls)
			}
		})
	}
}

func TestServerWrite_ResumeStillResumes(t *testing.T) {
	var calls []string
	cmdServerWrite(fakeServerAPI{calls: &calls}, server.ActionResume, "gx:alpha/02")()
	if want := []string{"resume gx:alpha/02"}; !slices.Equal(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestServerMode_EnterSkipsWebOnlyActions(t *testing.T) {
	for _, tc := range []struct {
		name    string
		actions []string
		want    []string
	}{
		{"only web verbs", []string{server.ActionAnswer, server.ActionChooseBase}, nil},
		{"web verb first", []string{server.ActionAnswer, server.ActionResume}, []string{"resume gx:alpha/02"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls []string
			m := newServerModel(t).WithServer(fakeServerAPI{calls: &calls, detail: detailWith(tc.actions...)})
			m = withState(m, server.Snapshot{Tickets: []server.TicketInfo{
				{Address: "gx:alpha/02", Title: "Second", Status: string(schema.StatusLegacyRepair), RenderedStatus: tickets.StatusPausedRepair.Word()},
			}})
			updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
			m = selectTicketRow(t, updated.(Model))

			_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			if cmd != nil {
				cmd()
			}
			if !slices.Equal(calls, tc.want) {
				t.Fatalf("calls = %v, want %v", calls, tc.want)
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

// A failed detail read opens the menu without recovery items and reports the
// error, instead of leaving the menu silently empty.
func TestCmdTicketMenu_ReportsAFailedDetailRead(t *testing.T) {
	api := fakeServerAPI{detailErr: errors.New("boom")}
	ticket := tickets.Ticket{Path: "gx:alpha/02", Status: string(schema.StatusPaused), PauseReason: schema.PauseIterationError}
	msg, ok := cmdTicketMenu(api, config.Dirs{}, ticketMenuInput{rendered: ticket.RawStatus(), ticket: ticket})().(ticketMenuReadyMsg)
	if !ok {
		t.Fatal("no ticketMenuReadyMsg")
	}
	if msg.detailErr == nil || msg.toast() == nil {
		t.Fatalf("detailErr = %v, toast = %v; want both set", msg.detailErr, msg.toast())
	}
	for _, v := range []schema.PauseVerb{schema.VerbRetry, schema.VerbStartOver} {
		if slices.ContainsFunc(msg.items, func(it contextmenu.Item[ticketTarget]) bool { return it.Label == v.Label() }) {
			t.Errorf("menu offers %q after a failed detail read", v.Label())
		}
	}
}

// The TUI trusts the server's ActionInfo; a word without one shows as itself.
func TestRecoveryActions_NoActionInfoFallsBackToTheWord(t *testing.T) {
	got := recoveryActions(&server.TicketDetail{Actions: []string{server.ActionRetry}})
	if len(got) != 1 || got[0].label != server.ActionRetry || got[0].help != "" {
		t.Fatalf("got %+v, want the bare word as label and no help", got)
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
