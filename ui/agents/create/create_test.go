package create

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/elentok/gx/server/wire"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

var (
	keyEnterMsg     = tea.KeyPressMsg{Code: tea.KeyEnter}
	keyEscMsg       = tea.KeyPressMsg{Code: tea.KeyEscape}
	keyUpMsg        = tea.KeyPressMsg{Code: tea.KeyUp}
	keyBackspaceMsg = tea.KeyPressMsg{Code: tea.KeyBackspace}
	tab             = tea.KeyPressMsg{Code: tea.KeyTab}
)

func letter(k string) tea.KeyPressMsg { return tea.KeyPressMsg{Code: rune(k[0]), Text: k} }

var idleStatus = wire.SessionStatus{State: wire.SessionIdle}

func createSessions() []wire.SessionInfo {
	return []wire.SessionInfo{
		{ID: "w1", Project: "web", Dir: "/r/web", Status: idleStatus},
		{ID: "g1", Project: "gx", Dir: "/r/gx/wt1", Status: idleStatus},
		{ID: "g2", Project: "gx", Dir: "/r/gx/wt1", Status: idleStatus},
		{ID: "g3", Project: "gx", Dir: "/r/gx/wt2", Status: wire.SessionStatus{State: wire.SessionExited}},
	}
}

func createProjects() ([]Project, error) {
	return []Project{
		{Name: "web", Root: "/r/web"},
		{Name: "gx", Root: "/r/gx", Worktrees: []string{"/r/gx/wt1", "/r/gx/wt2"}},
	}, nil
}

// fakeCreator records the create requests it receives.
type fakeCreator struct {
	mu      sync.Mutex
	creates []wire.CreateSessionRequest
	result  wire.SessionResult
}

func (f *fakeCreator) Create(_ context.Context, req wire.CreateSessionRequest) (wire.SessionResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.creates = append(f.creates, req)
	return f.result, nil
}

func (f *fakeCreator) last(t *testing.T) wire.CreateSessionRequest {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.creates) != 1 {
		t.Fatalf("creates = %+v, want one", f.creates)
	}
	return f.creates[0]
}

// host stands in for the Agents Model: it feeds the form's messages back.
type host struct {
	t        *testing.T
	form     *Form
	ctx      Ctx
	sessions []wire.SessionInfo
}

func newHost(t *testing.T, c *fakeCreator, list func() ([]Project, error), p Prefill) *host {
	t.Helper()
	h := &host{t: t, ctx: Ctx{Height: 30, MainWidth: 60, Sessions: c}, sessions: createSessions()}
	h.form = New("", p)
	h.form = h.form.UpdateProjects(h.form.Load(list)().(ProjectsMsg), h.ctx)
	return h
}

// open starts on the gx project, as the Model does for a focused gx session.
func open(t *testing.T, c *fakeCreator) *host {
	return newHost(t, c, createProjects, Prefill{Project: "gx"})
}

func (h *host) key(keys ...tea.KeyPressMsg) {
	h.t.Helper()
	for _, k := range keys {
		next, cmd, _ := h.form.Update(k, h.ctx)
		h.form = next
		if cmd != nil && h.form != nil {
			h.form, _ = h.form.UpdateCreated(cmd().(CreatedMsg))
		}
	}
}

func (h *host) view() string {
	return ansi.Strip(h.form.View(80, 30, h.sessions))
}

func TestCreate_DirPickerCountsSessionsPerDir(t *testing.T) {
	lines := strings.Split(open(t, &fakeCreator{}).view(), "\n")
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

func TestCreate_NewWorktreeAsksForAName(t *testing.T) {
	c := &fakeCreator{result: wire.SessionResult{Session: &wire.SessionInfo{ID: "new"}}}
	h := open(t, c)
	h.key(letter("g"), letter("x"), letter(" "), letter("n"), letter("e"), letter("w"), tab, letter("f"), letter("t"))
	if !strings.Contains(h.view(), "Worktree") {
		t.Fatal("new worktree did not ask for a name")
	}
	h.key(keyEnterMsg)
	if got := c.last(t); got.Project != "gx" || got.Worktree != "ft" || got.Dir != "" {
		t.Fatalf("create = %+v, want worktree ft and no dir", got)
	}
}

func TestCreate_ErrorStaysInTheForm(t *testing.T) {
	c := &fakeCreator{result: wire.SessionResult{Refused: true, Message: "dir /r/gx does not exist"}}
	h := open(t, c)
	h.key(keyEnterMsg)
	if h.form == nil || !strings.Contains(h.view(), "dir /r/gx does not exist") {
		t.Fatalf("refusal not shown in the form:\n%s", h.view())
	}
	if h.key(keyEscMsg); h.form != nil {
		t.Fatal("esc did not close the form")
	}
}

func TestCreate_PrefilledFormIsEditable(t *testing.T) {
	c := &fakeCreator{result: wire.SessionResult{Session: &wire.SessionInfo{ID: "new"}}}
	h := newHost(t, c, createProjects, Prefill{Project: "gx", Dir: "/r/gx/wt2", Name: "plan", Prompt: "read it", Ticket: "gx:e/7"})
	view := h.view()
	for _, want := range []string{"plan", "read it", "gx:e/7"} {
		if !strings.Contains(view, want) {
			t.Fatalf("pre-filled form lacks %q:\n%s", want, view)
		}
	}
	h.key(keyBackspaceMsg, letter("x"), keyEnterMsg)
	got := c.last(t)
	if got.Project != "gx" || got.Dir != "/r/gx/wt2" || got.Name != "plax" || got.Prompt != "read it" || got.Ticket != "gx:e/7" {
		t.Fatalf("create = %+v", got)
	}
}

func TestCreate_PrefillWithDirStartsOnName(t *testing.T) {
	for _, tc := range []struct {
		name      string
		prefill   Prefill
		wantField createField
	}{
		{"with dir", Prefill{Project: "gx", Dir: "/r/gx/wt2", Name: "plan"}, fieldName},
		{"without dir", Prefill{Project: "gx", Name: "plan"}, fieldWhere},
	} {
		if got := New("", tc.prefill).field; got != tc.wantField {
			t.Errorf("%s: focus = %v, want %v", tc.name, got, tc.wantField)
		}
	}
}

func TestCreate_PickerGroupsEveryProjectFocusedFirst(t *testing.T) {
	view := open(t, &fakeCreator{}).view()
	gx, web := strings.Index(view, "\n│   gx "), strings.Index(view, "\n│   web ")
	if gx < 0 || web < 0 || gx > web {
		t.Fatalf("want a gx header before a web header:\n%s", view)
	}
	if strings.Count(view, newWorktreeLabel) != 2 {
		t.Fatalf("want a new worktree entry per project:\n%s", view)
	}
}

func TestCreate_TypingFuzzyFiltersAcrossProjects(t *testing.T) {
	c := &fakeCreator{result: wire.SessionResult{Session: &wire.SessionInfo{ID: "new"}}}
	h := open(t, c)
	h.key(letter("W"), letter("b"))
	view := h.view()
	if strings.Contains(view, "/r/gx") || !strings.Contains(view, "/r/web") {
		t.Fatalf("query wb should keep only web:\n%s", view)
	}
	h.key(keyEnterMsg)
	if got := c.last(t); got.Project != "web" || got.Dir != "/r/web" {
		t.Fatalf("create = %+v, want web at /r/web", got)
	}
}

func TestCreate_TermsMatchProjectAndDirSeparately(t *testing.T) {
	h := open(t, &fakeCreator{})
	h.key(letter("g"), letter(" "), letter("w"), letter("2"))
	if e, ok := h.form.selected(); !ok || e.dir != "/r/gx/wt2" || len(h.form.matches) != 1 {
		t.Fatalf("matches = %+v, want only /r/gx/wt2", h.form.matches)
	}
}

func TestCreate_NoMatchRefusesToSubmit(t *testing.T) {
	c := &fakeCreator{}
	h := open(t, c)
	h.key(letter("z"), letter("z"), keyEnterMsg)
	view := h.view()
	if !strings.Contains(view, "no matches") || !strings.Contains(view, `nothing matches "zz"`) {
		t.Fatalf("want no matches and a refusal:\n%s", view)
	}
	if len(c.creates) != 0 {
		t.Fatalf("creates = %+v, want none", c.creates)
	}
}

func TestCreate_PickerScrollsToKeepTheSelectionVisible(t *testing.T) {
	list := func() ([]Project, error) {
		var wts []string
		for i := range 30 {
			wts = append(wts, "/r/gx/wt"+strconv.Itoa(i))
		}
		return []Project{{Name: "gx", Root: "/r/gx", Worktrees: wts}}, nil
	}
	h := newHost(t, &fakeCreator{}, list, Prefill{Project: "gx"})
	if strings.Contains(h.view(), "/r/gx/wt29 ") {
		t.Fatal("the last worktree should start off-screen")
	}
	h.key(keyUpMsg, keyUpMsg)
	view := h.view()
	if !strings.Contains(view, "/r/gx/wt29 ") || strings.Contains(view, "/r/gx/wt0 ") {
		t.Fatalf("up from the top should scroll to the end:\n%s", view)
	}
}

func TestCreate_MatchesRankByScoreWithinAndAcrossProjects(t *testing.T) {
	list := func() ([]Project, error) {
		return []Project{
			{Name: "gx", Root: "/r/gx", Worktrees: []string{"/r/gx/t-x-a-x-b"}},
			{Name: "web", Root: "/r/web", Worktrees: []string{"/r/web/t-x-a-x-b", "/r/web/tab"}},
		}, nil
	}
	h := newHost(t, &fakeCreator{}, list, Prefill{Project: "gx"})
	h.key(letter("t"), letter("a"), letter("b"))
	var got []string
	for _, e := range h.form.matches {
		got = append(got, e.dir)
	}
	want := []string{"/r/web/tab", "/r/web/t-x-a-x-b", "/r/gx/t-x-a-x-b"}
	if !slices.Equal(got, want) {
		t.Fatalf("matches = %v, want %v", got, want)
	}
}

func TestCreate_HighlightsTheMatchedLabelRunes(t *testing.T) {
	h := open(t, &fakeCreator{})
	h.key(letter("g"), letter(" "), letter("w"), letter("t"), letter("2"))
	e, _ := h.form.selected()
	if !slices.Equal(e.pos, []int{6, 7, 8}) {
		t.Fatalf("label pos = %v, want wt2 at 6..8 of /r/gx/wt2", e.pos)
	}
}
