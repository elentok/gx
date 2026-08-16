package tickets

import (
	"fmt"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/subscription"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/confirm"
	"github.com/elentok/gx/ui/notify"
)

// codexOnPath reports whether the codex CLI is on PATH: a fresh, uncached
// check run on every run-start modal open (see availableAgents), deliberately
// lighter than the full auth/herdr preflight, which still gates the actual
// launch unchanged. Swapped in tests.
var codexOnPath = func() bool {
	_, err := exec.LookPath("codex")
	return err == nil
}

// availableAgents lists the agent kinds actually usable on this machine right
// now: Claude has no availability check of its own and is always included;
// Codex is included only when codexOnPath reports true.
func availableAgents() []ralphloop.AgentKind {
	agents := []ralphloop.AgentKind{ralphloop.AgentClaude}
	if codexOnPath() {
		agents = append(agents, ralphloop.AgentCodex)
	}
	return agents
}

// runStartBannerLines returns the run-start modal's banner lines: the
// subscription extra-usage safety-check line (ticket 03), if any, followed by
// the configured budget's soft/hard limits and notification thresholds
// (ticket 01) — omitted entirely when both limits are disabled.
func runStartBannerLines(budget config.BudgetConfig, sub config.SubscriptionConfig) []string {
	var lines []string
	if line := subscription.BuildLine(subscription.Check(), sub.SuppressExtraUsageWarning); line != nil {
		lines = append(lines, styleSubscriptionLine(line))
	}
	lines = append(lines, budgetBannerLines(budget)...)
	return lines
}

// styleSubscriptionLine renders a subscription safety-check line per its
// severity: SeverityWarning is unmissable (bold, warning color) since gx has
// no control over the account setting itself; SeverityInfo is a quieter,
// muted confirmation/remind-only line.
func styleSubscriptionLine(line *subscription.Line) string {
	switch line.Severity {
	case subscription.SeverityWarning:
		return ui.StyleWarning.Bold(true).Render(line.Text)
	default:
		return ui.StyleMuted.Render(line.Text)
	}
}

// budgetBannerLines renders budget's soft/hard limits and notification
// thresholds, framed as estimated API-equivalent cost, or nil when both
// limits are disabled (0).
func budgetBannerLines(budget config.BudgetConfig) []string {
	if budget.SoftLimit == 0 && budget.HardLimit == 0 {
		return nil
	}
	var lines []string
	if budget.SoftLimit > 0 {
		lines = append(lines, fmt.Sprintf("Soft budget limit: %s estimated API-equivalent cost", formatCost(budget.SoftLimit)))
	}
	if budget.HardLimit > 0 {
		lines = append(lines, fmt.Sprintf("Hard budget limit: %s estimated API-equivalent cost", formatCost(budget.HardLimit)))
	}
	if len(budget.NotificationThresholds) > 0 {
		thresholds := make([]string, len(budget.NotificationThresholds))
		for i, t := range budget.NotificationThresholds {
			thresholds[i] = formatCost(t)
		}
		lines = append(lines, "Notification thresholds: "+strings.Join(thresholds, ", ")+" estimated API-equivalent cost")
	}
	return lines
}

// runStartBannerText joins runStartBannerLines into the multi-line text
// prepended to the run-start modal's prompt, or "" when there's nothing to
// show.
func runStartBannerText(budget config.BudgetConfig, sub config.SubscriptionConfig) string {
	lines := runStartBannerLines(budget, sub)
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}

// newRunStartAgentMenu is the Queue tab's run-start pick-list, offered only
// when openRunStartModal finds more than one available agent — a trailing
// Cancel item alongside Claude/Codex, since this modal (unlike the Tickets
// tab's dead agent-picker it replaces) has no separate confirm step for
// picking to fall back on.
func newRunStartAgentMenu() components.MenuState {
	return components.MenuState{
		Items: []components.MenuItem{
			{Label: "l  Claude", Value: string(ralphloop.AgentClaude)},
			{Label: "o  Codex", Value: string(ralphloop.AgentCodex)},
			{Label: "Cancel", Value: "cancel"},
		},
		Cursor: 0,
	}
}

// openRunStartModal implements ticket 09a's unified run-start modal: a
// banner (the subscription safety-check line plus configured budget limits)
// above an action area scoped to whichever agents are actually usable on
// this machine right now (see availableAgents — Codex's availability is
// rechecked fresh, uncached, on every open). Exactly one available agent
// collapses the action area to a plain Yes/No confirmation; more than one
// opens the pick-an-agent list (plus Cancel), where picking confirms
// directly; zero is a defensive, currently-unreachable case (Claude has no
// availability check) that never opens a modal at all.
func (m QueueModel) openRunStartModal() (tea.Model, tea.Cmd) {
	agents := availableAgents()
	banner := runStartBannerText(m.settings.Budget, m.settings.Subscription)
	switch len(agents) {
	case 0:
		return m, notify.Info("no agent is available to run this epic")
	case 1:
		prompt := fmt.Sprintf("Start the checked selection with %s?", agentDisplayName(agents[0]))
		if banner != "" {
			prompt = banner + "\n\n" + prompt
		}
		m.confirm = m.confirm.Open(confirm.Options{
			Prompt:    prompt,
			AcceptCmd: cmdConfirmRunStart(agents[0]),
		})
		return m, nil
	default:
		m.implementAgentMenu = newRunStartAgentMenu()
		m.implementAgentMenuOpen = true
		return m, nil
	}
}

// runStartConfirmedMsg carries the run-start modal's Yes/No confirmation
// acceptance: agent is captured when the modal opened (mirroring
// queueClearConfirmedMsg's same capture-at-open-time approach).
type runStartConfirmedMsg struct {
	agent ralphloop.AgentKind
}

func cmdConfirmRunStart(agent ralphloop.AgentKind) tea.Cmd {
	return func() tea.Msg {
		return runStartConfirmedMsg{agent: agent}
	}
}

// handleRunStartConfirmed applies runStartConfirmedMsg by launching the
// checked selection with the confirmed agent, same as picking an agent
// directly from the pick-list branch.
func (m QueueModel) handleRunStartConfirmed(msg runStartConfirmedMsg) (tea.Model, tea.Cmd) {
	return m.startCheckedEpic(msg.agent)
}
