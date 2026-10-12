package tickets

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/sessionlookup"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/notify"
)

func fakeSessionLookup(t *testing.T, sessions map[string]string) {
	t.Helper()
	prev := lookupSession
	t.Cleanup(func() { lookupSession = prev })
	lookupSession = func(_, id string) (sessionlookup.Session, error) {
		md, ok := sessions[id]
		if !ok {
			return sessionlookup.Session{Source: sessionlookup.Missing}, nil
		}
		return sessionlookup.Session{Source: sessionlookup.Claude, Markdown: md}, nil
	}
}

func writeTranscriptTicket(t *testing.T, sessionIDs string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "01-x.md")
	raw := "---\nid: 01\nstatus: done\ntype: implement\n" + sessionIDs + "---\n\n# 01 — x\n"
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSuggestedActionItems_TranscriptAlwaysListed(t *testing.T) {
	t.Parallel()
	statuses := []tickets.RenderedStatus{tickets.StatusOpen, tickets.StatusDone, tickets.StatusNeedsAnswer, tickets.StatusNeedsRepair, tickets.StatusError}
	for _, status := range statuses {
		for _, native := range []bool{false, true} {
			items := suggestedActionItems(status, tickets.Ticket{}, false, native, false)
			if !slices.Contains(itemValues(items), actionTranscript) {
				t.Errorf("status %v native %v: items = %v, want %q", status, native, itemValues(items), actionTranscript)
			}
		}
	}
	if ticketHasSuggestedActions(tickets.StatusDone, tickets.Ticket{}, false) {
		t.Error("a done ticket carries a badge, want none: Transcript alone must not add one")
	}
}

func TestTranscript_RendersLatestSessionInModal(t *testing.T) {
	fakeSessionLookup(t, map[string]string{"old": "# old run", "new": "# latest run\n\nhello from the agent"})
	path := writeTranscriptTicket(t, "session_ids:\n  - old\n  - new\n")

	loaded, ok := cmdTranscript(path, "gx:epic/01", 120)().(watchLoadedMsg)
	if !ok {
		t.Fatal("expected watchLoadedMsg")
	}
	m := watchModal{}.Open(loaded.title, loaded.lines, 120, 40)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "hello from the agent") || strings.Contains(view, "old run") {
		t.Errorf("modal does not show the latest session:\n%s", view)
	}
	if !m.vp.AtBottom() {
		t.Error("modal not scrolled to the bottom")
	}
}

func TestTranscript_NoSessionsToast(t *testing.T) {
	fakeSessionLookup(t, nil)
	path := writeTranscriptTicket(t, "")

	msg, ok := cmdTranscript(path, "gx:epic/01", 120)().(notify.NotifyMsg)
	if !ok || !strings.Contains(msg.Message, "no sessions") {
		t.Errorf("msg = %#v, want a \"no sessions\" toast", msg)
	}
}
