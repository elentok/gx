package rowactions

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/agentlog"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/terminalrun"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
	"github.com/elentok/gx/ui/tickets/tabapi"
)

const watchLogLine = `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hello from the agent"}]}}` + "\n"

func fakeNativeLog(t *testing.T, log agentlog.Log, err error) {
	t.Helper()
	fakeAgentLog(t, func(context.Context, config.Dirs, string, HerdrLookup) (agentlog.Log, error) {
		return log, err
	})
}

func fakeAgentLog(t *testing.T, f func(context.Context, config.Dirs, string, HerdrLookup) (agentlog.Log, error)) {
	t.Helper()
	prev := AgentLog
	t.Cleanup(func() { AgentLog = prev })
	AgentLog = f
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

// watchLogFile writes a one-line agent log and returns its path.
func watchLogFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "out.jsonl")
	appendWatchLog(t, path, "hello from the agent")
	return path
}

func endedFunc(ended bool) func() bool { return func() bool { return ended } }

func manyTexts(prefix string, n int) []string {
	texts := make([]string, n)
	for i := range texts {
		texts[i] = fmt.Sprintf("%s %d", prefix, i)
	}
	return texts
}

func TestWatchModal_KeepsNewestLines(t *testing.T) {
	w := WatchModal{}.Open(WatchLoadedMsg{lines: manyTexts("old", maxWatchLines)}, 120, 40)
	w = w.appendLines(manyTexts("new", 500))
	if len(w.lines) != maxWatchLines || w.lines[len(w.lines)-1] != "new 499" || w.lines[0] != "old 500" {
		t.Errorf("kept %d lines, first %q, last %q", len(w.lines), w.lines[0], w.lines[len(w.lines)-1])
	}
}

func TestWatchAgent_RendersLogInModal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.jsonl")
	if err := os.WriteFile(path, []byte(watchLogLine), 0o644); err != nil {
		t.Fatal(err)
	}
	fakeNativeLog(t, agentlog.Log{Path: path}, nil)

	loaded, ok := CmdWatchAgent(nil, config.Dirs{}, "gx:epic/01")().(WatchLoadedMsg)
	if !ok {
		t.Fatal("expected WatchLoadedMsg")
	}
	modal := WatchModal{}.Open(loaded, 120, 40)
	t.Cleanup(modal.Close)
	if view := modal.View(); !strings.Contains(view, "hello from the agent") {
		t.Errorf("modal missing the rendered log:\n%s", view)
	}
}

func TestWatchAgent_PtyShowsConversation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "transcript.jsonl")
	appendWatchLog(t, path, "pty conversation")
	fakeNativeLog(t, agentlog.Log{Path: path, PtyDir: "/pty/dir"}, nil)

	loaded := CmdWatchAgent(nil, config.Dirs{}, "gx:epic/01")().(WatchLoadedMsg)
	modal := WatchModal{}.Open(loaded, 120, 40)
	t.Cleanup(modal.Close)
	if view := modal.View(); !strings.Contains(view, "pty conversation") {
		t.Errorf("modal missing the pty agent's conversation:\n%s", view)
	}
}

// openLiveWatch opens a live Watch modal over a log holding texts; *ended
// reports the agent's end.
func openLiveWatch(t *testing.T, texts ...string) (w WatchModal, path string, ended *bool, tick tea.Cmd) {
	t.Helper()
	path = filepath.Join(t.TempDir(), "out.jsonl")
	appendWatchLog(t, path, texts...)
	ended = new(bool)
	fakeNativeLog(t, agentlog.Log{Path: path, Ended: func() bool { return *ended }}, nil)
	loaded := CmdWatchAgent(nil, config.Dirs{}, "gx:epic/01")().(WatchLoadedMsg)
	w = WatchModal{}.Open(loaded, 120, 40)
	t.Cleanup(w.Close)
	return w, path, ended, w.Tick()
}

// runWatchTick delivers one tick's step to w.
func runWatchTick(t *testing.T, w WatchModal, tick tea.Cmd) (WatchModal, tea.Cmd) {
	t.Helper()
	if tick == nil {
		t.Fatal("no tick armed")
	}
	return w.Step(tick().(WatchStepMsg))
}

func pressWatch(w WatchModal, k string) WatchModal {
	w, _ = w.Update(tea.KeyPressMsg{Code: []rune(k)[0], Text: k})
	return w
}

func TestWatchLive_FollowsNewLinesAtBottom(t *testing.T) {
	w, path, _, tick := openLiveWatch(t, "hello from the agent")
	if view := w.View(); !strings.Contains(view, "· live") {
		t.Errorf("header missing live state:\n%s", view)
	}
	appendWatchLog(t, path, "a newer line")
	w, tick = runWatchTick(t, w, tick)
	if view := w.View(); !strings.Contains(view, "a newer line") {
		t.Errorf("modal missing the appended line:\n%s", view)
	}
	if tick == nil {
		t.Error("tick not re-armed while the agent runs")
	}
}

func TestWatchLive_ScrolledUpStaysPutUntilG(t *testing.T) {
	w, path, _, tick := openLiveWatch(t, manyTexts("old", 60)...)
	w = pressWatch(w, "k")
	off := w.YOffset()

	appendWatchLog(t, path, "fresh line")
	w, tick = runWatchTick(t, w, tick)
	if got := w.YOffset(); got != off {
		t.Errorf("scrolled-up view moved: offset %d, want %d", got, off)
	}
	if view := w.View(); !strings.Contains(view, "↓ new lines") {
		t.Errorf("missing new-lines mark:\n%s", view)
	}

	w = pressWatch(w, "G")
	if view := w.View(); strings.Contains(view, "↓ new lines") || !strings.Contains(view, "fresh line") {
		t.Errorf("G did not return to the newest line:\n%s", view)
	}
	appendWatchLog(t, path, "after G")
	w, _ = runWatchTick(t, w, tick)
	if view := w.View(); !strings.Contains(view, "after G") {
		t.Errorf("G did not resume following:\n%s", view)
	}
}

func TestWatchLive_NoRefreshOrStepKeys(t *testing.T) {
	w, _, _, _ := openLiveWatch(t, "hello from the agent")
	for _, k := range []string{"r", "[", "]"} {
		if _, cmd := w.Update(tea.KeyPressMsg{Code: []rune(k)[0], Text: k}); cmd != nil {
			t.Errorf("%q returned a command in Watch mode", k)
		}
	}
}

func TestWatchLive_AgentEndedStopsTick(t *testing.T) {
	w, path, ended, tick := openLiveWatch(t, "hello from the agent")
	appendWatchLog(t, path, "last words")
	*ended = true
	w, next := runWatchTick(t, w, tick)
	view := w.View()
	if !strings.Contains(view, "agent ended") || !strings.Contains(view, "last words") {
		t.Errorf("ended modal missing state or final line:\n%s", view)
	}
	if next != nil {
		t.Error("tick re-armed after the log ended")
	}
}

func TestWatchLive_CloseStopsTick(t *testing.T) {
	w, _, _, tick := openLiveWatch(t, "hello from the agent")
	w = pressWatch(w, "q")
	if w.IsOpen() {
		t.Fatal("q did not close the modal")
	}
	if _, next := runWatchTick(t, w, tick); next != nil {
		t.Error("tick re-armed after the modal closed")
	}
}

// failOnNativeLog fails the test if anything resolves native liveness before
// the returned undo runs, i.e. on the Update goroutine.
func failOnNativeLog(t *testing.T) (undo func()) {
	t.Helper()
	prev := AgentLog
	AgentLog = func(context.Context, config.Dirs, string, HerdrLookup) (agentlog.Log, error) {
		t.Error("native liveness resolved on the Update goroutine")
		return agentlog.Log{}, agentlog.ErrNoAgent
	}
	return func() { AgentLog = prev }
}

func TestWatch_ResolvesLivenessInCmd(t *testing.T) {
	fakeNativeLog(t, agentlog.Log{}, agentlog.ErrNoAgent)
	undo := failOnNativeLog(t)
	cmd := CmdWatchAgent(tickettest.FakeReads{}, config.Dirs{}, "gx:epic/01")
	undo()
	cmd()
}

func TestWatchReason_NativeLog(t *testing.T) {
	cases := []struct {
		name string
		log  agentlog.Log
		err  error
		want string
	}{
		{"no native agent", agentlog.Log{}, agentlog.ErrNoAgent, NoAgentLog},
		{"running", agentlog.Log{Path: "x", Ended: endedFunc(false)}, nil, ""},
		{"ended", agentlog.Log{Path: "x", Ended: endedFunc(true)}, nil, ""},
		{"pruned", agentlog.Log{Pruned: true}, nil, NoAgentLog},
	}
	for _, c := range cases {
		fakeNativeLog(t, c.log, c.err)
		if got := WatchReason(tickettest.FakeReads{}, config.Dirs{}, "gx:epic/01"); got != c.want {
			t.Errorf("%s: WatchReason = %q, want %q", c.name, got, c.want)
		}
	}
}

// fakeTab is a tab with one selection and no modal; the yank and edit hooks
// are unused here.
type fakeTab struct {
	env Env
	sel Selection
}

func (f fakeTab) Env() Env                             { return f.env }
func (f fakeTab) Selection() Selection                 { return f.sel }
func (fakeTab) ModalOpen() bool                        { return false }
func (fakeTab) YankSummary() tea.Cmd                   { return nil }
func (fakeTab) YankFilePath() tea.Cmd                  { return nil }
func (fakeTab) EditFile(terminalrun.SplitType) tea.Cmd { return nil }

// serverTab is gx:alpha/01 in status, selected beside an open sibling (which
// keeps a done epic expanded), served by api over a live link.
func serverTab(status string, api tickettest.FakeServerAPI) fakeTab {
	first := tickets.Ticket{Number: 1, Identifier: "01", Path: "gx:alpha/01", Title: "First", Status: status}
	second := tickets.Ticket{Number: 2, Identifier: "02", Path: "gx:alpha/02", Title: "Second", Status: "open"}
	return fakeTab{
		env: Env{
			ServerAPI: api, Admin: tickettest.FakeAdmin{FakeServerAPI: api}, Reads: tickettest.FakeReads{FakeServerAPI: api},
			Tickets: api, Queue: tickettest.FakeQueue{FakeServerAPI: api}, Link: tabapi.ServerLinkUp,
		},
		sel: Selection{OK: true, Epic: tickets.Epic{Name: "gx:alpha", Path: "gx:alpha", Tickets: []tickets.Ticket{first, second}}, Ticket: first},
	}
}

// offlineTab is gx:my-epic/01 with no server.
func offlineTab() fakeTab {
	tk := tickets.Ticket{Number: 1, Identifier: "01", Path: "gx:my-epic/01", Title: "First ticket", Status: "open"}
	return fakeTab{
		env: Env{Link: tabapi.ServerLinkDown},
		sel: Selection{OK: true, Epic: tickets.Epic{Name: "gx:my-epic", Path: "gx:my-epic", Tickets: []tickets.Ticket{tk}}, Ticket: tk},
	}
}

// requestTicketMenu presses "m" on tab and returns the ready message its cmd
// resolves.
func requestTicketMenu(t *testing.T, rm *Menus, tab Tab) TicketReadyMsg {
	t.Helper()
	cmd := rm.HandleKey(tab)
	if cmd == nil {
		t.Fatal("m issued no cmd")
	}
	ready, ok := cmd().(TicketReadyMsg)
	if !ok {
		t.Fatalf("m resolved to %T, want TicketReadyMsg", ready)
	}
	return ready
}

// pickTicketMenuItem runs the menu item label and returns the Cmd the tab runs
// for it; nil when the item is dim.
func pickTicketMenuItem(t *testing.T, tab Tab, ready TicketReadyMsg, label string) tea.Cmd {
	t.Helper()
	item := itemByLabel(t, ready.Items, label)
	if item.Disabled != "" {
		return nil
	}
	msg, ok := item.Run(ready.Target)().(TicketPickMsg)
	if !ok {
		t.Fatalf("%s picked %T, want TicketPickMsg", label, item.Run(ready.Target)())
	}
	var rm Menus
	return rm.HandleTicketPick(msg, tab).Cmd
}

// openWatchFromMenu opens the Context menu and picks Watch, returning the
// modal it loads.
func openWatchFromMenu(t *testing.T, tab Tab) WatchModal {
	t.Helper()
	var rm Menus
	cmd := pickTicketMenuItem(t, tab, requestTicketMenu(t, &rm, tab), "Watch")
	if cmd == nil {
		t.Fatal("Watch is dim or returned no Cmd")
	}
	loaded, ok := cmd().(WatchLoadedMsg)
	if !ok {
		t.Fatalf("Watch resolved to %T, want WatchLoadedMsg", cmd())
	}
	w := WatchModal{}.Open(loaded, 120, 40)
	t.Cleanup(w.Close)
	return w
}

func TestMenuKey_ResolvesLivenessInCmd(t *testing.T) {
	fakeNativeLog(t, agentlog.Log{}, agentlog.ErrNoAgent)
	tab := serverTab("claimed", tickettest.FakeServerAPI{})

	undo := failOnNativeLog(t)
	var rm Menus
	cmd := rm.HandleKey(tab)
	undo()
	if rm.Menu.IsOpen {
		t.Fatal("menu opened before liveness resolved")
	}
	if cmd == nil {
		t.Fatal("m issued no cmd")
	}
	rm.HandleReady(cmd().(TicketReadyMsg), tab)
	if !rm.Menu.IsOpen {
		t.Error("menu closed after liveness resolved, want open")
	}
}

func TestWatch_EndedAgentOpensNotLive(t *testing.T) {
	for _, status := range []string{"claimed", "done"} {
		t.Run(status, func(t *testing.T) {
			fakeNativeLog(t, agentlog.Log{Path: watchLogFile(t), Ended: endedFunc(true)}, nil)
			w := openWatchFromMenu(t, serverTab(status, tickettest.FakeServerAPI{}))
			if view := w.View(); !w.IsOpen() || !strings.Contains(view, "· not live") || !strings.Contains(view, "hello from the agent") {
				t.Errorf("Watch did not open the ended log not live:\n%s", view)
			}
		})
	}
}

func TestWatch_CodexPtyFollowsTheRollout(t *testing.T) {
	rollout := filepath.Join(t.TempDir(), "rollout.jsonl")
	line := `{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"text":"codex says hi"}]}}` + "\n" +
		`{"type":"response_item","payload":{"type":"message","role":"developer","content":[{"text":"hidden developer text"}]}}` + "\n"
	if err := os.WriteFile(rollout, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct {
		ended  bool
		marker string
	}{
		"live":  {false, "· live"},
		"ended": {true, "· not live"},
	} {
		t.Run(name, func(t *testing.T) {
			fakeNativeLog(t, agentlog.Log{Path: rollout, PtyDir: "/pty/dir", Ended: endedFunc(c.ended), Render: agentlog.RenderCodex}, nil)
			w := openWatchFromMenu(t, serverTab("claimed", tickettest.FakeServerAPI{}))
			view := w.View()
			if !w.IsOpen() || !strings.Contains(view, c.marker) || !strings.Contains(view, "[text] codex says hi") || strings.Contains(view, "hidden developer") {
				t.Errorf("Watch on a codex pty agent (%s):\n%s", name, view)
			}
		})
	}
}

func TestWatch_NoServerOpensHeadlessAndPty(t *testing.T) {
	for name, log := range map[string]agentlog.Log{
		"headless": {Ended: endedFunc(false)},
		"pty":      {PtyDir: "/pty/dir", Ended: endedFunc(false)},
	} {
		t.Run(name, func(t *testing.T) {
			log.Path = watchLogFile(t)
			fakeAgentLog(t, func(_ context.Context, _ config.Dirs, address string, _ HerdrLookup) (agentlog.Log, error) {
				if !strings.HasSuffix(address, ":my-epic/01") {
					return agentlog.Log{}, agentlog.ErrNoAgent
				}
				return log, nil
			})
			w := openWatchFromMenu(t, offlineTab())
			if view := w.View(); !w.IsOpen() || !strings.Contains(view, "· live") {
				t.Errorf("Watch without a server did not open live:\n%s", view)
			}
		})
	}
}

func TestWatch_NoOrPrunedLogDims(t *testing.T) {
	cases := map[string]struct {
		log agentlog.Log
		err error
	}{
		"no agent": {agentlog.Log{}, agentlog.ErrNoAgent},
		"pruned":   {agentlog.Log{Pruned: true}, nil},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			fakeNativeLog(t, c.log, c.err)
			tab := serverTab("claimed", tickettest.FakeServerAPI{})
			var rm Menus
			ready := requestTicketMenu(t, &rm, tab)
			if got := itemByLabel(t, ready.Items, "Watch").Disabled; got != NoAgentLog {
				t.Errorf("Watch disabled = %q, want %q", got, NoAgentLog)
			}
		})
	}
}

// fakeHerdrOnlyLog runs the real locator against an empty state dir, so every
// log comes from the herdr lookup the tab hands it.
func fakeHerdrOnlyLog(t *testing.T) {
	t.Helper()
	stateDir := t.TempDir()
	fakeAgentLog(t, func(ctx context.Context, _ config.Dirs, address string, herdr HerdrLookup) (agentlog.Log, error) {
		a, err := tickets.ParseAddress(address, tickets.AddressContext{})
		if err != nil {
			return agentlog.Log{}, err
		}
		return agentlog.Locator{StateDir: stateDir, Herdr: herdr}.AgentLog(ctx, a)
	})
}

// herdrAPI is a server running one herdr iteration of agent on gx:alpha/01.
func herdrAPI(agent, transcript string) tickettest.FakeServerAPI {
	return tickettest.FakeServerAPI{IterationsRes: []wire.IterationInfo{
		{Address: "gx:alpha/01", Agent: agent, Transcript: transcript, Pane: "p1", Tab: "t1"},
	}}
}

func TestWatch_HerdrClaudeOpensLiveModal(t *testing.T) {
	fakeHerdrOnlyLog(t)
	w := openWatchFromMenu(t, serverTab("claimed", herdrAPI("claude", watchLogFile(t))))
	if view := w.View(); !w.IsOpen() || !strings.Contains(view, "· live") || !strings.Contains(view, "hello from the agent") {
		t.Errorf("Watch on a herdr claude agent did not open live:\n%s", view)
	}
}

func TestWatch_HerdrDimReasons(t *testing.T) {
	t.Run("codex", func(t *testing.T) {
		fakeHerdrOnlyLog(t)
		var rm Menus
		ready := requestTicketMenu(t, &rm, serverTab("claimed", herdrAPI("codex", "")))
		if got := itemByLabel(t, ready.Items, "Watch").Disabled; got != NotForCodex {
			t.Errorf("Watch disabled = %q, want %q", got, NotForCodex)
		}
		if got := itemByLabel(t, ready.Items, "Go to pane").Disabled; got != "" {
			t.Errorf("Go to pane disabled = %q, want enabled", got)
		}
	})
	t.Run("no server", func(t *testing.T) {
		fakeHerdrOnlyLog(t)
		var rm Menus
		ready := requestTicketMenu(t, &rm, offlineTab())
		if got := itemByLabel(t, ready.Items, "Watch").Disabled; got != NoAgentLog {
			t.Errorf("Watch disabled = %q, want %q", got, NoAgentLog)
		}
	})
}

// fakeFocus records the herdr tabs focused.
func fakeFocus(t *testing.T) *[]string {
	t.Helper()
	var focused []string
	prev := FocusIterationTab
	t.Cleanup(func() { FocusIterationTab = prev })
	FocusIterationTab = func(tab string) error { focused = append(focused, tab); return nil }
	return &focused
}

func TestGoToPane_HerdrOnlyAndFocusesPane(t *testing.T) {
	fakeHerdrOnlyLog(t)
	focused := fakeFocus(t)
	var rm Menus
	if hasItem(requestTicketMenu(t, &rm, serverTab("claimed", tickettest.FakeServerAPI{})).Items, "Go to pane") {
		t.Error("Go to pane shown for a ticket with no herdr agent")
	}
	tab := serverTab("claimed", herdrAPI("claude", ""))
	cmd := pickTicketMenuItem(t, tab, requestTicketMenu(t, &rm, tab), "Go to pane")
	if cmd == nil {
		t.Fatal("Go to pane returned no Cmd")
	}
	cmd()
	if !slices.Equal(*focused, []string{"t1"}) {
		t.Errorf("focused = %v, want [t1]", *focused)
	}
}
