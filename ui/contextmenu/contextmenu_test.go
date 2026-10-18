package contextmenu

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

type ran string

func item(label string, g Group) Item[string] {
	return Item[string]{Label: label, Group: g, Run: func(t string) tea.Cmd {
		return func() tea.Msg { return ran(label + "@" + t) }
	}}
}

func dim(label string, g Group, reason string) Item[string] {
	it := item(label, g)
	it.Disabled = reason
	return it
}

func open(items ...Item[string]) Menu[string] {
	return Menu[string]{}.Open("gx:epic/01", "Fix the widget", items)
}

func press(m Menu[string], keys ...tea.KeyPressMsg) (Menu[string], tea.Cmd) {
	var cmd tea.Cmd
	for _, k := range keys {
		m, cmd = m.Update(k)
	}
	return m, cmd
}

func typed(m Menu[string], text string) Menu[string] {
	for _, r := range text {
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

var (
	enter = tea.KeyPressMsg{Code: tea.KeyEnter}
	esc   = tea.KeyPressMsg{Code: tea.KeyEsc}
	down  = tea.KeyPressMsg{Code: tea.KeyDown}
	up    = tea.KeyPressMsg{Code: tea.KeyUp}
	ctrlN = tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl}
	ctrlP = tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl}
)

func wantRan(t *testing.T, cmd tea.Cmd, want string) {
	t.Helper()
	if cmd == nil {
		t.Fatalf("no command, want %q", want)
	}
	if got := cmd(); got != ran(want) {
		t.Fatalf("ran %v, want %q", got, want)
	}
}

// viewLines is the plain text of each View line, trimmed of frame, rules,
// cursor bar and space.
func viewLines(m Menu[string]) []string {
	var out []string
	for _, l := range strings.Split(ansi.Strip(m.View()), "\n") {
		out = append(out, strings.Trim(l, "│─▌ "))
	}
	return out
}

func lineOf(t *testing.T, m Menu[string], prefix string) int {
	t.Helper()
	i := slices.IndexFunc(viewLines(m), func(l string) bool { return strings.HasPrefix(l, prefix) })
	if i < 0 {
		t.Fatalf("no line starting %q in:\n%s", prefix, m.View())
	}
	return i
}

func TestEnterRunsItemOnTargetAndCloses(t *testing.T) {
	m, cmd := press(open(item("Resume", Suggested), item("Watch", Agent)), down, enter)
	if m.IsOpen {
		t.Fatal("menu still open after enter")
	}
	wantRan(t, cmd, "Watch@gx:epic/01")
}

func TestEscClosesWithoutRunning(t *testing.T) {
	m, cmd := press(open(item("Resume", Suggested)), esc)
	if m.IsOpen || cmd != nil {
		t.Fatalf("esc: open=%v cmd=%v", m.IsOpen, cmd != nil)
	}
}

func TestFilterNarrowsAndNoMatchSaysSo(t *testing.T) {
	m := typed(open(item("Resume", Suggested), item("Watch", Agent), item("Copy address", EditCopy)), "wa")
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Watch") || strings.Contains(view, "Resume") || strings.Contains(view, "Copy address") {
		t.Fatalf("filter wa:\n%s", view)
	}
	_, cmd := press(m, enter)
	wantRan(t, cmd, "Watch@gx:epic/01")

	m = typed(m, "zz")
	if !strings.Contains(ansi.Strip(m.View()), "no matching actions") {
		t.Fatalf("no match:\n%s", m.View())
	}
	if m, cmd = press(m, enter); cmd != nil || !m.IsOpen {
		t.Fatal("enter with no match should do nothing")
	}
}

func TestPrintableKeysAreFilterText(t *testing.T) {
	m := typed(open(item("Edit", EditCopy)), "q")
	if !m.IsOpen || m.filter != "q" {
		t.Fatalf("q: open=%v filter=%q", m.IsOpen, m.filter)
	}
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	if m.filter != "" {
		t.Fatalf("backspace left filter %q", m.filter)
	}
}

func TestCursorOpensOnFirstEnabledAndStepsOntoDimItems(t *testing.T) {
	m := open(dim("Resume", Suggested, "no pane"), item("Open", Status), dim("Done", Status, "already done"), item("Watch", Agent))
	if _, cmd := press(m, enter); cmd == nil {
		t.Fatal("cursor should open on the first enabled item")
	}
	for _, tc := range []struct {
		keys []tea.KeyPressMsg
		want string // "" = cursor on a dim item, enter does nothing
	}{
		{nil, "Open"},
		{[]tea.KeyPressMsg{down}, ""},
		{[]tea.KeyPressMsg{ctrlN, ctrlN}, "Watch"},
		{[]tea.KeyPressMsg{down, down, down}, "Watch"},
		{[]tea.KeyPressMsg{down, up}, "Open"},
		{[]tea.KeyPressMsg{up}, ""},
		{[]tea.KeyPressMsg{up, ctrlP}, ""},
	} {
		next, _ := press(m, tc.keys...)
		_, cmd := press(next, enter)
		if tc.want == "" {
			if cmd != nil {
				t.Errorf("keys %v: enter on a dim item ran a command", tc.keys)
			}
			continue
		}
		wantRan(t, cmd, tc.want+"@gx:epic/01")
	}
}

func TestEnterOnDimItemDoesNothing(t *testing.T) {
	m, cmd := press(open(dim("Resume", Suggested, "no pane")), enter)
	if cmd != nil || !m.IsOpen {
		t.Fatalf("dim enter: cmd=%v open=%v", cmd != nil, m.IsOpen)
	}
}

func TestGroupsRenderInOrderAndEmptyOnesDrop(t *testing.T) {
	m := open(item("Copy address", EditCopy), item("Watch", Agent), item("Resume", Suggested), item("Mark done", Status))
	var got []string
	for _, l := range viewLines(m) {
		if slices.Contains([]string{"Suggested", "Status", "Server", "Agent", "Queue", "Edit/copy"}, l) {
			got = append(got, l)
		}
	}
	if want := []string{"Suggested", "Status", "Agent", "Edit/copy"}; !slices.Equal(got, want) {
		t.Fatalf("headers %v, want %v", got, want)
	}
	if lineOf(t, m, "Resume") > lineOf(t, m, "Mark done") || lineOf(t, m, "Watch") > lineOf(t, m, "Copy address") {
		t.Fatalf("items out of group order:\n%s", m.View())
	}

	m = typed(m, "watch")
	lines := viewLines(m)
	if !slices.Contains(lines, "Agent") || slices.Contains(lines, "Suggested") || slices.Contains(lines, "Edit/copy") {
		t.Fatalf("filtered headers:\n%s", m.View())
	}
}

func TestDimItemShowsReasonOnlyUnderTheCursor(t *testing.T) {
	m := open(Item[string]{Label: "Resume", Key: "r r", Group: Suggested}, Item[string]{Label: "Watch", Key: "w", Group: Agent, Disabled: "no agent"})
	lines := viewLines(m)
	if l := lines[lineOf(t, m, "Resume")]; !strings.HasSuffix(l, "r r") {
		t.Fatalf("enabled row %q lacks key", l)
	}
	if l := lines[lineOf(t, m, "Watch")]; l != "Watch" {
		t.Fatalf("dim row %q should show neither key nor reason", l)
	}
	if strings.Contains(ansi.Strip(m.View()), "no agent") {
		t.Fatalf("reason shown with the cursor elsewhere:\n%s", m.View())
	}
	height := len(lines)

	m, _ = press(m, down)
	lineOf(t, m, "Watch: no agent")
	if got := len(viewLines(m)); got != height {
		t.Fatalf("height %d with the reason, want %d", got, height)
	}
}

func TestTitleAndPromptNameTarget(t *testing.T) {
	view := ansi.Strip(open(item("Resume", Suggested)).View())
	if first := strings.SplitN(view, "\n", 2)[0]; !strings.Contains(first, "Actions · Fix the widget") {
		t.Fatalf("view:\n%s", view)
	}
}

func TestClickRunsEnabledItem(t *testing.T) {
	m := open(item("Resume", Suggested), dim("Done", Status, "already done"), item("Watch", Agent))

	if next, cmd := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, Y: lineOf(t, m, "Done")}); cmd != nil || !next.IsOpen {
		t.Fatal("click on dim item should do nothing")
	}
	if _, cmd := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, Y: lineOf(t, m, "Status")}); cmd != nil {
		t.Fatal("click on header should do nothing")
	}
	if _, cmd := m.Update(tea.MouseClickMsg{Button: tea.MouseRight, Y: lineOf(t, m, "Watch")}); cmd != nil {
		t.Fatal("right click should do nothing")
	}
	next, cmd := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, Y: lineOf(t, m, "Watch")})
	if next.IsOpen {
		t.Fatal("menu still open after click")
	}
	wantRan(t, cmd, "Watch@gx:epic/01")

	m = Menu[string]{}.Open("gx:epic/01", strings.Repeat("a very long ticket title ", 8), m.items)
	_, cmd = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, Y: lineOf(t, m, "Watch")})
	wantRan(t, cmd, "Watch@gx:epic/01")

	m = typed(m, "wat")
	_, cmd = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, Y: lineOf(t, m, "Watch")})
	wantRan(t, cmd, "Watch@gx:epic/01")
}

func TestOriginPlacesNextToAnchor(t *testing.T) {
	const w, h = 10, 5
	for _, tc := range []struct {
		name         string
		a            Anchor
		screenW      int
		screenH      int
		wantX, wantY int
	}{
		{"below", Anchor{X: 1, Y: 2, H: 1}, 80, 20, 1, 3},
		{"below a multi-line row", Anchor{X: 1, Y: 2, H: 3}, 80, 20, 1, 5},
		{"above when below is short", Anchor{X: 1, Y: 16, H: 1}, 80, 20, 1, 11},
		{"neither: roomier below, pinned to bottom", Anchor{X: 1, Y: 2, H: 1}, 80, 6, 1, 1},
		{"neither: roomier above, pinned to top", Anchor{X: 1, Y: 4, H: 1}, 80, 6, 1, 0},
		{"clamped to the right edge", Anchor{X: 75, Y: 2, H: 1}, 80, 20, 70, 3},
		{"zero anchor centers", Anchor{}, 80, 20, 35, 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			x, y := Origin(tc.a, w, h, tc.screenW, tc.screenH)
			if x != tc.wantX || y != tc.wantY {
				t.Fatalf("got (%d,%d) want (%d,%d)", x, y, tc.wantX, tc.wantY)
			}
		})
	}
}

func TestUpdateMouseHitsAnchoredMenu(t *testing.T) {
	m := open(item("Start", Suggested))
	a := Anchor{X: 4, Y: 2, H: 1}
	// The menu opens on line 3; its first item is listTop+1 lines down (the
	// group header comes first), one cell inside the left edge.
	click := tea.MouseClickMsg{X: 4 + 2, Y: 3 + listTop + 1, Button: tea.MouseLeft}
	m, cmd := m.UpdateMouse(click, a, 80, 40)
	if cmd == nil || m.IsOpen {
		t.Fatalf("click on the anchored item did not run it")
	}
	if got := cmd(); got != ran("Start@gx:epic/01") {
		t.Fatalf("ran %v", got)
	}
}

func TestClickOutsideCloses(t *testing.T) {
	a := Anchor{X: 4, Y: 2, H: 1}
	for _, tc := range []struct {
		name string
		x, y int
		btn  tea.MouseButton
	}{
		{"above", 6, 1, tea.MouseLeft},
		{"left", 0, 3 + listTop + 1, tea.MouseLeft},
		{"right, on an item's line", 4 + width + 2, 3 + listTop + 1, tea.MouseLeft},
		{"right button", 6, 1, tea.MouseRight},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, cmd := open(item("Start", Suggested)).UpdateMouse(tea.MouseClickMsg{X: tc.x, Y: tc.y, Button: tc.btn}, a, 80, 40)
			if m.IsOpen || cmd != nil {
				t.Fatalf("open=%v cmd=%v, want closed with no command", m.IsOpen, cmd != nil)
			}
		})
	}
}
