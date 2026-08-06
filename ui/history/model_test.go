package history

import (
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/claudehistory"
)

var fixtureProjects = []claudehistory.Project{
	{Dir: "gx-main", Cwd: "/dev/gx/main", Label: "gx", Subtitle: "~/dev/gx/main"},
	{Dir: "blf", Cwd: "/dev/blf", Label: "blf", Subtitle: "~/dev/blf"},
}

var fixtureConversations = map[string][]claudehistory.Conversation{
	"gx-main": {
		{SessionID: "sess-101", Title: "Split queue.go", LastAccessed: time.Now()},
		{SessionID: "sess-102", Title: "Dedupe handler", LastAccessed: time.Now()},
	},
	"blf": {
		{SessionID: "sess-201", Title: "Port statusline", LastAccessed: time.Now()},
	},
}

func fixtureProjectLoader(_ string) ([]claudehistory.Project, error) {
	return fixtureProjects, nil
}

func fixtureConversationLoader(dir string) ([]claudehistory.Conversation, error) {
	return fixtureConversations[dir], nil
}

func newTestModel() Model {
	m := NewModel("", fixtureProjectLoader, fixtureConversationLoader)
	m, _ = update(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = update(m, projectsLoadedMsg{projects: fixtureProjects})
	return m
}

func update(m Model, msg tea.Msg) (Model, tea.Cmd) {
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

func press(m Model, key string) (Model, tea.Cmd) {
	return update(m, tea.KeyPressMsg{Code: rune(key[0]), Text: key})
}

func TestInit_LoadsProjects(t *testing.T) {
	m := NewModel("", fixtureProjectLoader, fixtureConversationLoader)
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init() returned nil cmd, want a load-projects cmd")
	}
	msg := cmd()
	loaded, ok := msg.(projectsLoadedMsg)
	if !ok {
		t.Fatalf("Init() cmd produced %T, want projectsLoadedMsg", msg)
	}
	if len(loaded.projects) != len(fixtureProjects) {
		t.Fatalf("loaded %d projects, want %d", len(loaded.projects), len(fixtureProjects))
	}
}

func TestInit_LoadError(t *testing.T) {
	wantErr := errors.New("boom")
	m := NewModel("", func(string) ([]claudehistory.Project, error) { return nil, wantErr }, fixtureConversationLoader)
	m, _ = update(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = update(m, projectsLoadedMsg{err: wantErr})
	if m.projErr == nil {
		t.Fatal("expected projErr to be set")
	}
}

func TestEnter_OpensConversations(t *testing.T) {
	m := newTestModel()
	m, cmd := update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.page != pageConversations {
		t.Fatalf("page = %v, want pageConversations", m.page)
	}
	if m.convProjectDir != "gx-main" {
		t.Fatalf("convProjectDir = %q, want gx-main", m.convProjectDir)
	}
	if cmd == nil {
		t.Fatal("expected a load-conversations cmd after enter")
	}
	msg := cmd()
	loaded, ok := msg.(conversationsLoadedMsg)
	if !ok {
		t.Fatalf("cmd produced %T, want conversationsLoadedMsg", msg)
	}
	m, _ = update(m, loaded)
	if len(m.conversations) != 2 {
		t.Fatalf("loaded %d conversations, want 2", len(m.conversations))
	}
}

func enterConversationsFixture(t *testing.T) Model {
	t.Helper()
	m := newTestModel()
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = update(m, conversationsLoadedMsg{conversations: fixtureConversations["gx-main"]})
	return m
}

func TestEsc_ReturnsToProjects(t *testing.T) {
	m := enterConversationsFixture(t)
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.page != pageProjects {
		t.Fatalf("page = %v, want pageProjects after esc", m.page)
	}
}

func TestEsc_ClearsFilterBeforeGoingBack(t *testing.T) {
	m := enterConversationsFixture(t)
	m, _ = press(m, "/")
	m, _ = press(m, "d")
	m, _ = press(m, "e")
	if !m.convFilter.IsActive() {
		t.Fatal("expected filter to be active after typing")
	}
	if len(m.filteredConversations()) != 1 {
		t.Fatalf("filtered %d conversations, want 1", len(m.filteredConversations()))
	}

	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.convFilter.IsActive() {
		t.Fatal("expected first esc to clear the filter, not navigate back")
	}
	if m.page != pageConversations {
		t.Fatal("expected first esc to stay on conversations page")
	}

	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.page != pageProjects {
		t.Fatal("expected second esc to return to projects page")
	}
}

func TestFilter_ProjectsNarrowsList(t *testing.T) {
	m := newTestModel()
	m, _ = press(m, "/")
	if !m.projFilter.InputFocused() {
		t.Fatal("expected filter input to be focused after '/'")
	}
	m, _ = press(m, "b")
	m, _ = press(m, "l")
	m, _ = press(m, "f")
	filtered := m.filteredProjects()
	if len(filtered) != 1 || filtered[0].Label != "blf" {
		t.Fatalf("filteredProjects = %+v, want just blf", filtered)
	}

	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.projFilter.IsActive() {
		t.Fatal("expected esc to clear the projects filter")
	}
	if len(m.filteredProjects()) != len(fixtureProjects) {
		t.Fatal("expected clearing the filter to restore the full projects list")
	}
}

func TestNavigate_JKMoveSelection(t *testing.T) {
	m := newTestModel()
	if m.projList.Selected() != 0 {
		t.Fatalf("initial selection = %d, want 0", m.projList.Selected())
	}
	m, _ = press(m, "j")
	if m.projList.Selected() != 1 {
		t.Fatalf("selection after j = %d, want 1", m.projList.Selected())
	}
	// Boundary: further j past the last item stays clamped.
	m, _ = press(m, "j")
	if m.projList.Selected() != 1 {
		t.Fatalf("selection after second j = %d, want clamped at 1", m.projList.Selected())
	}
	m, _ = press(m, "k")
	if m.projList.Selected() != 0 {
		t.Fatalf("selection after k = %d, want 0", m.projList.Selected())
	}
	// Boundary: k at the first item stays clamped.
	m, _ = press(m, "k")
	if m.projList.Selected() != 0 {
		t.Fatalf("selection after second k = %d, want clamped at 0", m.projList.Selected())
	}
}

func TestNavigate_CtrlDCtrlUAtBoundaries(t *testing.T) {
	m := newTestModel()
	before := m.projList.Selected()
	m, _ = update(m, tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	if m.projList.Selected() != before {
		t.Fatalf("ctrl+u at top should no-op, got selection %d", m.projList.Selected())
	}

	m, _ = update(m, tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	if m.projList.Selected() != len(fixtureProjects)-1 {
		t.Fatalf("ctrl+d should move to the last item, got %d", m.projList.Selected())
	}
	// Boundary: ctrl+d past the last item stays clamped.
	m, _ = update(m, tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	if m.projList.Selected() != len(fixtureProjects)-1 {
		t.Fatalf("ctrl+d at bottom should no-op, got %d", m.projList.Selected())
	}
}

func TestQuit_OnProjectsPage(t *testing.T) {
	m := newTestModel()
	_, cmd := press(m, "q")
	if cmd == nil {
		t.Fatal("expected q to produce tea.Quit cmd")
	}
}
