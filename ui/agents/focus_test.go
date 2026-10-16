package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/ui"
	ticketsui "github.com/elentok/gx/ui/tickets"
	"github.com/elentok/gx/viewmodel"
)

// openedModel opens the tab on sessions, remembering lastViewed (none when
// empty) in a temp file.
func openedModel(t *testing.T, f *fakeSessionClient, lastViewed string, sessions ...server.SessionInfo) Model {
	t.Helper()
	path := filepath.Join(t.TempDir(), "last-viewed")
	if lastViewed != "" {
		if err := os.WriteFile(path, []byte(lastViewed+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := NewModel(f, ui.Settings{}).WithLastViewedFile(path)
	// Runs before TempDir's cleanup: a late save would race its removal.
	t.Cleanup(func() { waitIdle(t, m.reporter) })
	m.sessionsDir = "/sessions"
	m.dial = (&fakeHolders{attaches: map[string]*fakeAttach{}}).dial
	m.termProgram = "iTerm.app"
	m = update1(t, m, tea.WindowSizeMsg{Width: 80, Height: 12})
	return withState(t, m, &viewmodel.State{Sessions: sessions}, ticketsui.ServerLinkUp)
}

func waitIdle(t *testing.T, r *focusReporter) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		r.mu.Lock()
		sending := r.sending
		r.mu.Unlock()
		if !sending {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("focus reporter never went idle")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func attentionSessions() []server.SessionInfo {
	return []server.SessionInfo{
		{ID: "a", Project: "gx", Status: idleStatus},
		{ID: "b", Project: "gx", Status: idleStatus, Unread: server.SessionUnread{Unread: true}},
		{ID: "c", Project: "gx", Status: server.SessionStatus{State: server.SessionBlocked}},
	}
}

func assertFocus(t *testing.T, m Model, terminal bool, session string) {
	t.Helper()
	if m.terminalFocused != terminal || m.selectedID() != session {
		t.Fatalf("terminal focused %v on %q, want %v on %q", m.terminalFocused, m.selectedID(), terminal, session)
	}
}

func TestFocus_OpensOnTheSessionViewedLast(t *testing.T) {
	m := openedModel(t, &fakeSessionClient{}, "a", attentionSessions()...)
	assertFocus(t, m, true, "a")
}

func TestFocus_OpensOnTheFirstSessionNeedingAttention(t *testing.T) {
	for name, lastViewed := range map[string]string{"none viewed": "", "viewed one is gone": "zz"} {
		t.Run(name, func(t *testing.T) {
			m := openedModel(t, &fakeSessionClient{}, lastViewed, attentionSessions()...)
			assertFocus(t, m, true, "c")
		})
	}
}

func TestFocus_OpensOnUnreadWithNothingActionable(t *testing.T) {
	m := openedModel(t, &fakeSessionClient{}, "", attentionSessions()[:2]...)
	assertFocus(t, m, true, "b")
}

func TestFocus_OpensOnTheSidebarWithNothingNeedingAttention(t *testing.T) {
	m := openedModel(t, &fakeSessionClient{}, "", attentionSessions()[0])
	assertFocus(t, m, false, "a")
}

func TestFocus_OpensOnTheSidebarHintWithNoSessions(t *testing.T) {
	m := openedModel(t, &fakeSessionClient{}, "")
	assertFocus(t, m, false, "")
	if !strings.Contains(plainView(m), "c new session") {
		t.Fatalf("no new-session hint:\n%s", plainView(m))
	}
}

func TestFocus_LaterSnapshotsKeepTheFocus(t *testing.T) {
	m := openedModel(t, &fakeSessionClient{}, "", attentionSessions()[0])
	m = withState(t, m, &viewmodel.State{Sessions: attentionSessions()}, ticketsui.ServerLinkUp)
	assertFocus(t, m, false, "a")
}

func leaveAndReturn(t *testing.T, m Model) Model {
	t.Helper()
	m.OnPageDeactivated()
	return update1(t, m, m.OnPageActivated()())
}

func TestFocus_TabMemoryKeepsFocusAndSession(t *testing.T) {
	t.Run("terminal", func(t *testing.T) {
		m := openedModel(t, &fakeSessionClient{}, "b", attentionSessions()...)
		assertFocus(t, leaveAndReturn(t, m), true, "b")
	})
	t.Run("sidebar", func(t *testing.T) {
		m := openedModel(t, &fakeSessionClient{}, "", attentionSessions()[0], attentionSessions()[1])
		m = m.toggleFocus()
		m = press(t, m, "k")
		assertFocus(t, leaveAndReturn(t, m), false, "a")
	})
}

func TestFocus_ReportsFocusBlurSessionAndTab(t *testing.T) {
	f := &fakeSessionClient{}
	m := openedModel(t, f, "", attentionSessions()[0], attentionSessions()[1])
	report := func(focused bool, session string) server.FocusReport {
		return server.FocusReport{Client: FocusClient, Focused: focused, Session: session, TermProgram: "iTerm.app"}
	}
	f.waitReport(t, report(true, "b"))

	m = update1(t, m, tea.BlurMsg{})
	f.waitReport(t, report(false, "b"))

	m = update1(t, m, tea.FocusMsg{})
	f.waitReport(t, report(true, "b"))

	m = m.toggleFocus()
	m = press(t, m, "k")
	f.waitReport(t, report(true, "a"))

	m.OnPageDeactivated()
	f.waitReport(t, report(true, ""))

	update1(t, m, m.OnPageActivated()())
	f.waitReport(t, report(true, "a"))
}

func TestFocus_RefusedReportIsResent(t *testing.T) {
	f := &fakeSessionClient{focusResult: server.SessionResult{Refused: true}}
	m := openedModel(t, f, "", attentionSessions()[0])
	want := server.FocusReport{Client: FocusClient, Focused: true, Session: "a", TermProgram: "iTerm.app"}
	f.waitReport(t, want)

	f.mu.Lock()
	f.focusResult, f.reports = server.SessionResult{}, nil
	f.mu.Unlock()
	// The refusal is forgotten off the update loop; any later update resends.
	deadline := time.Now().Add(2 * time.Second)
	for {
		update1(t, m, tea.WindowSizeMsg{Width: 80, Height: 12})
		f.mu.Lock()
		n := len(f.reports)
		f.mu.Unlock()
		if n > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("refused report was never resent")
		}
		time.Sleep(5 * time.Millisecond)
	}
	f.waitReport(t, want)
}

func TestFocus_SavesTheSessionOnScreen(t *testing.T) {
	f := &fakeSessionClient{}
	m := openedModel(t, f, "", attentionSessions()[0], attentionSessions()[1])
	m = m.toggleFocus()
	press(t, m, "k")
	f.waitReport(t, server.FocusReport{Client: FocusClient, Focused: true, Session: "a", TermProgram: "iTerm.app"})

	waitIdle(t, m.reporter)
	m.reporter.mu.Lock()
	path := m.reporter.lastViewedPath
	m.reporter.mu.Unlock()
	if got := NewModel(nil, ui.Settings{}).WithLastViewedFile(path).lastViewed; got != "a" {
		t.Fatalf("saved last viewed %q, want a", got)
	}
}

func TestFocus_ViewEnablesFocusReporting(t *testing.T) {
	if !NewModel(nil, ui.Settings{}).View().ReportFocus {
		t.Fatal("focus reporting off: focus and blur would never arrive")
	}
}
