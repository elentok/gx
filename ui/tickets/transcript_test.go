package tickets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/sessionlookup"
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

func TestTranscript_RendersLatestSessionInModal(t *testing.T) {
	fakeSessionLookup(t, map[string]string{"old": "# old run", "new": "# latest run\n\nhello from the agent"})
	path := writeTranscriptTicket(t, "session_ids:\n  - old\n  - new\n")

	loaded, ok := cmdTranscript(path, "gx:epic/01", 120)().(watchLoadedMsg)
	if !ok {
		t.Fatal("expected watchLoadedMsg")
	}
	m := watchModal{}.Open(loaded, 120, 40)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "hello from the agent") || strings.Contains(view, "old run") {
		t.Errorf("modal does not show the latest session:\n%s", view)
	}
	if !m.vp.AtBottom() {
		t.Error("modal not scrolled to the bottom")
	}
}

func TestTranscript_StepsBetweenSessions(t *testing.T) {
	fakeSessionLookup(t, map[string]string{"s1": "first run", "s2": "second run", "s3": "third run"})
	path := writeTranscriptTicket(t, "session_ids:\n  - s1\n  - s2\n  - s3\n")

	open := func(cmd tea.Cmd) watchModal {
		t.Helper()
		loaded, ok := cmd().(watchLoadedMsg)
		if !ok {
			t.Fatal("expected watchLoadedMsg")
		}
		return watchModal{}.Open(loaded, 120, 40)
	}
	press := func(m watchModal, key string) (watchModal, tea.Cmd) {
		return m.Update(tea.KeyPressMsg{Code: rune(key[0]), Text: key})
	}
	assertShows := func(m watchModal, body, pos string) {
		t.Helper()
		view := ansi.Strip(m.View())
		if !strings.Contains(view, body) || !strings.Contains(view, pos) {
			t.Errorf("modal does not show %q at %q:\n%s", body, pos, view)
		}
	}

	m := open(cmdTranscript(path, "gx:epic/01", 120))
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
	path := writeTranscriptTicket(t, "")

	msg, ok := cmdTranscript(path, "gx:epic/01", 120)().(notify.NotifyMsg)
	if !ok || !strings.Contains(msg.Message, "no sessions") {
		t.Errorf("msg = %#v, want a \"no sessions\" toast", msg)
	}
}
