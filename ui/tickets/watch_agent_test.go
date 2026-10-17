package tickets

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/agentlog"
	"github.com/elentok/gx/server"
)

const watchLogLine = `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hello from the agent"}]}}` + "\n"

func fakeNativeLog(t *testing.T, log agentlog.Log, err error) {
	t.Helper()
	prev := nativeAgentLog
	t.Cleanup(func() { nativeAgentLog = prev })
	nativeAgentLog = func(context.Context, string) (agentlog.Log, error) { return log, err }
}

func TestWatchAgent_RendersLogInModal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.jsonl")
	if err := os.WriteFile(path, []byte(watchLogLine), 0o644); err != nil {
		t.Fatal(err)
	}
	fakeNativeLog(t, agentlog.Log{Path: path}, nil)

	loaded, ok := cmdWatchAgent("gx:epic/01")().(watchLoadedMsg)
	if !ok {
		t.Fatal("expected watchLoadedMsg")
	}
	modal := watchModal{}.Open(loaded, 120, 40)
	if view := modal.View(); !strings.Contains(view, "hello from the agent") {
		t.Errorf("modal missing the rendered log:\n%s", view)
	}
}

func watchLine(text string) string {
	return strings.Replace(watchLogLine, "hello from the agent", text, 1)
}

func appendWatchLog(t *testing.T, path string, texts ...string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, text := range texts {
		if _, err := f.WriteString(watchLine(text)); err != nil {
			t.Fatal(err)
		}
	}
}

// openLiveWatch opens a live Watch on the Tickets tab over a log holding
// texts; *ended reports the agent's end.
func openLiveWatch(t *testing.T, texts ...string) (m Model, path string, ended *bool, tick tea.Cmd) {
	t.Helper()
	path = filepath.Join(t.TempDir(), "out.jsonl")
	appendWatchLog(t, path, texts...)
	ended = new(bool)
	fakeNativeLog(t, agentlog.Log{Path: path, Ended: func() bool { return *ended }}, nil)
	m = newServerModel(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	next, tick = next.(Model).Update(cmdWatchAgent("gx:epic/01")())
	t.Cleanup(func() {
		if m.watch.tail != nil {
			m.watch.tail.Close()
		}
	})
	return next.(Model), path, ended, tick
}

// runWatchTick delivers one tick's step to m.
func runWatchTick(t *testing.T, m Model, tick tea.Cmd) (Model, tea.Cmd) {
	t.Helper()
	if tick == nil {
		t.Fatal("no tick armed")
	}
	next, cmd := m.Update(tick())
	return next.(Model), cmd
}

func pressWatch(m Model, k string) Model {
	code := []rune(k)[0]
	next, _ := m.Update(tea.KeyPressMsg{Code: code, Text: k})
	return next.(Model)
}

func manyTexts(prefix string, n int) []string {
	texts := make([]string, n)
	for i := range texts {
		texts[i] = fmt.Sprintf("%s %d", prefix, i)
	}
	return texts
}

func TestWatchLive_FollowsNewLinesAtBottom(t *testing.T) {
	m, path, _, tick := openLiveWatch(t, "hello from the agent")
	if view := m.watch.View(); !strings.Contains(view, "· live") {
		t.Errorf("header missing live state:\n%s", view)
	}
	appendWatchLog(t, path, "a newer line")
	m, tick = runWatchTick(t, m, tick)
	if view := m.watch.View(); !strings.Contains(view, "a newer line") {
		t.Errorf("modal missing the appended line:\n%s", view)
	}
	if tick == nil {
		t.Error("tick not re-armed while the agent runs")
	}
}

func TestWatchLive_ScrolledUpStaysPutUntilG(t *testing.T) {
	m, path, _, tick := openLiveWatch(t, manyTexts("old", 60)...)
	m = pressWatch(m, "k")
	off := m.watch.vp.YOffset()

	appendWatchLog(t, path, "fresh line")
	m, tick = runWatchTick(t, m, tick)
	if got := m.watch.vp.YOffset(); got != off {
		t.Errorf("scrolled-up view moved: offset %d, want %d", got, off)
	}
	if view := m.watch.View(); !strings.Contains(view, "↓ new lines") {
		t.Errorf("missing new-lines mark:\n%s", view)
	}

	m = pressWatch(m, "G")
	if view := m.watch.View(); strings.Contains(view, "↓ new lines") || !strings.Contains(view, "fresh line") {
		t.Errorf("G did not return to the newest line:\n%s", view)
	}
	appendWatchLog(t, path, "after G")
	m, _ = runWatchTick(t, m, tick)
	if view := m.watch.View(); !strings.Contains(view, "after G") {
		t.Errorf("G did not resume following:\n%s", view)
	}
}

func TestWatchLive_NoRefreshOrStepKeys(t *testing.T) {
	m, _, _, _ := openLiveWatch(t, "hello from the agent")
	for _, k := range []string{"r", "[", "]"} {
		if _, cmd := m.watch.Update(tea.KeyPressMsg{Code: []rune(k)[0], Text: k}); cmd != nil {
			t.Errorf("%q returned a command in Watch mode", k)
		}
	}
}

func TestWatchLive_AgentEndedStopsTick(t *testing.T) {
	m, path, ended, tick := openLiveWatch(t, "hello from the agent")
	appendWatchLog(t, path, "last words")
	*ended = true
	m, next := runWatchTick(t, m, tick)
	view := m.watch.View()
	if !strings.Contains(view, "agent ended") || !strings.Contains(view, "last words") {
		t.Errorf("ended modal missing state or final line:\n%s", view)
	}
	if next != nil {
		t.Error("tick re-armed after the log ended")
	}
}

func TestWatchLive_CloseStopsTick(t *testing.T) {
	m, _, _, tick := openLiveWatch(t, "hello from the agent")
	m = pressWatch(m, "q")
	if m.watch.open {
		t.Fatal("q did not close the modal")
	}
	if _, next := runWatchTick(t, m, tick); next != nil {
		t.Error("tick re-armed after the modal closed")
	}
}

func TestWatchModal_KeepsNewestLines(t *testing.T) {
	w := watchModal{}.Open(watchLoadedMsg{lines: manyTexts("old", maxWatchLines)}, 120, 40)
	w = w.appendLines(manyTexts("new", 500))
	if len(w.lines) != maxWatchLines || w.lines[len(w.lines)-1] != "new 499" || w.lines[0] != "old 500" {
		t.Errorf("kept %d lines, first %q, last %q", len(w.lines), w.lines[0], w.lines[len(w.lines)-1])
	}
}

func TestQueueWatch_OpensLiveModal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.jsonl")
	appendWatchLog(t, path, "hello from the queue")
	fakeNativeLog(t, agentlog.Log{Path: path, Ended: func() bool { return false }}, nil)

	m := newServerQueueModel(t, nil)
	m = selectFirstQueueTicketRow(t, m)
	_, cmd := chooseQueueMenuItem(t, openQueueMenu(t, m), "Watch")
	if cmd == nil {
		t.Fatal("Watch returned no Cmd")
	}
	next, tick := m.Update(cmd())
	m = next.(QueueModel)
	t.Cleanup(func() { m.watch.tail.Close() })
	if view := m.watch.View(); !m.watch.open || !strings.Contains(view, "hello from the queue") || !strings.Contains(view, "· live") {
		t.Errorf("Queue Watch did not open the live modal:\n%s", view)
	}
	if tick == nil {
		t.Error("Queue Watch armed no tick")
	}
}

func TestWatchAgent_PtyShowsConversation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "transcript.jsonl")
	appendWatchLog(t, path, "pty conversation")
	fakeNativeLog(t, agentlog.Log{Path: path, PtyDir: "/pty/dir"}, nil)

	loaded := cmdWatchAgent("gx:epic/01")().(watchLoadedMsg)
	t.Cleanup(func() { loaded.tail.Close() })
	if view := (watchModal{}).Open(loaded, 120, 40).View(); !strings.Contains(view, "pty conversation") {
		t.Errorf("modal missing the pty agent's conversation:\n%s", view)
	}
}

func TestWatchAgent_PrunedLogPointsAtTranscript(t *testing.T) {
	fakeNativeLog(t, agentlog.Log{Pruned: true, Transcript: "/t/x.jsonl"}, nil)
	loaded := cmdWatchAgent("gx:epic/01")().(watchLoadedMsg)
	if got := strings.Join(loaded.lines, "\n"); !strings.Contains(got, "/t/x.jsonl") {
		t.Errorf("lines = %q", got)
	}
}

func TestWatch_NativeLive_OpensLogNotPane(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.jsonl")
	if err := os.WriteFile(path, []byte(watchLogLine), 0o644); err != nil {
		t.Fatal(err)
	}
	fakeNativeLog(t, agentlog.Log{Path: path, Ended: func() bool { return false }}, nil)

	cmd := cmdServerWatch(nil, "gx:epic/01")
	if _, ok := cmd().(watchLoadedMsg); !ok {
		t.Error("expected the native log modal")
	}
}

// failOnNativeLog fails the test if anything resolves native liveness before
// the returned undo runs, i.e. on the Update goroutine.
func failOnNativeLog(t *testing.T) (undo func()) {
	t.Helper()
	prev := nativeAgentLog
	nativeAgentLog = func(context.Context, string) (agentlog.Log, error) {
		t.Error("native liveness resolved on the Update goroutine")
		return agentlog.Log{}, agentlog.ErrNoAgent
	}
	return func() { nativeAgentLog = prev }
}

func TestWatch_ResolvesLivenessInCmd(t *testing.T) {
	fakeNativeLog(t, agentlog.Log{}, agentlog.ErrNoAgent)
	undo := failOnNativeLog(t)
	cmd := cmdServerWatch(fakeServerAPI{}, "gx:epic/01")
	undo()
	cmd()
}

func TestMenuKey_ResolvesLivenessInCmd(t *testing.T) {
	fakeNativeLog(t, agentlog.Log{}, agentlog.ErrNoAgent)
	m := newServerModel(t).WithServer(fakeServerAPI{})
	m = withState(m, server.Snapshot{Tickets: []server.TicketInfo{{Address: "gx:alpha/01", Title: "First", Status: "claimed"}}})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = selectTicketRow(t, updated.(Model))

	undo := failOnNativeLog(t)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	undo()
	if updated.(Model).ticketMenu.IsOpen {
		t.Fatal("menu opened before liveness resolved")
	}
	if cmd == nil {
		t.Fatal("m issued no cmd")
	}
	updated, _ = updated.(Model).Update(cmd())
	if !updated.(Model).ticketMenu.IsOpen {
		t.Error("menu closed after liveness resolved, want open")
	}
}

// requestMenu selects the first of two tickets and presses "m", returning the
// model and the not-yet-delivered ready message.
func requestMenu(t *testing.T) (Model, tea.Msg) {
	t.Helper()
	fakeNativeLog(t, agentlog.Log{}, agentlog.ErrNoAgent)
	m := newServerModel(t).WithServer(fakeServerAPI{})
	m = withState(m, server.Snapshot{Tickets: []server.TicketInfo{
		{Address: "gx:alpha/01", Title: "First", Status: "claimed"},
		{Address: "gx:alpha/02", Title: "Second", Status: "claimed"},
	}})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = selectTicketRow(t, updated.(Model))
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	return updated.(Model), cmd()
}

func TestMenuReady_DroppedWhenStale(t *testing.T) {
	cases := map[string]tea.KeyPressMsg{
		"esc":              {Code: tea.KeyEscape},
		"selection change": {Code: 'j', Text: "j"},
		"help open":        {Code: '?', Text: "?"},
	}
	for name, key := range cases {
		t.Run(name, func(t *testing.T) {
			m, ready := requestMenu(t)
			updated, _ := m.Update(key)
			updated, _ = updated.(Model).Update(ready)
			if updated.(Model).ticketMenu.IsOpen {
				t.Error("stale ready message opened the menu")
			}
		})
	}
}

func TestMenuReady_DroppedAfterLeavingAndReturning(t *testing.T) {
	m, ready := requestMenu(t)
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	updated, _ = updated.(Model).Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	updated, _ = updated.(Model).Update(ready)
	if updated.(Model).ticketMenu.IsOpen {
		t.Error("ready message opened the menu after the selection moved away")
	}
}

func TestMenuReady_DeliveredOnce(t *testing.T) {
	m, ready := requestMenu(t)
	updated, _ := m.Update(ready)
	updated, _ = updated.(Model).Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	updated, _ = updated.(Model).Update(ready)
	if updated.(Model).ticketMenu.IsOpen {
		t.Error("a second delivery of the same ready message reopened the menu")
	}
}

func TestNativeAgentLive(t *testing.T) {
	alive := func() bool { return false }
	ended := func() bool { return true }
	cases := []struct {
		name string
		log  agentlog.Log
		err  error
		want bool
	}{
		{"no native agent", agentlog.Log{}, agentlog.ErrNoAgent, false},
		{"running", agentlog.Log{Path: "x", Ended: alive}, nil, true},
		{"ended", agentlog.Log{Path: "x", Ended: ended}, nil, false},
		{"pruned", agentlog.Log{Pruned: true}, nil, false},
	}
	for _, c := range cases {
		fakeNativeLog(t, c.log, c.err)
		if got := nativeAgentLive("gx:epic/01"); got != c.want {
			t.Errorf("%s: nativeAgentLive = %v, want %v", c.name, got, c.want)
		}
	}
}
