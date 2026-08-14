package tickets

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/notify"
)

const (
	drainMenuValueDrainOnly = "drain-only"
	drainMenuValueReplace   = "replace"
)

// handleDrainReplaceKey applies the "D" ("drain") combo: the epic under the
// cursor must have a live run — draining a queue with nothing live to drain
// is what "r" (handleReplaceQueueKey) already does — and the agent is
// captured now, while the run is still confirmed live, since agentFor stops
// returning ok=true the moment the run actually finishes (see epicRun.agent's
// doc comment in loop_registry.go). It opens a menu rather than draining
// outright, since "drain only" and "drain and replace the queue" are both
// reachable from here and the menu's header explains what draining does
// before either is chosen.
func (m Model) handleDrainReplaceKey() (tea.Model, tea.Cmd) {
	r, ok := m.selectedRow()
	if !ok {
		return m, nil
	}
	epic := m.epics[r.epicIdx]
	if !ralphLoopRegistry.isRunningEpic(epic.Name) {
		return m, notify.Info(fmt.Sprintf("epic %q isn't running", epic.Name))
	}
	agent, ok := ralphLoopRegistry.agentFor(epic.Name)
	if !ok {
		return m, notify.Info(fmt.Sprintf("epic %q isn't running", epic.Name))
	}
	m.drainMenuEpic = epic.Name
	m.drainMenuAgent = agent
	m.drainMenu = newDrainMenu(len(m.checked) > 0)
	m.drainMenuOpen = true
	return m, nil
}

// newDrainMenu builds the "D" menu's items: "Drain and replace..." is
// omitted entirely (not merely disabled) when nothing is checked, so the
// menu itself explains why it's missing.
func newDrainMenu(offerReplace bool) components.MenuState {
	items := []components.MenuItem{
		{Label: "Drain only", Value: drainMenuValueDrainOnly},
	}
	if offerReplace {
		items = append(items, components.MenuItem{
			Label: "Drain and replace queue with checked selection",
			Value: drainMenuValueReplace,
		})
	}
	return components.MenuState{Items: items}
}

// handleDrainMenuKey drives the open drain menu: esc/q cancels without
// draining anything, and enter commits whichever item is under the cursor —
// "drain only" just drains, "drain and replace..." reuses the existing
// drain-then-replace flow unchanged (cmdConfirmDrainReplace/
// handleDrainReplaceConfirmed/handleDrainReplacePoll/launchDrainedReplace).
func (m Model) handleDrainMenuKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	next, decided, accepted, handled := components.UpdateMenu(msg, m.drainMenu)
	if !handled {
		return m, nil
	}
	m.drainMenu = next
	if !decided {
		return m, nil
	}
	m.drainMenuOpen = false
	if !accepted {
		return m, nil
	}
	if m.drainMenu.Items[m.drainMenu.Cursor].Value == drainMenuValueReplace {
		return m, cmdConfirmDrainReplace(m.worktreeRoot, m.drainMenuEpic, m.drainMenuAgent)
	}
	return m, cmdDrainOnly(m.drainMenuEpic)
}

// cmdDrainOnly drains epicName's Gate with no follow-up queue replace or
// launch — the "Drain only" menu item.
func cmdDrainOnly(epicName string) tea.Cmd {
	return func() tea.Msg {
		ralphLoopRegistry.drain(epicName)
		return notify.Info(fmt.Sprintf("epic %q draining", epicName))()
	}
}

func (m Model) drainMenuView() string {
	prompt := fmt.Sprintf(
		"Draining %q stops admitting new claims, lets the current run finish naturally instead of killing it.",
		m.drainMenuEpic,
	)
	return components.RenderMenuModal(
		"Drain Epic",
		prompt,
		m.drainMenu,
		"",
		ui.ColorBorder,
		ui.ColorBlue,
		ui.ColorSubtle,
		ui.ColorText,
		56,
	)
}

// drainReplaceConfirmedMsg carries the drain-choice menu's accepted
// selection: worktreeRoot/epicName/agent are captured when the menu is
// opened/accepted (mirroring implementAgentMenu) and threaded through so the
// drain and eventual replace+launch run against the live Model.
type drainReplaceConfirmedMsg struct {
	worktreeRoot string
	epicName     string
	agent        ralphloop.AgentKind
}

// cmdConfirmDrainReplace drains epicName's Gate (no new claims are admitted
// from this point on) and starts the poll loop that waits for its run to
// actually finish before replacing the queue and launching the next one.
func cmdConfirmDrainReplace(worktreeRoot, epicName string, agent ralphloop.AgentKind) tea.Cmd {
	return func() tea.Msg {
		ralphLoopRegistry.drain(epicName)
		return drainReplaceConfirmedMsg{worktreeRoot: worktreeRoot, epicName: epicName, agent: agent}
	}
}

func (m Model) handleDrainReplaceConfirmed(msg drainReplaceConfirmedMsg) (tea.Model, tea.Cmd) {
	return m, cmdPollDrainReplace(msg.worktreeRoot, msg.epicName, msg.agent)
}

// drainReplacePollMsg drives the drain-wait loop started by
// drainReplaceConfirmedMsg: on each tick it re-checks ralphLoopRegistry for
// epicName, mirroring implementPollMsg/cmdPollImplement's own tea.Tick loop.
type drainReplacePollMsg struct {
	worktreeRoot string
	epicName     string
	agent        ralphloop.AgentKind
}

func cmdPollDrainReplace(worktreeRoot, epicName string, agent ralphloop.AgentKind) tea.Cmd {
	return tea.Tick(implementPollInterval, func(time.Time) tea.Msg {
		return drainReplacePollMsg{worktreeRoot: worktreeRoot, epicName: epicName, agent: agent}
	})
}

// handleDrainReplacePoll re-ticks while epicName is still draining, and once
// the registry no longer reports it running, replaces the queue with the
// checked selection and launches the next run with the captured agent — no
// further input from the user between drain-start and new-run-start.
func (m Model) handleDrainReplacePoll(msg drainReplacePollMsg) (tea.Model, tea.Cmd) {
	if ralphLoopRegistry.isRunningEpic(msg.epicName) {
		return m, cmdPollDrainReplace(msg.worktreeRoot, msg.epicName, msg.agent)
	}
	return m.launchDrainedReplace(msg.worktreeRoot, msg.agent)
}

// launchDrainedReplace applies replaceQueuedSelection, then launches every
// checked epic plan (up to the registry's available slots) with agent,
// mirroring QueueModel.startAvailableEpics — see 02a's design notes for why
// this launches directly from Model rather than routing through the Queue
// tab's own launch path. Re-reads m.queueStore.Snapshot() after the replace
// (rather than m.checked/m.checkOrder) since that's the queue's actual
// Items/Order, the same source QueueModel.checkedEpicPlans() reads.
func (m Model) launchDrainedReplace(worktreeRoot string, agent ralphloop.AgentKind) (tea.Model, tea.Cmd) {
	if err := m.replaceQueuedSelection(); err != nil {
		return m, notify.Error("save queue: " + err.Error())
	}
	snapshot := m.queueStore.Snapshot()
	plans := checkedEpicPlansFor(m.epics, snapshot.Checked, snapshot.Order)
	slots := min(ralphLoopRegistry.availableSlots(), len(plans))
	cmds := make([]tea.Cmd, 0, slots+1)
	for _, plan := range plans[:slots] {
		runTicketIDs := plan.ticketIDs
		if plan.dynamic {
			runTicketIDs = nil
		}
		cmds = append(cmds, cmdStartImplement(
			worktreeRoot, plan.epic.Name, agent, plan.done, len(plan.ticketIDs),
			m.settings.MaxConcurrentTicketsPerEpic(), runTicketIDs, m.settings.Notifications,
			m.settings.ImplementSkill(), m.settings.ResolvedAgents(),
		))
	}
	cmds = append(cmds, cmdOpenQueueTab(worktreeRoot))
	return m, tea.Batch(cmds...)
}
