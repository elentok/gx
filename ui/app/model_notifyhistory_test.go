package app

import (
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/nav"

	tea "charm.land/bubbletea/v2"
)

func TestGNChordOpensNotifyHistory(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = updated.(Model)

	if !m.notifyHistory.IsOpen {
		t.Fatal("expected notifyHistory.IsOpen=true after g n")
	}
}

func TestGNChordDoesNotOpenNotifyHistoryWhenModalOpen(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})

	stub := &modalOpenStub{}
	live := m.livePageByTab[nav.TabStatus]
	live.model = stub
	m.livePageByTab[nav.TabStatus] = live

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = updated.(Model)

	if m.notifyHistory.IsOpen {
		t.Fatal("expected notifyHistory to stay closed when the active page has its own modal open")
	}
}

// tickRecorderStub records every msg it receives and returns a fixed cmd, so
// tests can assert a tick-like message reaches the active page (and its
// follow-up cmd survives) even while an overlay is open.
type tickRecorderStub struct {
	received []tea.Msg
	cmd      tea.Cmd
}

func (s *tickRecorderStub) Init() tea.Cmd { return nil }
func (s *tickRecorderStub) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	s.received = append(s.received, msg)
	return s, s.cmd
}
func (s *tickRecorderStub) View() tea.View { return tea.NewView("stub") }

type fakeTickMsg struct{}
type fakeTickCmdMsg struct{}

func TestNotifyHistoryOpenFallsThroughNonKeyMouseMessages(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})

	stub := &tickRecorderStub{cmd: func() tea.Msg { return fakeTickCmdMsg{} }}
	live := m.livePageByTab[nav.TabStatus]
	live.model = stub
	m.livePageByTab[nav.TabStatus] = live

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = updated.(Model)
	if !m.notifyHistory.IsOpen {
		t.Fatal("expected notifyHistory.IsOpen=true after g n")
	}

	updated, cmd := m.Update(fakeTickMsg{})
	m = updated.(Model)

	if len(stub.received) != 1 || stub.received[0] != (fakeTickMsg{}) {
		t.Fatalf("expected tick msg to reach the active page while modal open, got %#v", stub.received)
	}
	if cmd == nil {
		t.Fatal("expected the active page's follow-up cmd to survive while modal open")
	}
	if msg := cmd(); msg != (fakeTickCmdMsg{}) {
		t.Fatalf("cmd() = %#v, want fakeTickCmdMsg{}", msg)
	}
	if !m.notifyHistory.IsOpen {
		t.Fatal("expected notifyHistory to remain open after an unrelated message")
	}
}

func TestNotifyHistoryOpenStillUpdatesWindowSize(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = updated.(Model)
	if !m.notifyHistory.IsOpen {
		t.Fatal("expected notifyHistory.IsOpen=true after g n")
	}

	updated, _ = m.Update(tea.WindowSizeMsg{Width: 111, Height: 33})
	m = updated.(Model)

	if m.width != 111 || m.height != 33 {
		t.Fatalf("width/height = %d/%d, want 111/33", m.width, m.height)
	}
	if !m.notifyHistory.IsOpen {
		t.Fatal("expected notifyHistory to remain open after a resize")
	}
}

func TestNotifyHistoryOpenStillInterceptsMouse(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})

	stub := &mouseRecorderStub{}
	live := m.livePageByTab[nav.TabStatus]
	live.model = stub
	m.livePageByTab[nav.TabStatus] = live

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = updated.(Model)
	if !m.notifyHistory.IsOpen {
		t.Fatal("expected notifyHistory.IsOpen=true after g n")
	}

	updated, _ = m.Update(tea.MouseClickMsg{X: 0, Y: 0, Button: tea.MouseLeft})
	m = updated.(Model)

	if len(stub.received) != 0 {
		t.Fatalf("expected mouse click to be intercepted while modal open, got %#v", stub.received)
	}
	if !m.notifyHistory.IsOpen {
		t.Fatal("expected notifyHistory to remain open")
	}
}

func TestNotifyHistoryEscClosesModal(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = updated.(Model)
	if !m.notifyHistory.IsOpen {
		t.Fatal("expected notifyHistory.IsOpen=true after g n")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(Model)
	if m.notifyHistory.IsOpen {
		t.Fatal("expected notifyHistory.IsOpen=false after esc")
	}
}
