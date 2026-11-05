package jump

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/ui"
)

var update = flag.Bool("update", false, "rewrite the golden files")

var (
	ctrlN     = tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl}
	ctrlP     = tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl}
	enter     = tea.KeyPressMsg{Code: tea.KeyEnter}
	esc       = tea.KeyPressMsg{Code: tea.KeyEscape}
	tab       = tea.KeyPressMsg{Code: tea.KeyTab}
	backspace = tea.KeyPressMsg{Code: tea.KeyBackspace}
)

// fixtureSessions covers every status, both marks, +bg and a Session ticket.
func fixtureSessions() []wire.SessionInfo {
	return []wire.SessionInfo{
		{ID: "s1", Name: "fix login", Project: "gx", Ticket: "gx:auth/03",
			Status: wire.SessionStatus{State: wire.SessionWorking, Background: true},
			Unread: wire.SessionUnread{Unread: true}},
		{ID: "s2", Name: "nvim", Project: "dotfiles",
			Status: wire.SessionStatus{State: wire.SessionExited}, Exit: wire.SessionExitCrashed,
			Actionable: wire.SessionActionable{Reason: "crashed"}},
		{ID: "s3", Name: "docs", Project: "gx", Ticket: "gx:map/01",
			Status: wire.SessionStatus{State: wire.SessionIdle}},
		{ID: "s4", Name: "review", Project: "gx", Status: wire.SessionStatus{State: wire.SessionBlocked},
			Unread: wire.SessionUnread{Unread: true}},
		{ID: "s5", Name: "zsh", Project: "dotfiles",
			Status: wire.SessionStatus{State: wire.SessionExited}, Exit: wire.SessionExitClean},
		{ID: "s6", Name: "limits", Project: "dotfiles", Status: wire.SessionStatus{State: wire.SessionRateLimited}},
		{ID: "s7", Project: "dotfiles", Status: wire.SessionStatus{State: wire.SessionStarting}},
	}
}

func fixtureCtx() Ctx {
	return Ctx{
		Sessions: fixtureSessions(),
		Now:      func() time.Time { return time.Time{} },
		Width:    110,
		Height:   16,
		Settings: ui.Settings{},
	}
}

// harness drives a popup the way the agents Model does.
type harness struct {
	t *testing.T
	c Ctx
	p *Popup
}

func newHarness(t *testing.T) *harness { return &harness{t: t, c: fixtureCtx(), p: &Popup{}} }

func (h *harness) press(k tea.KeyPressMsg) Result {
	h.t.Helper()
	var res Result
	h.p, res = h.p.Update(k, h.c)
	return res
}

func (h *harness) typeText(text string) {
	h.t.Helper()
	for _, r := range text {
		h.press(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

func (h *harness) ids() string {
	var ids []string
	for _, i := range h.p.Matches(h.c) {
		ids = append(ids, h.c.Sessions[i].ID)
	}
	return strings.Join(ids, " ")
}

func (h *harness) plainView() string { return ansi.Strip(h.p.Render(h.c)) }

func assertGolden(t *testing.T, name, text string) {
	t.Helper()
	golden := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.WriteFile(golden, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if text != string(want) {
		t.Errorf("render mismatch (run with -update to accept)\ngot:\n%s\nwant:\n%s", ansi.Strip(text), ansi.Strip(string(want)))
	}
}

func TestJump_Golden(t *testing.T) {
	assertGolden(t, "jump_popup", newHarness(t).p.Render(fixtureCtx()))
}

func TestJump_AttentionOrderBucketsAreStable(t *testing.T) {
	h := newHarness(t)
	// Actionable s2 (reason) and s4 (Blocked), then the Unread s1, then the
	// rest, each bucket in session order.
	if got := h.ids(); got != "s2 s4 s1 s3 s5 s6 s7" {
		t.Fatalf("attention order = %q", got)
	}
	if !strings.Contains(h.plainView(), "attention · 7/7") {
		t.Fatalf("border missing the order and count:\n%s", h.plainView())
	}
}

func TestJump_TabTogglesProjectOrder(t *testing.T) {
	h := newHarness(t)
	h.press(tab)
	if got := h.ids(); got != "s1 s2 s3 s4 s5 s6 s7" {
		t.Fatalf("project order = %q", got)
	}
	if !strings.Contains(h.plainView(), "project · 7/7") {
		t.Fatalf("border missing the project order:\n%s", h.plainView())
	}
	if h.press(tab); !h.p.attention() {
		t.Fatal("a second tab did not go back to attention order")
	}
}

func TestJump_TypingFiltersOverNameProjectAndDir(t *testing.T) {
	h := newHarness(t)
	h.c.Sessions[2].Dir = "/src/website" // s3
	for _, tc := range []struct{ query, want string }{
		{"RVW", "s4"},          // name, case-insensitive subsequence
		{"dtf", "s2 s5 s6 s7"}, // project
		{"wbst", "s3"},         // dir
	} {
		h.p.query = ""
		h.typeText(tc.query)
		if got := h.ids(); got != tc.want {
			t.Fatalf("%q matched %q, want %q", tc.query, got, tc.want)
		}
	}
	if !strings.Contains(h.plainView(), "attention · 1/7") {
		t.Fatalf("count not updated by the filter:\n%s", h.plainView())
	}
	h.press(backspace)
	if h.p.query != "wbs" {
		t.Fatalf("backspace left query %q", h.p.query)
	}
}

func TestJump_MovementClampsToMatches(t *testing.T) {
	h := newHarness(t)
	for _, step := range []struct {
		key  tea.KeyPressMsg
		want int
	}{
		{ctrlN, 1}, {tea.KeyPressMsg{Code: tea.KeyDown}, 2}, {ctrlP, 1},
		{tea.KeyPressMsg{Code: tea.KeyUp}, 0}, {ctrlP, 0},
	} {
		if h.press(step.key); h.p.cursor != step.want {
			t.Fatalf("after %s: cursor %d, want %d", step.key, h.p.cursor, step.want)
		}
	}
	h.press(ctrlN)
	h.press(ctrlN)
	h.typeText("rvw")
	if h.p.cursor != 0 {
		t.Fatalf("cursor %d after filtering to one match, want 0", h.p.cursor)
	}
}

func TestJump_EnterPicksTheMatchedSession(t *testing.T) {
	h := newHarness(t)
	h.typeText("rvw")
	res := h.press(enter)
	if h.p != nil || !res.Closed || !res.Picked || h.c.Sessions[res.Session].ID != "s4" {
		t.Fatalf("popup=%v result=%+v, want closed and s4 picked", h.p, res)
	}
}

func TestJump_EnterWithNoMatchKeepsThePopup(t *testing.T) {
	h := newHarness(t)
	h.typeText("zzz")
	if res := h.press(enter); h.p == nil || res.Closed || res.Picked {
		t.Fatal("enter with no match closed the popup or picked a session")
	}
}

func TestJump_EscCloses(t *testing.T) {
	h := newHarness(t)
	if res := h.press(esc); h.p != nil || !res.Closed {
		t.Fatal("esc did not close the popup")
	}
}

func TestJump_RowShowsReasonOrDetail(t *testing.T) {
	h := newHarness(t)
	h.c.Sessions[5].Status = wire.SessionStatus{State: wire.SessionRateLimited, Kind: "limit", Message: "resets 5pm"}
	view := h.plainView()
	for _, want := range []string{"crashed", "limit: resets 5pm", "dotfiles"} {
		if !strings.Contains(view, want) {
			t.Fatalf("popup missing %q:\n%s", want, view)
		}
	}
}
