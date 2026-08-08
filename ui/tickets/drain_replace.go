package tickets

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/ui/confirm"
	"github.com/elentok/gx/ui/notify"
)

// handleDrainReplaceKey applies the "D" ("drain & replace queue") combo: the
// epic under the cursor must have a live run — draining a queue with nothing
// live to drain is what "r" (handleReplaceQueueKey) already does — and the
// agent is captured now, while the run is still confirmed live, since
// agentFor stops returning ok=true the moment the run actually finishes (see
// epicRun.agent's doc comment in loop_registry.go).
func (m Model) handleDrainReplaceKey() (tea.Model, tea.Cmd) {
	r, ok := m.selectedRow()
	if !ok {
		return m, nil
	}
	epic := m.epics[r.epicIdx]
	if !ralphLoopRegistry.isRunningEpic(epic.Name) {
		return m, notify.Info(fmt.Sprintf("epic %q isn't running", epic.Name))
	}
	if len(m.checked) == 0 {
		return m, notify.Info("check at least one ticket to build an execution plan")
	}
	agent, ok := ralphLoopRegistry.agentFor(epic.Name)
	if !ok {
		return m, notify.Info(fmt.Sprintf("epic %q isn't running", epic.Name))
	}
	m.confirm = m.confirm.Open(confirm.Options{
		Prompt:    fmt.Sprintf("Drain epic %q, then replace the queue with the checked selection?", epic.Name),
		AcceptCmd: cmdConfirmDrainReplace(m.worktreeRoot, epic.Name, agent),
	})
	return m, nil
}

// drainReplaceConfirmedMsg carries the "D" confirmation acceptance:
// worktreeRoot/epicName/agent are captured at confirm-open time (mirroring
// replaceQueueConfirmedMsg) since the drain and eventual replace+launch must
// run against the live Model, not the value m.confirm.Open closed over.
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
