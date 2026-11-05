package rowactions

import (
	"errors"
	"slices"
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui/contextmenu"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
)

func recoveryMenu(t *testing.T, status schema.Status, reason schema.PauseReason, d wire.TicketDetail) []contextmenu.Item[TicketTarget] {
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
		{"repair pause", schema.StatusPaused, schema.PauseIterationError, []string{wire.ActionRetry, wire.ActionStartOver}},
		{"answer pause", schema.StatusPaused, schema.PauseSelfReported, []string{wire.ActionAnswer, wire.ActionResume}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := detailWith(tc.actions...)
			items := recoveryMenu(t, tc.status, tc.reason, d)
			for _, a := range tc.actions {
				if it := itemByLabel(t, items, d.ActionInfo[a].Label); it.Disabled != "" && it.Disabled != ServerNotRunning {
					t.Errorf("%s disabled: %q", a, it.Disabled)
				}
			}
			for _, label := range []string{"Cancel", "Investigate", "Set open"} {
				itemByLabel(t, items, label)
			}
			if slices.ContainsFunc(items, func(it contextmenu.Item[TicketTarget]) bool { return it.Label == "Relaunch" }) {
				t.Error("menu still offers Relaunch")
			}
			// A verb the detail does not list is not offered.
			unlisted := schema.VerbResume.Label()
			if tc.name == "answer pause" {
				unlisted = schema.VerbRetry.Label()
			}
			if slices.ContainsFunc(items, func(it contextmenu.Item[TicketTarget]) bool { return it.Label == unlisted }) {
				t.Errorf("menu offers %q, which detail.actions lacks", unlisted)
			}
		})
	}
}

// The TUI has no input for a reply or a base, and the server has no repair verb
// for reopen-and-retry, so these verbs must never reach the server as a resume.
func TestServerWrite_WebOnlyVerbsNeverResume(t *testing.T) {
	for _, verb := range []string{wire.ActionAnswer, wire.ActionChooseBase, wire.ActionReopenAndRetry, "no-such-verb"} {
		t.Run(verb, func(t *testing.T) {
			var calls []string
			cmd := cmdServerWrite(tickettest.FakeAdmin{FakeServerAPI: tickettest.FakeServerAPI{Calls: &calls}}, tickettest.FakeServerAPI{Calls: &calls}, verb, "gx:alpha/02")
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
	cmdServerWrite(tickettest.FakeAdmin{FakeServerAPI: tickettest.FakeServerAPI{Calls: &calls}}, tickettest.FakeServerAPI{Calls: &calls}, wire.ActionResume, "gx:alpha/02")()
	if want := []string{"resume gx:alpha/02"}; !slices.Equal(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestRecoveryItems_NoDetailOffersNone(t *testing.T) {
	ticket := tickets.Ticket{Status: string(schema.StatusPaused), PauseReason: schema.PauseIterationError}
	items := ticketMenuItems(ticketMenuInput{rendered: ticket.RawStatus(), ticket: ticket})
	if slices.ContainsFunc(items, func(it contextmenu.Item[TicketTarget]) bool { return it.Label == schema.VerbRetry.Label() }) {
		t.Error("menu offers Retry without a detail")
	}
}

// A failed detail read opens the menu without recovery items and reports the
// error, instead of leaving the menu silently empty.
func TestCmdTicketMenu_ReportsAFailedDetailRead(t *testing.T) {
	api := tickettest.FakeServerAPI{DetailErr: errors.New("boom")}
	ticket := tickets.Ticket{Path: "gx:alpha/02", Status: string(schema.StatusPaused), PauseReason: schema.PauseIterationError}
	msg, ok := cmdTicketMenu(tickettest.FakeReads{FakeServerAPI: api}, config.Dirs{}, ticketMenuInput{rendered: ticket.RawStatus(), ticket: ticket})().(TicketReadyMsg)
	if !ok {
		t.Fatal("no TicketReadyMsg")
	}
	if msg.DetailErr == nil || msg.Toast() == nil {
		t.Fatalf("DetailErr = %v, toast = %v; want both set", msg.DetailErr, msg.Toast())
	}
	for _, v := range []schema.PauseVerb{schema.VerbRetry, schema.VerbStartOver} {
		if slices.ContainsFunc(msg.Items, func(it contextmenu.Item[TicketTarget]) bool { return it.Label == v.Label() }) {
			t.Errorf("menu offers %q after a failed detail read", v.Label())
		}
	}
}

// The TUI trusts the server's ActionInfo; a word without one shows as itself.
func TestRecoveryActions_NoActionInfoFallsBackToTheWord(t *testing.T) {
	got := RecoveryActions(&wire.TicketDetail{Actions: []string{wire.ActionRetry}})
	if len(got) != 1 || got[0].Label != wire.ActionRetry || got[0].Help != "" {
		t.Fatalf("got %+v, want the bare word as label and no help", got)
	}
}

func TestRecoveryItems_ShowTheTooltipAsHelp(t *testing.T) {
	d := detailWith(wire.ActionRetry)
	it := itemByLabel(t, recoveryMenu(t, schema.StatusPaused, schema.PauseIterationError, d), schema.VerbRetry.Label())
	if it.Help == "" || it.Help != schema.VerbRetry.Tooltip() {
		t.Errorf("help = %q, want %q", it.Help, schema.VerbRetry.Tooltip())
	}
}

func TestRecoveryItems_MarkTheRecommendedVerb(t *testing.T) {
	d := detailWith(wire.ActionRetry, wire.ActionStartOver)
	d.Recommendation = &wire.DetailRecommendation{Verb: wire.ActionStartOver, Diagnosis: "bad branch"}
	items := recoveryMenu(t, schema.StatusPaused, schema.PauseIterationError, d)

	rec := itemByLabel(t, items, schema.VerbStartOver.Label()+RecommendedMark)
	if rec.Group != contextmenu.Suggested {
		t.Errorf("recommended group = %v, want Suggested", rec.Group)
	}
	itemByLabel(t, items, schema.VerbRetry.Label())
}
