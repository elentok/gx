package agents

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

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
	// log orders the keys and scrolls sent.
	log []string
	// stall, when set, holds every key until it's closed: a holder that
	// stopped reading.
	stall chan struct{}
	// keyErr fails every key write.
	keyErr error

	mu     sync.Mutex
	closed bool
}

func newFakeAttach() *fakeAttach {
	return &fakeAttach{msgs: make(chan nativerunner.AttachMessage, 16), done: make(chan struct{})}
}

func (f *fakeAttach) isClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

func (f *fakeAttach) Next() (nativerunner.AttachMessage, error) {
	select {
	case msg := <-f.msgs:
		return msg, nil
	case <-f.done:
		return nativerunner.AttachMessage{}, io.EOF
	}
}

func (f *fakeAttach) SendKey(k screen.Key) error {
	if f.stall != nil {
		<-f.stall
	}
	if f.keyErr != nil {
		return f.keyErr
	}
	f.keys = append(f.keys, k)
	f.log = append(f.log, "key:"+k.Text)
	return nil
}

func (f *fakeAttach) Paste(text string) error { f.pastes = append(f.pastes, text); return nil }

func (f *fakeAttach) Scroll(offset int) error {
	f.log = append(f.log, "scroll:"+strconv.Itoa(offset))
	return nil
}
func (f *fakeAttach) Copy(start, end nativerunner.LinePos) error {
	f.log = append(f.log, fmt.Sprintf("copy:%d.%d-%d.%d", start.Line, start.Col, end.Line, end.Col))
	return nil
}

func (f *fakeAttach) Resize(cols, rows int) error {
	f.sizes = append(f.sizes, [2]int{cols, rows})
	return nil
}

func (f *fakeAttach) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
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
	// down lists the sessions whose holder socket doesn't exist yet.
	down   map[string]bool
	dialed []string
}

func (h *fakeHolders) dial(dir string) (AttachStream, []string, error) {
	id := filepath.Base(dir)
	h.dialed = append(h.dialed, id)
	if h.down[id] {
		return nil, nil, errors.New("no holder")
	}
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
	return withStatusOfA(m, idleStatus)
}

// withStatusOfA delivers a snapshot of sessions a (in status) and b (idle).
func withStatusOfA(m Model, status server.SessionStatus) (Model, tea.Cmd) {
	st := &viewmodel.State{Sessions: []server.SessionInfo{
		{ID: "a", Name: "alpha", Project: "gx", Status: status},
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
	m = settle(m)
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
	m = settle(next.(Model))
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

func TestAttach_TypingSendsNoResize(t *testing.T) {
	m, fa := focusedAttach(t)
	fa.sizes = nil
	m = pressKeys(t, m, letters("xyz")...)
	update1(t, m, tea.PasteMsg{Content: "p"})
	if len(fa.sizes) != 0 {
		t.Fatalf("typing sent sizes %v, want none", fa.sizes)
	}
}

func TestAttach_RefocusSendsTheSizeAgain(t *testing.T) {
	m, fa := focusedAttach(t)
	m = pressKeys(t, m, tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl}, tea.KeyPressMsg{Code: tea.KeyTab})
	m = pressKeys(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	size := [2]int{80 - sidebarWidth - 1 - 2, 12 - 2}
	if !slices.Equal(fa.sizes, [][2]int{size, size}) {
		t.Fatalf("focusing twice sent sizes %v, want the size on each focus", fa.sizes)
	}
}

func TestAttach_HidingTheSidebarWhileFocusedSendsTheNewSize(t *testing.T) {
	m, fa := focusedAttach(t)
	fa.sizes = nil
	pressKeys(t, m, tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl}, letter("b"))
	if !slices.Equal(fa.sizes, [][2]int{{80 - 2, 12 - 2}}) {
		t.Fatalf("hiding the sidebar sent sizes %v, want the full width", fa.sizes)
	}
}

func TestAttach_StalledHolderDoesNotBlockKeys(t *testing.T) {
	m, fa := focusedAttach(t)
	fa.stall = make(chan struct{})
	done := make(chan Model)
	go func() {
		mm := m
		for _, r := range "abc" {
			next, _ := mm.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
			mm = next.(Model)
		}
		done <- mm
	}()
	select {
	case m = <-done:
	case <-time.After(time.Second):
		t.Fatal("keys to a stalled holder blocked Update")
	}
	if !strings.Contains(plainView(m), "alpha") {
		t.Fatal("the panel did not render while the holder stalled")
	}

	close(fa.stall)
	m = update1(t, m, letter("d"))
	if got := strings.Join(fa.log, ","); got != "key:a,key:b,key:c,key:d" {
		t.Fatalf("keys after the stall = %s, want a,b,c,d in order", got)
	}
}

func TestAttach_WriteErrorEndsTheAttach(t *testing.T) {
	h := &fakeHolders{attaches: map[string]*fakeAttach{}}
	m, cmd := attachModel(t, h)
	m, next := dialed(t, m, cmd)
	m = update1(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	fa := h.attaches["a"]
	fa.keyErr = errors.New("broken pipe")
	m = update1(t, m, letter("x"))
	if !fa.isClosed() {
		t.Fatal("a failed write left the stream open")
	}
	m, _ = read(t, m, next)
	if !m.attach.ended {
		t.Fatal("a failed write did not end the attach")
	}
	if view := plainView(m); !strings.Contains(view, "broken pipe") {
		t.Fatalf("the write error is not shown:\n%s", view)
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

var startingStatus = server.SessionStatus{State: server.SessionStarting}

// startingModel shows session a Starting over holders where a's is down.
func startingModel(t *testing.T) (Model, tea.Cmd, *fakeHolders) {
	t.Helper()
	h := &fakeHolders{attaches: map[string]*fakeAttach{}, down: map[string]bool{"a": true}}
	m := NewModel(&fakeSessionClient{}, ui.Settings{})
	m.sessionsDir = "/sessions"
	m.dial = h.dial
	m = update1(t, m, tea.WindowSizeMsg{Width: 80, Height: 12})
	m, cmd := withStatusOfA(m, startingStatus)
	return m, cmd, h
}

func TestAttach_StartingSessionRetriesUntilTheHolderAppears(t *testing.T) {
	m, cmd, h := startingModel(t)
	m, retry := dialed(t, m, cmd)
	if retry == nil {
		t.Fatal("a failed dial of a Starting session was not retried")
	}
	if view := plainView(m); strings.Contains(view, "attach failed") {
		t.Fatalf("attach failed shown while retrying:\n%s", view)
	}

	h.down["a"] = false
	next, cmd := m.Update(attachRetryMsg{a: m.attach})
	m, _ = dialed(t, next.(Model), cmd)
	h.attaches["a"].frame("$ claude")
	m, _ = read(t, m, readAttach(m.attach))
	if view := plainView(m); !strings.Contains(view, "$ claude") || strings.Contains(view, "attach failed") {
		t.Fatalf("the retry did not attach:\n%s", view)
	}
}

func TestAttach_RetriesAreBounded(t *testing.T) {
	m, cmd, h := startingModel(t)
	m, retry := dialed(t, m, cmd)
	for retry != nil {
		next, cmd := m.Update(attachRetryMsg{a: m.attach})
		m, retry = dialed(t, next.(Model), cmd)
	}
	if len(h.dialed) != 1+maxAttachRetries {
		t.Fatalf("dialed %d times, want %d", len(h.dialed), 1+maxAttachRetries)
	}
	if view := plainView(m); !strings.Contains(view, "attach failed: no holder") {
		t.Fatalf("giving up did not show the error:\n%s", view)
	}
}

func TestAttach_StaleRetryIsDropped(t *testing.T) {
	m, cmd, h := startingModel(t)
	m, _ = dialed(t, m, cmd)
	stale := attachRetryMsg{a: m.attach}
	m = update1(t, m, tea.KeyPressMsg{Code: 'j', Text: "j"})
	if _, cmd := m.Update(stale); cmd != nil {
		t.Fatal("a retry of a session no longer on screen redialed")
	}
	if !slices.Equal(h.dialed, []string{"a"}) {
		t.Fatalf("dialed %v", h.dialed)
	}
}

func TestAttach_FailedAttachRedialsWhenTheSessionChanges(t *testing.T) {
	h := &fakeHolders{attaches: map[string]*fakeAttach{}, down: map[string]bool{"a": true}}
	m, cmd := attachModel(t, h)
	m, retry := dialed(t, m, cmd)
	if retry != nil {
		t.Fatal("a failed dial of an Idle session was retried")
	}

	m, cmd = withStatusOfA(m, idleStatus)
	if cmd != nil {
		t.Fatal("an unchanged session redialed")
	}

	h.down["a"] = false
	m, cmd = withStatusOfA(m, startingStatus)
	m, _ = dialed(t, m, cmd)
	if m.attach.stream != h.attaches["a"] {
		t.Fatal("a session change did not redial the failed attach")
	}
}

func TestAttach_ExitedAttachIsNotRedialedOnSessionUpdate(t *testing.T) {
	h := &fakeHolders{attaches: map[string]*fakeAttach{}}
	m, cmd := attachModel(t, h)
	m, next := dialed(t, m, cmd)
	h.attaches["a"].msgs <- nativerunner.AttachMessage{Type: nativerunner.StreamExit}
	m, _ = read(t, m, next)

	if _, cmd := withStatusOfA(m, startingStatus); cmd != nil {
		t.Fatal("a session update redialed an exited attach")
	}
	if !slices.Equal(h.dialed, []string{"a"}) {
		t.Fatalf("dialed %v", h.dialed)
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
