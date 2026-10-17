package tickets

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/agentlog"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/notify"
)

// actionWatch looks at a live agent, per runner: a native agent's log in the
// Watch modal, a herdr agent's pane by jumping to it. Handled by
// handleActionsMenuKey/handleQueueActionsMenuKey.
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

// watchLoadedMsg carries the rendered log a Watch action read.
type watchLoadedMsg struct {
	title string
	lines []string
	// step, when set, loads the content delta away (`[` -1, `]` +1), or
	// returns nil past an end.
	step func(delta int) tea.Cmd
	// refresh, when set, rereads the content for `r`: a snapshot, not a live
	// tail.
	refresh tea.Cmd
}

// watchModal is the open Watch agent modal: a scrollable view of the agent's
// rendered log, opened at the newest line.
type watchModal struct {
	open    bool
	title   string
	step    func(delta int) tea.Cmd
	refresh tea.Cmd
	vp      viewport.Model
}

func (w watchModal) Open(msg watchLoadedMsg, width, height int) watchModal {
	vpH := max(height-10, 3)
	vp := viewport.New(viewport.WithWidth(watchContentWidth(width)), viewport.WithHeight(vpH))
	vp.SetContent(strings.Join(msg.lines, "\n"))
	vp.GotoBottom()
	return watchModal{open: true, title: msg.title, step: msg.step, refresh: msg.refresh, vp: vp}
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
	case "r":
		return w, w.refresh
	}
	var cmd tea.Cmd
	w.vp, cmd = w.vp.Update(msg)
	return w, cmd
}

func (w watchModal) View() string {
	hint := ui.HintDismiss()
	if w.step != nil {
		hint = ui.JoinStatus(hint, ui.RenderInlineBindings(key.NewBinding(key.WithHelp("[/]", "session"))))
	}
	if w.refresh != nil {
		hint = ui.JoinStatus(hint, ui.RenderInlineBindings(key.NewBinding(key.WithHelp("r", "refresh"))))
	}
	return components.RenderOutputModal(w.title, w.vp.View(), hint, ui.ColorBorder, ui.ColorGreen, ui.ColorGray, w.vp.Width())
}

// holderScreen is a package-level seam so tests can fake a pty holder's
// visible screen rows.
var holderScreen = func(ptyDir string) ([]string, error) {
	h, err := nativerunner.DialHolder(ptyDir)
	if err != nil {
		return nil, err
	}
	rows, _, err := h.Screen(0)
	return rows, err
}

// cmdWatchAgent shows address's native agent: a pty agent's holder screen,
// else its log rendered with the same renderer as `gx server agents watch`.
func cmdWatchAgent(address string) tea.Cmd {
	var cmd tea.Cmd
	cmd = func() tea.Msg {
		log, err := nativeAgentLog(context.Background(), address)
		if err != nil {
			return notify.Error("watch: " + err.Error())()
		}
		var lines []string
		if log.PtyDir != "" && !log.Pruned {
			lines, err = holderScreen(log.PtyDir)
		} else {
			lines, err = renderedLog(log)
		}
		if err != nil {
			return notify.Error("watch: " + err.Error())()
		}
		return watchLoadedMsg{title: "Agent " + address, lines: lines, refresh: cmd}
	}
	return cmd
}

// renderedLog is the last maxWatchLines rendered lines of log.
func renderedLog(log agentlog.Log) ([]string, error) {
	if log.Pruned {
		return []string{"log pruned; transcript: " + log.Transcript}, nil
	}
	f, err := os.Open(log.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if _, err := f.Seek(log.Offset, io.SeekStart); err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, l := range strings.Split(string(raw), "\n") {
		if l != "" {
			lines = append(lines, agentlog.Render([]byte(l))...)
		}
	}
	if len(lines) == 0 {
		return nil, errors.New("the agent has not logged anything yet")
	}
	return lines[max(len(lines)-maxWatchLines, 0):], nil
}
