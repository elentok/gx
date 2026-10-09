package tickets

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/agentlog"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/notify"
)

// actionWatchAgent opens the Watch agent modal on a native runner's log. It
// replaces "Answer in pane": a native agent has no pane to focus. Dispatched
// like actionAnswerInPane (see serverAnswerActionCmd).
const actionWatchAgent = "watch-agent"

// maxWatchLines bounds the modal to the log's tail so a long run can't stall
// the Update goroutine.
const maxWatchLines = 2000

// nativeAgentLog is a package-level seam so tests can fake the agent log. It
// locates only native agents (no herdr lookup): a ticket without one is not
// watchable here and keeps its herdr "Answer in pane".
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

// hasNativeAgent reports whether address runs on a native runner.
func hasNativeAgent(address string) bool {
	_, err := nativeAgentLog(context.Background(), address)
	return err == nil
}

// watchLoadedMsg carries the rendered log a Watch agent action read.
type watchLoadedMsg struct {
	title string
	lines []string
}

// watchModal is the open Watch agent modal: a scrollable view of the agent's
// rendered log, opened at the newest line.
type watchModal struct {
	open  bool
	title string
	vp    viewport.Model
}

func (w watchModal) Open(title string, lines []string, width, height int) watchModal {
	vpW := min(max(width*2/3, 40), 120)
	vpH := max(height-10, 3)
	vp := viewport.New(viewport.WithWidth(vpW-2), viewport.WithHeight(vpH))
	vp.SetContent(strings.Join(lines, "\n"))
	vp.GotoBottom()
	return watchModal{open: true, title: title, vp: vp}
}

func (w watchModal) Update(msg tea.KeyPressMsg) (watchModal, tea.Cmd) {
	switch msg.String() {
	case "esc", "enter", "q":
		w.open = false
		return w, nil
	}
	var cmd tea.Cmd
	w.vp, cmd = w.vp.Update(msg)
	return w, cmd
}

func (w watchModal) View() string {
	return components.RenderOutputModal(w.title, w.vp.View(), ui.HintDismiss(), ui.ColorBorder, ui.ColorGreen, ui.ColorGray, w.vp.Width())
}

// cmdWatchAgent reads address's native log and renders it with the same
// renderer as `gx server agents watch`.
func cmdWatchAgent(address string) tea.Cmd {
	return func() tea.Msg {
		log, err := nativeAgentLog(context.Background(), address)
		if err != nil {
			return notify.Error("watch: " + err.Error())()
		}
		lines, err := renderedLog(log)
		if err != nil {
			return notify.Error("watch: " + err.Error())()
		}
		return watchLoadedMsg{title: "Agent " + address, lines: lines}
	}
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
