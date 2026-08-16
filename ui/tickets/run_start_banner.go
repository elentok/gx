package tickets

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/subscription"
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
		lines = append(lines, line.Text)
	}
	lines = append(lines, budgetBannerLines(budget)...)
	return lines
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
		lines = append(lines, "Notification thresholds: "+strings.Join(thresholds, ", "))
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
