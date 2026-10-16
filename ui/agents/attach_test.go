package agents

import (
	"io"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/agentrunner/screen"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/notify"
	ticketsui "github.com/elentok/gx/ui/tickets"
	"github.com/elentok/gx/viewmodel"
)

// fakeAttach is an attach stream whose messages the test queues.
type fakeAttach struct {
	msgs   chan nativerunner.AttachMessage
	done   chan struct{}
	keys   []screen.Key
	pastes []string
	sizes  [][2]int
	closed bool
}

func newFakeAttach() *fakeAttach {
	return &fakeAttach{msgs: make(chan nativerunner.AttachMessage, 16), done: make(chan struct{})}
}

func (f *fakeAttach) Next() (nativerunner.AttachMessage, error) {
	select {
	case msg := <-f.msgs:
		return msg, nil
	case <-f.done:
		return nativerunner.AttachMessage{}, io.EOF
	}
}

func (f *fakeAttach) SendKey(k screen.Key) error { f.keys = append(f.keys, k); return nil }
func (f *fakeAttach) Paste(text string) error    { f.pastes = append(f.pastes, text); return nil }
func (f *fakeAttach) Resize(cols, rows int) error {
	f.sizes = append(f.sizes, [2]int{cols, rows})
	return nil
}

func (f *fakeAttach) Close() error {
	if !f.closed {
		f.closed = true
		close(f.done)
	}
	return nil
}

func (f *fakeAttach) frame(rows ...string) {
	var fr nativerunner.Frame
	for i, r := range rows {
		fr.Rows = append(fr.Rows, nativerunner.FrameRow{Index: i, ANSI: r})
	}
	f.msgs <- nativerunner.AttachMessage{Type: nativerunner.StreamFrame, Frame: fr}
}

// fakeHolders serves each session id an attach, or a snapshot for an older
// holder.
type fakeHolders struct {
	attaches  map[string]*fakeAttach
	snapshots map[string][]string
	dialed    []string
}

func (h *fakeHolders) dial(dir string) (AttachStream, []string, error) {
	id := filepath.Base(dir)
	h.dialed = append(h.dialed, id)
	if rows, ok := h.snapshots[id]; ok {
		return nil, rows, nil
	}
	if h.attaches[id] == nil {
		h.attaches[id] = newFakeAttach()
	}
	return h.attaches[id], nil, nil
}

var idleStatus = server.SessionStatus{State: server.SessionIdle}

// attachModel shows sessions a and b (idle, so no spinner cmd) over fake
// holders, and returns the pending dial.
func attachModel(t *testing.T, h *fakeHolders) (Model, tea.Cmd) {
	t.Helper()
	m := NewModel(&fakeSessionClient{}, ui.Settings{})
	m.sessionsDir = "/sessions"
	m.dial = h.dial
	m = update1(t, m, tea.WindowSizeMsg{Width: 80, Height: 12})
	st := &viewmodel.State{Sessions: []server.SessionInfo{
		{ID: "a", Name: "alpha", Project: "gx", Status: idleStatus},
		{ID: "b", Name: "beta", Project: "gx", Status: idleStatus},
	}}
	next, cmd := m.WithServerState(st, ticketsui.ServerLinkUp)
	return next.(Model), cmd
}

// dialed runs the pending dial and returns the attach's first read.
func dialed(t *testing.T, m Model, cmd tea.Cmd) (Model, tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("no dial scheduled")
	}
	msg, ok := cmd().(attachedMsg)
	if !ok {
		t.Fatalf("expected a dial, got %T", msg)
	}
	next, read := m.Update(msg)
	return next.(Model), read
}

// read delivers the attach's next message and returns the following read.
func read(t *testing.T, m Model, cmd tea.Cmd) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(cmd())
	return next.(Model), cmd
}

func plainView(m Model) string { return ansi.Strip(m.View().Content) }

func TestAttach_FramesRenderInThePanel(t *testing.T) {
	h := &fakeHolders{attaches: map[string]*fakeAttach{}}
	m, cmd := attachModel(t, h)
	m, next := dialed(t, m, cmd)
	h.attaches["a"].frame("$ claude", "> hello")
	m, next = read(t, m, next)
	if next == nil {
		t.Fatal("the attach stopped reading after a frame")
	}
	view := plainView(m)
	if !strings.Contains(view, "$ claude") || !strings.Contains(view, "> hello") {
		t.Fatalf("frame rows not drawn:\n%s", view)
	}

	h.attaches["a"].msgs <- nativerunner.AttachMessage{Type: nativerunner.StreamFrame,
		Frame: nativerunner.Frame{Rows: []nativerunner.FrameRow{{Index: 1, ANSI: "> world"}}}}
	m, _ = read(t, m, next)
	view = plainView(m)
	if !strings.Contains(view, "$ claude") || !strings.Contains(view, "> world") || strings.Contains(view, "> hello") {
		t.Fatalf("dirty row not applied over the screen:\n%s", view)
	}
}

func TestAttach_TerminalFocusSendsEveryKeyToTheSession(t *testing.T) {
	h := &fakeHolders{attaches: map[string]*fakeAttach{}}
	m, cmd := attachModel(t, h)
	m, _ = dialed(t, m, cmd)
	m = update1(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.InputFocused() {
		t.Fatal("enter in the sidebar did not focus the terminal")
	}
	fa := h.attaches["a"]
	if !slices.Equal(fa.sizes, [][2]int{{80 - sidebarWidth - 1 - 2, 12 - 2}}) {
		t.Fatalf("focus sent sizes %v, want the panel's inner size", fa.sizes)
	}

	presses := []tea.KeyPressMsg{
		{Code: '1', Text: "1"}, {Code: 'g', Text: "g"}, {Code: 'q', Text: "q"},
		{Code: 'j', Text: "j"}, {Code: 'c', Mod: tea.ModCtrl}, {Code: tea.KeyEnter},
	}
	for _, p := range presses {
		next, cmd := m.Update(p)
		m = next.(Model)
		if cmd != nil {
			t.Fatalf("%s returned a cmd in terminal focus", p.String())
		}
	}
	if len(fa.keys) != len(presses) {
		t.Fatalf("sent %d keys, want %d", len(fa.keys), len(presses))
	}
	for i, p := range presses {
		if k := fa.keys[i]; k.Code != p.Code || k.Text != p.Text || k.Mod != p.Mod {
			t.Fatalf("key %d sent as %+v, want %s", i, k, p.String())
		}
	}
	if m.selectedID() != "a" {
		t.Fatal("j moved the sidebar cursor in terminal focus")
	}

	m = update1(t, m, tea.PasteMsg{Content: "pasted"})
	if !slices.Equal(fa.pastes, []string{"pasted"}) {
		t.Fatalf("pastes = %v", fa.pastes)
	}
}

// focusedAttach is attachModel with session a attached and the terminal
// focused.
func focusedAttach(t *testing.T) (Model, *fakeAttach) {
	t.Helper()
	h := &fakeHolders{attaches: map[string]*fakeAttach{}}
	m, cmd := attachModel(t, h)
	m, _ = dialed(t, m, cmd)
	return update1(t, m, tea.KeyPressMsg{Code: tea.KeyEnter}), h.attaches["a"]
}

func TestAttach_PasteOverTheLimitIsRefused(t *testing.T) {
	m, fa := focusedAttach(t)
	next, cmd := m.Update(tea.PasteMsg{Content: strings.Repeat("x", 2<<20)})
	m = next.(Model)
	if len(fa.pastes) != 0 {
		t.Fatal("an oversized paste was sent")
	}
	if cmd == nil {
		t.Fatal("an oversized paste showed no message")
	}
	if n, ok := cmd().(notify.NotifyMsg); !ok || n.Kind != notify.KindWarning {
		t.Fatalf("refusal message = %#v, want a warning", n)
	}

	limit := strings.Repeat("x", maxPasteBytes)
	m = update1(t, m, tea.PasteMsg{Content: limit})
	if !slices.Equal(fa.pastes, []string{limit}) {
		t.Fatal("a paste at the limit was not sent")
	}
}

func TestAttach_CtrlVPassesThroughAsAKey(t *testing.T) {
	m, fa := focusedAttach(t)
	update1(t, m, tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl})
	if len(fa.keys) != 1 || fa.keys[0].Code != 'v' || fa.keys[0].Mod != tea.ModCtrl {
		t.Fatalf("keys = %+v, want ctrl+v", fa.keys)
	}
}

func TestAttach_TypingSendsThePanelSize(t *testing.T) {
	m, fa := focusedAttach(t)
	fa.sizes = nil
	m = update1(t, m, tea.KeyPressMsg{Code: 'x', Text: "x"})
	update1(t, m, tea.PasteMsg{Content: "y"})
	size := [2]int{80 - sidebarWidth - 1 - 2, 12 - 2}
	if !slices.Equal(fa.sizes, [][2]int{size, size}) {
		t.Fatalf("typing sent sizes %v, want the panel's inner size before each input", fa.sizes)
	}
}

func TestAttach_WindowResizeSendsTheSizeOnlyWhileFocused(t *testing.T) {
	h := &fakeHolders{attaches: map[string]*fakeAttach{}}
	m, cmd := attachModel(t, h)
	m, _ = dialed(t, m, cmd)
	fa := h.attaches["a"]
	m = update1(t, m, tea.WindowSizeMsg{Width: 100, Height: 20})
	if len(fa.sizes) != 0 {
		t.Fatalf("an unfocused resize sent sizes %v", fa.sizes)
	}
	m = update1(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	fa.sizes = nil
	update1(t, m, tea.WindowSizeMsg{Width: 90, Height: 30})
	if !slices.Equal(fa.sizes, [][2]int{{90 - sidebarWidth - 1 - 2, 30 - 2}}) {
		t.Fatalf("a focused resize sent sizes %v", fa.sizes)
	}
}

func TestAttach_SwitchingSessionDetachesTheOldOne(t *testing.T) {
	h := &fakeHolders{attaches: map[string]*fakeAttach{}}
	m, cmd := attachModel(t, h)
	m, readA := dialed(t, m, cmd)

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = next.(Model)
	if !h.attaches["a"].closed {
		t.Fatal("switching session left the old attach open")
	}
	// The old stream's last read lands after the switch and must be dropped.
	m, _ = read(t, m, readA)
	m, _ = dialed(t, m, cmd)
	if !slices.Equal(h.dialed, []string{"a", "b"}) {
		t.Fatalf("dialed %v, want a then b", h.dialed)
	}
	if m.attach == nil || m.attach.id != "b" || m.attach.stream != h.attaches["b"] {
		t.Fatal("the new session is not attached")
	}
}

func TestAttach_LeavingTheTabDetachesAndReturningReattaches(t *testing.T) {
	h := &fakeHolders{attaches: map[string]*fakeAttach{}}
	m, cmd := attachModel(t, h)
	m, _ = dialed(t, m, cmd)
	first := h.attaches["a"]

	m.OnPageDeactivated()
	if !first.closed {
		t.Fatal("leaving the tab left the attach open")
	}

	delete(h.attaches, "a")
	next, cmd := m.Update(m.OnPageActivated()())
	m, _ = dialed(t, next.(Model), cmd)
	if h.attaches["a"] == nil || m.attach.stream != h.attaches["a"] {
		t.Fatal("returning to the tab did not reattach")
	}
}

func TestAttach_OlderHolderShowsASnapshot(t *testing.T) {
	h := &fakeHolders{attaches: map[string]*fakeAttach{}, snapshots: map[string][]string{"a": {"old holder screen"}}}
	m, cmd := attachModel(t, h)
	m, read := dialed(t, m, cmd)
	if read != nil {
		t.Fatal("a snapshot has no stream to read")
	}
	if view := plainView(m); !strings.Contains(view, "old holder screen") || strings.Contains(view, "attach failed") {
		t.Fatalf("snapshot not shown:\n%s", view)
	}
}

func TestAttach_ExitSwitchesToTheExitedView(t *testing.T) {
	h := &fakeHolders{attaches: map[string]*fakeAttach{}}
	m, cmd := attachModel(t, h)
	m, next := dialed(t, m, cmd)
	m = update1(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	fa := h.attaches["a"]
	fa.frame("bye")
	m, next = read(t, m, next)
	fa.msgs <- nativerunner.AttachMessage{Type: nativerunner.StreamExit}
	m, next = read(t, m, next)
	if next != nil {
		t.Fatal("the attach kept reading after the exit")
	}
	if m.InputFocused() {
		t.Fatal("terminal focus kept on an exited session")
	}
	if view := plainView(m); !strings.Contains(view, "<leader> r to resume") || !strings.Contains(view, "bye") {
		t.Fatalf("Exited view not shown:\n%s", view)
	}
}
