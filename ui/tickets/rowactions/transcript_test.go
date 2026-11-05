package rowactions

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
)

func TestTranscript_RendersLatestSessionInModal(t *testing.T) {
	prev := LookupSession
	t.Cleanup(func() { LookupSession = prev })
	LookupSession = tickettest.SessionLookup(map[string]string{"old": "# old run", "new": "# latest run\n\nhello from the agent"})
	path := tickettest.WriteTranscriptTicket(t, "session_ids:\n  - old\n  - new\n")

	loaded, ok := CmdTranscript(config.Dirs{}, path, "gx:epic/01", 120)().(WatchLoadedMsg)
	if !ok {
		t.Fatal("expected WatchLoadedMsg")
	}
	m := WatchModal{}.Open(loaded, 120, 40)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "hello from the agent") || strings.Contains(view, "old run") {
		t.Errorf("modal does not show the latest session:\n%s", view)
	}
	if !m.vp.AtBottom() {
		t.Error("modal not scrolled to the bottom")
	}
}

func fakeSessionLookup(t *testing.T, sessions map[string]string) {
	t.Helper()
	prev := LookupSession
	t.Cleanup(func() { LookupSession = prev })
	LookupSession = tickettest.SessionLookup(sessions)
}

func TestTranscript_StepsBetweenSessions(t *testing.T) {
	fakeSessionLookup(t, map[string]string{"s1": "first run", "s2": "second run", "s3": "third run"})
	path := tickettest.WriteTranscriptTicket(t, "session_ids:\n  - s1\n  - s2\n  - s3\n")

	open := func(cmd tea.Cmd) WatchModal {
		t.Helper()
		loaded, ok := cmd().(WatchLoadedMsg)
		if !ok {
			t.Fatal("expected WatchLoadedMsg")
		}
		return WatchModal{}.Open(loaded, 120, 40)
	}
	press := func(m WatchModal, key string) (WatchModal, tea.Cmd) {
		return m.Update(tea.KeyPressMsg{Code: rune(key[0]), Text: key})
	}
	assertShows := func(m WatchModal, body, pos string) {
		t.Helper()
		view := ansi.Strip(m.View())
		if !strings.Contains(view, body) || !strings.Contains(view, pos) {
			t.Errorf("modal does not show %q at %q:\n%s", body, pos, view)
		}
	}

	m := open(CmdTranscript(config.Dirs{}, path, "gx:epic/01", 120))
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

	msg, ok := CmdTranscript(config.Dirs{}, path, "gx:epic/01", 120)().(notify.NotifyMsg)
	if !ok || !strings.Contains(msg.Message, "no sessions") {
		t.Errorf("msg = %#v, want a \"no sessions\" toast", msg)
	}
}
