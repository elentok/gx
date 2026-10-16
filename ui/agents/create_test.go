package agents

import (
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/server"
	ticketsui "github.com/elentok/gx/ui/tickets"
	"github.com/elentok/gx/viewmodel"
)

var (
	keyEnterMsg     = tea.KeyPressMsg{Code: tea.KeyEnter}
	keyEscMsg       = tea.KeyPressMsg{Code: tea.KeyEscape}
	keyDownMsg      = tea.KeyPressMsg{Code: tea.KeyDown}
	keyUpMsg        = tea.KeyPressMsg{Code: tea.KeyUp}
	keyBackspaceMsg = tea.KeyPressMsg{Code: tea.KeyBackspace}
)

func createSessions() []server.SessionInfo {
	return []server.SessionInfo{
		{ID: "w1", Project: "web", Dir: "/r/web", Status: idleStatus},
		{ID: "g1", Project: "gx", Dir: "/r/gx/wt1", Status: idleStatus},
		{ID: "g2", Project: "gx", Dir: "/r/gx/wt1", Status: idleStatus},
		{ID: "g3", Project: "gx", Dir: "/r/gx/wt2", Status: server.SessionStatus{State: server.SessionExited}},
	}
}

func createProjects() ([]formProject, error) {
	return []formProject{
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

func lastCreate(t *testing.T, f *fakeSessionClient) server.CreateSessionRequest {
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
	for _, want := range []string{"New session", "Where", "gx", "/r/gx", newWorktreeLabel, "Name", "Prompt"} {
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
	if m.create == nil || m.create.selectedProject() != "gx" {
		t.Fatalf("leader c form = %+v, want one on gx", m.create)
	}
}

func TestCreate_WithNoSessionsDefaultsToTheCwdProject(t *testing.T) {
	m := openedModel(t, &fakeSessionClient{}, "").WithCwdProject("gx")
	m.listProjects = createProjects
	m = update1(t, m, tea.WindowSizeMsg{Width: 80, Height: 30})
	m.terminalFocused = false
	m = step(t, m, letter("c"))
	if m.create == nil || m.create.selectedProject() != "gx" {
		t.Fatalf("form = %+v, want one on the cwd project gx", m.create)
	}
}

func TestCreate_SelectedSessionBeatsTheCwdProject(t *testing.T) {
	m := createModel(t, &fakeSessionClient{}).WithCwdProject("web")
	m = step(t, m, letter("c"))
	if m.create == nil || m.create.selectedProject() != "gx" {
		t.Fatalf("form = %+v, want one on the selected session's project gx", m.create)
	}
}

func TestCreate_DirPickerCountsSessionsPerDir(t *testing.T) {
	m := step(t, createModel(t, &fakeSessionClient{}), letter("c"))
	lines := strings.Split(ansi.Strip(m.renderCreate()), "\n")
	row := func(dir string) string {
		for _, l := range lines {
			if strings.Contains(l, "│     "+dir+" ") {
				return l
			}
		}
		t.Fatalf("no row for %s in:\n%s", dir, strings.Join(lines, "\n"))
		return ""
	}
	if r := row("/r/gx/wt1"); !strings.Contains(r, "2 sessions") {
		t.Fatalf("wt1 row = %q, want 2 sessions", r)
	}
	// Exited sessions no longer use their dir.
	for _, dir := range []string{"/r/gx", "/r/gx/wt2"} {
		if r := row(dir); strings.Contains(r, "session") {
			t.Fatalf("%s row = %q, want no count", dir, r)
		}
	}
}

func TestCreate_SubmitSendsTheFieldsAndPanelSize(t *testing.T) {
	f := &fakeSessionClient{result: server.SessionResult{Session: &server.SessionInfo{ID: "new"}}}
	m := step(t, createModel(t, f), letter("c"))
	m = typeKeys(t, m, keyDownMsg, tab, letter("f"), letter("i"), letter("x"), tab, letter("g"), letter("o"))
	m = step(t, m, keyEnterMsg)
	want := server.CreateSessionRequest{Project: "gx", Dir: "/r/gx/wt1", Name: "fix", Prompt: "go", Cols: 80 - sidebarWidth - 3, Rows: 28}
	if got := lastCreate(t, f); got != want {
		t.Fatalf("create = %+v, want %+v", got, want)
	}
	if m.create != nil {
		t.Fatal("the form stayed open after a create")
	}
	st := &viewmodel.State{Sessions: append(createSessions(), server.SessionInfo{ID: "new", Project: "gx", Dir: "/r/gx/wt1"})}
	m = withState(t, m, st, ticketsui.ServerLinkUp)
	assertFocus(t, m, true, "new")
}

func TestCreate_NewWorktreeAsksForAName(t *testing.T) {
	f := &fakeSessionClient{result: server.SessionResult{Session: &server.SessionInfo{ID: "new"}}}
	m := step(t, createModel(t, f), letter("c"))
	m = typeKeys(t, m, letter("g"), letter("x"), letter(" "), letter("n"), letter("e"), letter("w"), tab, letter("f"), letter("t"))
	if !strings.Contains(plainView(m), "Worktree") {
		t.Fatal("new worktree did not ask for a name")
	}
	m = step(t, m, keyEnterMsg)
	if got := lastCreate(t, f); got.Project != "gx" || got.Worktree != "ft" || got.Dir != "" {
		t.Fatalf("create = %+v, want worktree ft and no dir", got)
	}
}

func TestCreate_ErrorStaysInTheForm(t *testing.T) {
	f := &fakeSessionClient{result: server.SessionResult{Refused: true, Message: "dir /r/gx does not exist"}}
	m := step(t, step(t, createModel(t, f), letter("c")), keyEnterMsg)
	if m.create == nil || !strings.Contains(plainView(m), "dir /r/gx does not exist") {
		t.Fatalf("refusal not shown in the form:\n%s", plainView(m))
	}
	if m = update1(t, m, keyEscMsg); m.create != nil {
		t.Fatal("esc did not close the form")
	}
}

func TestCreate_PrefilledFormIsEditable(t *testing.T) {
	f := &fakeSessionClient{result: server.SessionResult{Session: &server.SessionInfo{ID: "new"}}}
	m := createModel(t, f)
	m.cursor = 0
	next, cmd := m.OpenCreateForm(CreatePrefill{Project: "gx", Dir: "/r/gx/wt2", Name: "plan", Prompt: "read it", Ticket: "gx:e/7"})
	m = update1(t, next, cmd())
	view := plainView(m)
	for _, want := range []string{"plan", "read it", "gx:e/7"} {
		if !strings.Contains(view, want) {
			t.Fatalf("pre-filled form lacks %q:\n%s", want, view)
		}
	}
	m = typeKeys(t, m, keyUpMsg, tab, keyBackspaceMsg, letter("x"))
	m = step(t, m, keyEnterMsg)
	got := lastCreate(t, f)
	if got.Project != "gx" || got.Dir != "/r/gx/wt1" || got.Name != "plax" || got.Prompt != "read it" || got.Ticket != "gx:e/7" {
		t.Fatalf("create = %+v", got)
	}
}

func TestCreate_PickerGroupsEveryProjectFocusedFirst(t *testing.T) {
	m := step(t, createModel(t, &fakeSessionClient{}), letter("c"))
	view := ansi.Strip(m.renderCreate())
	gx, web := strings.Index(view, "\n│   gx "), strings.Index(view, "\n│   web ")
	if gx < 0 || web < 0 || gx > web {
		t.Fatalf("want a gx header before a web header:\n%s", view)
	}
	if strings.Count(view, newWorktreeLabel) != 2 {
		t.Fatalf("want a new worktree entry per project:\n%s", view)
	}
}

func TestCreate_TypingFuzzyFiltersAcrossProjects(t *testing.T) {
	f := &fakeSessionClient{result: server.SessionResult{Session: &server.SessionInfo{ID: "new"}}}
	m := step(t, createModel(t, f), letter("c"))
	m = typeKeys(t, m, letter("W"), letter("b"))
	view := ansi.Strip(m.renderCreate())
	if strings.Contains(view, "/r/gx") || !strings.Contains(view, "/r/web") {
		t.Fatalf("query wb should keep only web:\n%s", view)
	}
	m = step(t, m, keyEnterMsg)
	if got := lastCreate(t, f); got.Project != "web" || got.Dir != "/r/web" {
		t.Fatalf("create = %+v, want web at /r/web", got)
	}
}

func TestCreate_TermsMatchProjectAndDirSeparately(t *testing.T) {
	m := step(t, createModel(t, &fakeSessionClient{}), letter("c"))
	m = typeKeys(t, m, letter("g"), letter(" "), letter("w"), letter("2"))
	if e, ok := m.create.selected(); !ok || e.dir != "/r/gx/wt2" || len(m.create.matches) != 1 {
		t.Fatalf("matches = %+v, want only /r/gx/wt2", m.create.matches)
	}
}

func TestCreate_NoMatchRefusesToSubmit(t *testing.T) {
	f := &fakeSessionClient{}
	m := step(t, createModel(t, f), letter("c"))
	m = typeKeys(t, m, letter("z"), letter("z"))
	m = step(t, m, keyEnterMsg)
	view := ansi.Strip(m.renderCreate())
	if !strings.Contains(view, "no matches") || !strings.Contains(view, `nothing matches "zz"`) {
		t.Fatalf("want no matches and a refusal:\n%s", view)
	}
	if len(f.creates) != 0 {
		t.Fatalf("creates = %+v, want none", f.creates)
	}
}

func TestCreate_PickerScrollsToKeepTheSelectionVisible(t *testing.T) {
	m := createModel(t, &fakeSessionClient{})
	m.listProjects = func() ([]formProject, error) {
		var wts []string
		for i := range 30 {
			wts = append(wts, "/r/gx/wt"+strconv.Itoa(i))
		}
		return []formProject{{Name: "gx", Root: "/r/gx", Worktrees: wts}}, nil
	}
	m = step(t, m, letter("c"))
	if strings.Contains(ansi.Strip(m.renderCreate()), "/r/gx/wt29 ") {
		t.Fatal("the last worktree should start off-screen")
	}
	m = typeKeys(t, m, keyUpMsg, keyUpMsg)
	view := ansi.Strip(m.renderCreate())
	if !strings.Contains(view, "/r/gx/wt29 ") || strings.Contains(view, "/r/gx/wt0 ") {
		t.Fatalf("up from the top should scroll to the end:\n%s", view)
	}
}
