package agents

import (
	"strings"
	"testing"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/ui/agents/create"
	"github.com/elentok/gx/ui/tickets/tabapi"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/viewmodel"
)

var (
	keyEnterMsg = tea.KeyPressMsg{Code: tea.KeyEnter}
	keyDownMsg  = tea.KeyPressMsg{Code: tea.KeyDown}
)

func createSessions() []wire.SessionInfo {
	return []wire.SessionInfo{
		{ID: "w1", Project: "web", Dir: "/r/web", Status: idleStatus},
		{ID: "g1", Project: "gx", Dir: "/r/gx/wt1", Status: idleStatus},
		{ID: "g2", Project: "gx", Dir: "/r/gx/wt1", Status: idleStatus},
		{ID: "g3", Project: "gx", Dir: "/r/gx/wt2", Status: wire.SessionStatus{State: wire.SessionExited}},
	}
}

func createProjects() ([]create.Project, error) {
	return []create.Project{
		{Name: "web", Root: "/r/web"},
		{Name: "gx", Root: "/r/gx", Worktrees: []string{"/r/gx/wt1", "/r/gx/wt2"}},
	}, nil
}

// createModel has the cursor on session g1, in project gx.
func createModel(t *testing.T, f *fakeSessionClient) Model {
	t.Helper()
	m := openedModel(t, f, "", createSessions()...)
	m.listProjects = createProjects
	m = update1(t, m, tea.WindowSizeMsg{Width: 80, Height: 30})
	m.terminalFocused = false
	m.cursor = 1
	return m
}

// step sends msg and feeds back the message its cmd produces.
func step(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	next, cmd := m.Update(msg)
	m = next.(Model)
	if cmd != nil {
		m = update1(t, m, cmd())
	}
	return m
}

func typeKeys(t *testing.T, m Model, keys ...tea.KeyPressMsg) Model {
	t.Helper()
	for _, k := range keys {
		m = update1(t, m, k)
	}
	return m
}

func lastCreate(t *testing.T, f *fakeSessionClient) wire.CreateSessionRequest {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.creates) != 1 {
		t.Fatalf("creates = %+v, want one", f.creates)
	}
	return f.creates[0]
}

func TestCreate_COpensTheFormOnTheFocusedProject(t *testing.T) {
	m := step(t, createModel(t, &fakeSessionClient{}), letter("c"))
	if m.create == nil || !m.InputFocused() {
		t.Fatal("c did not open the form")
	}
	view := plainView(m)
	for _, want := range []string{"New session", "Where", "gx", "/r/gx", "new worktree…", "Name", "Prompt"} {
		if !strings.Contains(view, want) {
			t.Fatalf("form lacks %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Harness") {
		t.Fatal("the form must not offer a harness picker")
	}
}

func TestCreate_LeaderCOpensTheForm(t *testing.T) {
	m, cmd := leaderThen(t, createModel(t, &fakeSessionClient{}), letter("c"))
	m = update1(t, m, cmd())
	if m.create == nil || m.create.SelectedProject() != "gx" {
		t.Fatalf("leader c form = %+v, want one on gx", m.create)
	}
}

func TestCreate_WithNoSessionsDefaultsToTheCwdProject(t *testing.T) {
	m := openedModel(t, &fakeSessionClient{}, "").WithCwdProject("gx")
	m.listProjects = createProjects
	m = update1(t, m, tea.WindowSizeMsg{Width: 80, Height: 30})
	m.terminalFocused = false
	m = step(t, m, letter("c"))
	if m.create == nil || m.create.SelectedProject() != "gx" {
		t.Fatalf("form = %+v, want one on the cwd project gx", m.create)
	}
}

func TestCreate_SelectedSessionBeatsTheCwdProject(t *testing.T) {
	m := createModel(t, &fakeSessionClient{}).WithCwdProject("web")
	m = step(t, m, letter("c"))
	if m.create == nil || m.create.SelectedProject() != "gx" {
		t.Fatalf("form = %+v, want one on the selected session's project gx", m.create)
	}
}

func TestCreate_SubmitSendsTheFieldsAndPanelSize(t *testing.T) {
	f := &fakeSessionClient{result: wire.SessionResult{Session: &wire.SessionInfo{ID: "new"}}}
	m := step(t, createModel(t, f), letter("c"))
	m = typeKeys(t, m, keyDownMsg, tab, letter("f"), letter("i"), letter("x"), tab, letter("g"), letter("o"))
	m = step(t, m, keyEnterMsg)
	want := wire.CreateSessionRequest{Project: "gx", Dir: "/r/gx/wt1", Name: "fix", Prompt: "go", Cols: 80 - sidebarWidth - 3, Rows: 29}
	if got := lastCreate(t, f); got != want {
		t.Fatalf("create = %+v, want %+v", got, want)
	}
	if m.create != nil {
		t.Fatal("the form stayed open after a create")
	}
	st := &viewmodel.State{Sessions: append(createSessions(), wire.SessionInfo{ID: "new", Project: "gx", Dir: "/r/gx/wt1"})}
	m = withState(t, m, st, tabapi.ServerLinkUp)
	assertFocus(t, m, true, "new")
}
