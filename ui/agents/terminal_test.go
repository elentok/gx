package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/ui"
	ticketsui "github.com/elentok/gx/ui/tickets"
	"github.com/elentok/gx/viewmodel"
)

var terminalNow = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

func terminalSession(st server.SessionStatus) server.SessionInfo {
	return server.SessionInfo{ID: "s1", Name: "fix login", Project: "gx", Dir: "/work/gx",
		Ticket: "gx:auth/03", Status: st, StatusChangedAt: terminalNow.Add(-12 * time.Minute)}
}

// terminalModel shows s alone with the sidebar hidden, running every command
// the update returns so the last screen loads.
func terminalModel(t *testing.T, s server.SessionInfo, sessionsDir string) Model {
	t.Helper()
	m := NewModel(&fakeSessionClient{}, ui.Settings{UseNerdFontIcons: true})
	m.now = func() time.Time { return terminalNow }
	m.sessionsDir = sessionsDir
	m.hideSidebar = true
	m = update1(t, m, tea.WindowSizeMsg{Width: 80, Height: 8})
	next, cmd := m.WithServerState(&viewmodel.State{Sessions: []server.SessionInfo{s}}, ticketsui.ServerLinkUp)
	m = next.(Model)
	if cmd != nil {
		m = runBatch(t, m, cmd)
	}
	return m
}

func runBatch(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			if c != nil {
				m = runBatch(t, m, c)
			}
		}
	case lastScreenMsg:
		m = update1(t, m, msg)
	}
	return m
}

func TestTerminal_Golden(t *testing.T) {
	sessionsDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(sessionsDir, "s1"), 0o700); err != nil {
		t.Fatal(err)
	}
	screen := "\x1b[31m$ claude\x1b[0m\nbye\n"
	if err := os.WriteFile(filepath.Join(sessionsDir, "s1", nativerunner.LastScreenFile), []byte(screen), 0o644); err != nil {
		t.Fatal(err)
	}
	exited := terminalSession(server.SessionStatus{State: server.SessionExited})
	exited.Exit = server.SessionExitClean
	crashed := terminalSession(server.SessionStatus{State: server.SessionExited})
	crashed.Exit = server.SessionExitCrashed
	crashed.Actionable = server.SessionActionable{Reason: "crashed"}
	actionable := terminalSession(server.SessionStatus{State: server.SessionIdle})
	actionable.Actionable = server.SessionActionable{Reason: "tests failed", Action: []string{"/fix", "run the tests"}}

	for name, s := range map[string]server.SessionInfo{
		"terminal_starting":       terminalSession(server.SessionStatus{State: server.SessionStarting}),
		"terminal_working_bg":     terminalSession(server.SessionStatus{State: server.SessionWorking, Background: true}),
		"terminal_idle":           terminalSession(server.SessionStatus{State: server.SessionIdle}),
		"terminal_blocked_detail": terminalSession(server.SessionStatus{State: server.SessionBlocked, Kind: "permission", Message: "Bash: rm -rf build"}),
		"terminal_rate_limited":   terminalSession(server.SessionStatus{State: server.SessionRateLimited, Message: "resets 3pm"}),
		"terminal_actionable":     actionable,
		"terminal_crashed_reason": crashed,
		"terminal_exited_screen":  exited,
	} {
		t.Run(name, func(t *testing.T) {
			assertGolden(t, name, terminalModel(t, s, sessionsDir).View().Content)
		})
	}
}

func TestTerminal_HeaderShowsTimeSinceTheLastStatusChange(t *testing.T) {
	m := terminalModel(t, terminalSession(server.SessionStatus{State: server.SessionBlocked}), "")
	header := strings.SplitN(ansi.Strip(m.View().Content), "\n", 2)[0]
	for _, want := range []string{"fix login", "blocked", "gx:auth/03", "gx", "/work/gx", "for 12m"} {
		if !strings.Contains(header, want) {
			t.Errorf("header %q lacks %q", header, want)
		}
	}
}

func TestTerminal_ActionableReasonWinsOverStatusDetail(t *testing.T) {
	s := terminalSession(server.SessionStatus{State: server.SessionBlocked, Kind: "permission", Message: "Bash"})
	s.Actionable = server.SessionActionable{Reason: "needs approval", Action: []string{"yes"}}
	got := ansi.Strip(infoRow(s))
	if got != "needs approval  <leader> a: yes" {
		t.Fatalf("info row = %q", got)
	}
}

func TestTerminal_ExitedScreenReloadsAfterALaterExit(t *testing.T) {
	sessionsDir := t.TempDir()
	path := filepath.Join(sessionsDir, "s1", nativerunner.LastScreenFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("first\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := terminalSession(server.SessionStatus{State: server.SessionExited})
	m := terminalModel(t, s, sessionsDir)
	if err := os.WriteFile(path, []byte("second\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.StatusChangedAt = terminalNow
	next, cmd := m.WithServerState(&viewmodel.State{Sessions: []server.SessionInfo{s}}, ticketsui.ServerLinkUp)
	m = runBatch(t, next.(Model), cmd)
	if body := ansi.Strip(m.View().Content); !strings.Contains(body, "second") {
		t.Fatalf("last screen not reloaded:\n%s", body)
	}
}
