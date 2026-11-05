package tickets

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
	"github.com/elentok/gx/ui/tickets/rowactions"
)

func fakeSessionLookup(t *testing.T, sessions map[string]string) {
	t.Helper()
	prev := rowactions.LookupSession
	t.Cleanup(func() { rowactions.LookupSession = prev })
	rowactions.LookupSession = tickettest.SessionLookup(sessions)
}

func TestTranscript_StepsBetweenSessions(t *testing.T) {
	fakeSessionLookup(t, map[string]string{"s1": "first run", "s2": "second run", "s3": "third run"})
	path := tickettest.WriteTranscriptTicket(t, "session_ids:\n  - s1\n  - s2\n  - s3\n")

	open := func(cmd tea.Cmd) rowactions.WatchModal {
		t.Helper()
		loaded, ok := cmd().(rowactions.WatchLoadedMsg)
		if !ok {
			t.Fatal("expected WatchLoadedMsg")
		}
		return rowactions.WatchModal{}.Open(loaded, 120, 40)
	}
	press := func(m rowactions.WatchModal, key string) (rowactions.WatchModal, tea.Cmd) {
		return m.Update(tea.KeyPressMsg{Code: rune(key[0]), Text: key})
	}
	assertShows := func(m rowactions.WatchModal, body, pos string) {
		t.Helper()
		view := ansi.Strip(m.View())
		if !strings.Contains(view, body) || !strings.Contains(view, pos) {
			t.Errorf("modal does not show %q at %q:\n%s", body, pos, view)
		}
	}

	m := open(rowactions.CmdTranscript(config.Dirs{}, path, "gx:epic/01", 120))
	assertShows(m, "third run", "session 3/3")
	if _, cmd := press(m, "]"); cmd != nil {
		t.Error("] on the newest session stepped, want a no-op")
	}

	_, cmd := press(m, "[")
	m = open(cmd)
	assertShows(m, "second run", "session 2/3")
	_, cmd = press(m, "[")
	m = open(cmd)
	assertShows(m, "first run", "session 1/3")
	if _, cmd := press(m, "["); cmd != nil {
		t.Error("[ on the oldest session stepped, want a no-op")
	}

	_, cmd = press(m, "]")
	m = open(cmd)
	assertShows(m, "second run", "session 2/3")
}

func TestTranscript_NoSessionsToast(t *testing.T) {
	fakeSessionLookup(t, nil)
	path := tickettest.WriteTranscriptTicket(t, "")

	msg, ok := rowactions.CmdTranscript(config.Dirs{}, path, "gx:epic/01", 120)().(notify.NotifyMsg)
	if !ok || !strings.Contains(msg.Message, "no sessions") {
		t.Errorf("msg = %#v, want a \"no sessions\" toast", msg)
	}
}
