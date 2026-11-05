package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/ui"
	ticketsui "github.com/elentok/gx/ui/tickets"
	"github.com/elentok/gx/viewmodel"
)

var terminalNow = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

func terminalSession(st wire.SessionStatus) wire.SessionInfo {
	return wire.SessionInfo{ID: "s1", Name: "fix login", Project: "gx", Dir: "/work/gx",
		Ticket: "gx:auth/03", Status: st, StatusChangedAt: terminalNow.Add(-12 * time.Minute)}
}

// terminalModel shows s alone with the sidebar hidden, running every command
// the update returns so the last screen loads.
func terminalModel(t *testing.T, s wire.SessionInfo, sessionsDir string) Model {
	t.Helper()
	m := newTestModel(&fakeSessionClient{}, ui.Settings{UseNerdFontIcons: true})
	m.now = func() time.Time { return terminalNow }
	m.sessionsDir = sessionsDir
	m.hideSidebar = true
	m = update1(t, m, tea.WindowSizeMsg{Width: 80, Height: 8})
	next, cmd := m.WithServerState(&viewmodel.State{Sessions: []wire.SessionInfo{s}}, ticketsui.ServerLinkUp)
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
	if err := os.WriteFile(filepath.Join(sessionsDir, "s1", holder.LastScreenFile), []byte(screen), 0o644); err != nil {
		t.Fatal(err)
	}
	exited := terminalSession(wire.SessionStatus{State: wire.SessionExited})
	exited.Exit = wire.SessionExitClean
	crashed := terminalSession(wire.SessionStatus{State: wire.SessionExited})
	crashed.Exit = wire.SessionExitCrashed
	crashed.Actionable = wire.SessionActionable{Reason: "crashed"}
	actionable := terminalSession(wire.SessionStatus{State: wire.SessionIdle})
	actionable.Actionable = wire.SessionActionable{Reason: "tests failed", Action: []string{"/fix", "run the tests"}}

	for name, s := range map[string]wire.SessionInfo{
		"terminal_starting":       terminalSession(wire.SessionStatus{State: wire.SessionStarting}),
		"terminal_working_bg":     terminalSession(wire.SessionStatus{State: wire.SessionWorking, Background: true}),
		"terminal_idle":           terminalSession(wire.SessionStatus{State: wire.SessionIdle}),
		"terminal_blocked_detail": terminalSession(wire.SessionStatus{State: wire.SessionBlocked, Kind: "permission", Message: "Bash: rm -rf build"}),
		"terminal_rate_limited":   terminalSession(wire.SessionStatus{State: wire.SessionRateLimited, Message: "resets 3pm"}),
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
	m := terminalModel(t, terminalSession(wire.SessionStatus{State: wire.SessionBlocked}), "")
	header := strings.SplitN(ansi.Strip(m.View().Content), "\n", 2)[0]
	for _, want := range []string{"fix login", "blocked", "gx:auth/03", "gx", "/work/gx", "for 12m"} {
		if !strings.Contains(header, want) {
			t.Errorf("header %q lacks %q", header, want)
		}
	}
}

func TestTerminal_ActionableReasonWinsOverStatusDetail(t *testing.T) {
	s := terminalSession(wire.SessionStatus{State: wire.SessionBlocked, Kind: "permission", Message: "Bash"})
	s.Actionable = wire.SessionActionable{Reason: "needs approval", Action: []string{"yes"}}
	got := ansi.Strip(infoRow(s, time.Now()))
	if got != "needs approval  <leader> a: yes" {
		t.Fatalf("info row = %q", got)
	}
}

func TestTerminal_InfoRowShowsTheCountdownAndCancel(t *testing.T) {
	now := time.Now()
	at := now.Add(7 * time.Second)
	s := terminalSession(idleStatus)
	s.Actionable = wire.SessionActionable{Reason: "waiting", Action: []string{"Continue"}}
	s.AutoAdvanceAt = &at
	got := ansi.Strip(infoRow(s, now))
	for _, want := range []string{"Continuing in 7s", "Cancel"} {
		if !strings.Contains(got, want) {
			t.Errorf("info row %q lacks %q", got, want)
		}
	}
}

func TestTerminal_ExitedScreenReloadsAfterALaterExit(t *testing.T) {
	sessionsDir := t.TempDir()
	path := filepath.Join(sessionsDir, "s1", holder.LastScreenFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("first\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := terminalSession(wire.SessionStatus{State: wire.SessionExited})
	m := terminalModel(t, s, sessionsDir)
	if err := os.WriteFile(path, []byte("second\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.StatusChangedAt = terminalNow
	next, cmd := m.WithServerState(&viewmodel.State{Sessions: []wire.SessionInfo{s}}, ticketsui.ServerLinkUp)
	m = runBatch(t, next.(Model), cmd)
	if body := ansi.Strip(m.View().Content); !strings.Contains(body, "second") {
		t.Fatalf("last screen not reloaded:\n%s", body)
	}
}
