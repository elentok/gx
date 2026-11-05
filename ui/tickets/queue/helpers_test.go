package queue

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui/contextmenu"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
	"github.com/elentok/gx/ui/tickets/rowactions"
	"github.com/elentok/gx/viewmodel"
)

var (
	muted       = tickettest.Muted
	detailWith  = tickettest.DetailWith
	resumeLabel = schema.VerbResume.Label()
)

// pausedAnswerInfo and pausedRepairInfo are server tickets paused for each
// class: status paused, rendered as the class's word.
func pausedAnswerInfo(address, title string) wire.TicketInfo {
	return wire.TicketInfo{Address: address, Title: title, Status: "paused", RenderedStatus: tickets.StatusPausedAnswer.Word()}
}

func pausedRepairInfo(address, title string) wire.TicketInfo {
	return wire.TicketInfo{Address: address, Title: title, Status: "paused", RenderedStatus: tickets.StatusPausedRepair.Word()}
}

// withRenderedStatus mirrors a server snapshot: a ticket with no rendered status
// word reads as its raw status; a snapshot with none at all reads as an older
// server's.
func withRenderedStatus(snap wire.Snapshot) wire.Snapshot {
	if snap.Seq == 0 {
		snap.Seq = 1
	}
	snap.Tickets = slices.Clone(snap.Tickets)
	for i := range snap.Tickets {
		if snap.Tickets[i].RenderedStatus == "" {
			snap.Tickets[i].RenderedStatus = snap.Tickets[i].Status
		}
	}
	return snap
}

func applySnapshot(st viewmodel.State, snap wire.Snapshot) viewmodel.State {
	return st.ApplySnapshot(withRenderedStatus(snap))
}

// olderServerState is a snapshot from a server that predates rendered status.
func olderServerState() viewmodel.State {
	return viewmodel.State{}.ApplySnapshot(wire.Snapshot{Seq: 1, Tickets: []wire.TicketInfo{
		{Address: "gx:alpha/01", Title: "First", Status: "open"},
	}}).SetQueue([]wire.QueueItem{{Address: "gx:alpha/01"}})
}

func pressKeys(t *testing.T, m tea.Model, keys ...rune) tea.Model {
	t.Helper()
	for _, k := range keys {
		m, _ = m.Update(tea.KeyPressMsg{Code: k, Text: string(k)})
	}
	return m
}

// assertOnlyToast: a finished write only toasts, since the new state arrives on
// the event stream and the tab must not refetch.
func assertOnlyToast(t *testing.T, name string, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatalf("%s: no toast", name)
	}
	if _, ok := cmd().(notify.NotifyMsg); !ok {
		t.Errorf("%s: command is not a bare toast (a batch means a refetch rode along)", name)
	}
}

func fakeSessionLookup(t *testing.T, sessions map[string]string) {
	t.Helper()
	prev := rowactions.LookupSession
	t.Cleanup(func() { rowactions.LookupSession = prev })
	rowactions.LookupSession = tickettest.SessionLookup(sessions)
}

func orchestrationLabels() []string {
	return []string{"Unmute & Reopen", "Pause", "Cancel"}
}

func withMutes(info wire.TicketInfo, mutes []schema.MuteRecord) wire.TicketInfo {
	info.Mutes = mutes
	return info
}

func itemByLabel(t *testing.T, items []contextmenu.Item[rowactions.TicketTarget], label string) contextmenu.Item[rowactions.TicketTarget] {
	t.Helper()
	i := slices.IndexFunc(items, func(it contextmenu.Item[rowactions.TicketTarget]) bool { return it.Label == label })
	if i < 0 {
		t.Fatalf("no %q item", label)
	}
	return items[i]
}

func queueWithTicket(t *testing.T, m QueueModel, reason schema.PauseReason, mutes []schema.MuteRecord) QueueModel {
	t.Helper()
	m.rows.Epics[0].Tickets[0].Status = string(schema.StatusPaused)
	m.rows.Epics[0].Tickets[0].PauseReason = reason
	m.rows.Epics[0].Tickets[0].Mutes = mutes
	m.clampSelected()
	m = selectFirstQueueTicketRow(t, m)
	return m
}

// writeMenuTicketFile writes a paused (answer) ticket file and returns its path.
func writeMenuTicketFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "01-first.md")
	body := "---\nid: \"01\"\ntitle: First\nstatus: paused\npause_reason: blocked-pane\n---\n\n# A\n\n" + tickets.LegacyAnswerHeading + "\n\nWhich approach?\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
