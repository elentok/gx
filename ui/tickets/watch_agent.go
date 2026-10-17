package tickets

import (
	"context"
	"strings"
	"sync/atomic"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/agentlog"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/notify"
)

// actionWatch looks at a live agent, per runner: a native agent's log in the
// Watch modal, a herdr agent's pane by jumping to it. Handled by
// dispatchMenuAction.
const actionWatch = "watch"

// maxWatchLines bounds the modal to the log's tail so a long run can't stall
// the Update goroutine.
const maxWatchLines = 2000

// nativeAgentLog is a package-level seam so tests can fake the agent log. It
// locates only native agents (no herdr lookup): a ticket without one is
// watched through its herdr pane instead.
var nativeAgentLog = func(ctx context.Context, address string) (agentlog.Log, error) {
	a, err := tickets.ParseAddress(address, tickets.AddressContext{})
	if err != nil {
		return agentlog.Log{}, err
	}
	stateDir, err := config.StateDir()
	if err != nil {
		return agentlog.Log{}, err
	}
	return agentlog.Locator{StateDir: stateDir}.AgentLog(ctx, a)
}

// nativeAgentLive reports whether address's agent runs on a native runner and
// its process is still alive.
func nativeAgentLive(address string) bool {
	log, err := nativeAgentLog(context.Background(), address)
	return err == nil && !log.Pruned && log.Ended != nil && !log.Ended()
}

// watchTickInterval matches `gx server agents watch`'s follow interval.
const watchTickInterval = 200 * time.Millisecond

// watchGen tags each live modal so a tick armed by an earlier one is dropped
// (and its tail closed) instead of feeding the current one.
var watchGen atomic.Int64

// watchLoadedMsg carries the rendered log a Watch or Transcript action read.
type watchLoadedMsg struct {
	title string
	lines []string
	// step, when set, loads the content delta away (`[` -1, `]` +1), or
	// returns nil past an end.
	step func(delta int) tea.Cmd
	// live marks a Watch: the header shows live or "agent ended".
	live  bool
	ended bool
	// tail, when set, is stepped once per tick for new lines.
	tail *agentlog.Tail
}

// watchStepMsg carries one tick's step of a live modal's tail.
type watchStepMsg struct {
	gen   int64
	tail  *agentlog.Tail
	lines []string
	ended bool
	err   error
}

// watchModal is the open Watch agent modal: a scrollable view of the agent's
// rendered log, opened at the newest line. A live one follows new lines while
// at the bottom.
type watchModal struct {
	open     bool
	title    string
	step     func(delta int) tea.Cmd
	live     bool
	ended    bool
	tail     *agentlog.Tail
	gen      int64
	lines    []string
	newLines bool
	vp       viewport.Model
}

// Open replaces the modal with msg's content; tick starts a live one's
// follow.
func (w watchModal) Open(msg watchLoadedMsg, width, height int) watchModal {
	vpH := max(height-10, 3)
	vp := viewport.New(viewport.WithWidth(watchContentWidth(width)), viewport.WithHeight(vpH))
	vp.SetContent(strings.Join(msg.lines, "\n"))
	vp.GotoBottom()
	w = watchModal{open: true, title: msg.title, step: msg.step, live: msg.live, ended: msg.ended, tail: msg.tail, lines: msg.lines, vp: vp}
	if w.tail != nil {
		w.gen = watchGen.Add(1)
	}
	return w
}

// tick arms the next step of a live modal's tail, or nil when nothing follows.
// The step runs in the Cmd since Ended may probe the agent's process.
func (w watchModal) tick() tea.Cmd {
	if w.tail == nil {
		return nil
	}
	gen, tail := w.gen, w.tail
	return tea.Tick(watchTickInterval, func(time.Time) tea.Msg {
		lines, ended, err := tail.Step()
		return watchStepMsg{gen: gen, tail: tail, lines: lines, ended: ended, err: err}
	})
}

// Step applies a tick's lines and re-arms the tick until the log ends. The
// tail is closed here, never on close, so it isn't closed under a running step.
func (w watchModal) Step(msg watchStepMsg) (watchModal, tea.Cmd) {
	if !w.open || msg.gen != w.gen || msg.err != nil || msg.ended {
		msg.tail.Close()
	}
	if !w.open || msg.gen != w.gen {
		return w, nil
	}
	w = w.appendLines(msg.lines)
	if msg.err != nil {
		w.tail = nil
		return w, notify.Error("watch: " + msg.err.Error())
	}
	if msg.ended {
		w.ended, w.tail = true, nil
		return w, nil
	}
	return w, w.tick()
}

// appendLines adds lines, keeping the newest maxWatchLines: at the bottom it
// follows them, scrolled up it stays on the same lines and marks new ones.
func (w watchModal) appendLines(lines []string) watchModal {
	if len(lines) == 0 {
		return w
	}
	atBottom, off := w.vp.AtBottom(), w.vp.YOffset()
	w.lines = append(w.lines, lines...)
	dropped := max(len(w.lines)-maxWatchLines, 0)
	w.lines = w.lines[dropped:]
	w.vp.SetContent(strings.Join(w.lines, "\n"))
	if atBottom {
		w.vp.GotoBottom()
	} else {
		w.vp.SetYOffset(max(off-dropped, 0))
		w.newLines = true
	}
	return w
}

// watchContentWidth is the modal's content width on a width-wide screen.
func watchContentWidth(width int) int {
	return min(max(width*2/3, 40), 120) - 2
}

func (w watchModal) Update(msg tea.KeyPressMsg) (watchModal, tea.Cmd) {
	switch msg.String() {
	case "esc", "enter", "q":
		w.open = false
		return w, nil
	case "[", "]":
		if w.step == nil {
			return w, nil
		}
		if msg.String() == "[" {
			return w, w.step(-1)
		}
		return w, w.step(1)
	case "g", "home":
		w.vp.GotoTop()
	case "G", "end":
		w.vp.GotoBottom()
	default:
		var cmd tea.Cmd
		w.vp, cmd = w.vp.Update(msg)
		w.newLines = w.newLines && !w.vp.AtBottom()
		return w, cmd
	}
	w.newLines = w.newLines && !w.vp.AtBottom()
	return w, nil
}

func (w watchModal) View() string {
	title := w.title
	switch {
	case w.live && w.ended:
		title += " · agent ended"
	case w.live:
		title += " · live"
	}
	hint := ui.HintDismiss()
	if w.step != nil {
		hint = ui.JoinStatus(hint, ui.RenderInlineBindings(key.NewBinding(key.WithHelp("[/]", "session"))))
	}
	if w.newLines {
		hint = ui.JoinStatus(hint, ui.RenderInlineBindings(key.NewBinding(key.WithHelp("G", "↓ new lines"))))
	}
	return components.RenderOutputModal(title, w.vp.View(), hint, ui.ColorBorder, ui.ColorGreen, ui.ColorGray, w.vp.Width())
}

// cmdWatchAgent opens address's native agent log live, rendered with the same
// renderer and tail stepper as `gx server agents watch`. A pty agent's log is
// its conversation transcript, not its terminal screen. The tail stays on
// this run's log, so a later resumed run is not followed.
func cmdWatchAgent(address string) tea.Cmd {
	return func() tea.Msg {
		log, err := nativeAgentLog(context.Background(), address)
		if err != nil {
			return notify.Error("watch: " + err.Error())()
		}
		msg := watchLoadedMsg{title: "Agent " + address, live: true}
		if log.Pruned {
			msg.lines, msg.ended = []string{"log pruned; transcript: " + log.Transcript}, true
			return msg
		}
		tail, err := agentlog.OpenTail(log, nil)
		if err != nil {
			return notify.Error("watch: " + err.Error())()
		}
		lines, ended, err := tail.Step()
		if err != nil {
			tail.Close()
			return notify.Error("watch: " + err.Error())()
		}
		msg.lines, msg.ended = lines[max(len(lines)-maxWatchLines, 0):], ended
		if ended {
			tail.Close()
		} else {
			msg.tail = tail
		}
		return msg
	}
}
