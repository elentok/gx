package app

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	ticketsui "github.com/elentok/gx/ui/tickets"
)

// loopStatusPollInterval mirrors ui/tickets' own implementPollInterval: the
// app shell only routes tea.Msgs to the active page, so the overlay (visible
// on every tab) has to poll tickets.LoopStatus() independently rather than
// wait for a message the launching page's Model might swallow.
const loopStatusPollInterval = 300 * time.Millisecond

type loopStatusTickMsg struct{}

// loopStatusOverlay renders a cross-tab "Implementing {epic} (ticket X/Y)..."
// line per currently-running epic (ticket 05: ralphLoopRegistry now allows
// more than one), mirroring ui/notify.Model's spinner pattern, so a
// ralph-loop launched from the tickets tab stays visible while the user is on
// another tab (worktrees, log, status, stash, PRs).
type loopStatusOverlay struct {
	spinner spinner.Model
	running bool
	epics   []ticketsui.EpicProgress
}

func newLoopStatusOverlay() loopStatusOverlay {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	return loopStatusOverlay{spinner: sp}
}

func (m loopStatusOverlay) Init() tea.Cmd {
	return m.cmdPoll()
}

func (m loopStatusOverlay) Update(msg tea.Msg) (loopStatusOverlay, tea.Cmd) {
	switch v := msg.(type) {
	case loopStatusTickMsg:
		wasRunning := m.running
		m.epics = ticketsui.LoopStatusAll()
		m.running = len(m.epics) > 0
		cmds := []tea.Cmd{m.cmdPoll()}
		if m.running && !wasRunning {
			cmds = append(cmds, m.spinner.Tick)
		}
		return m, tea.Batch(cmds...)
	case spinner.TickMsg:
		if !m.running {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(v)
		return m, cmd
	}
	return m, nil
}

func (m loopStatusOverlay) View() string {
	if !m.running {
		return ""
	}
	lines := make([]string, len(m.epics))
	for i, epic := range m.epics {
		lines[i] = fmt.Sprintf("%s Implementing %s (ticket %d/%d)...", m.spinner.View(), epic.Name, epic.Done, epic.Total)
	}
	return strings.Join(lines, "\n")
}

func (m loopStatusOverlay) cmdPoll() tea.Cmd {
	return tea.Tick(loopStatusPollInterval, func(time.Time) tea.Msg {
		return loopStatusTickMsg{}
	})
}
