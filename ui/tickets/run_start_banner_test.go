package tickets

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/subscription"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
)

func TestRunStartBannerLines_BudgetLinesOmittedWhenBothLimitsDisabled(t *testing.T) {
	t.Parallel()
	lines := budgetBannerLines(config.BudgetConfig{})
	if lines != nil {
		t.Fatalf("expected no budget lines when both limits disabled, got %v", lines)
	}
}

func TestRunStartBannerLines_RendersSoftHardAndThresholds(t *testing.T) {
	t.Parallel()
	lines := budgetBannerLines(config.BudgetConfig{
		SoftLimit:              5,
		HardLimit:              10,
		NotificationThresholds: []float64{2.5, 7.5},
	})
	joined := strings.Join(lines, "\n")
	for _, want := range []string{
		"Soft budget limit: $5.00 estimated API-equivalent cost",
		"Hard budget limit: $10.00 estimated API-equivalent cost",
		"Notification thresholds: $2.50, $7.50 estimated API-equivalent cost",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected banner lines to contain %q, got:\n%s", want, joined)
		}
	}
}

func TestStyleSubscriptionLine_SeverityRendersDistinctly(t *testing.T) {
	t.Parallel()
	warning := styleSubscriptionLine(&subscription.Line{Text: "unmissable", Severity: subscription.SeverityWarning})
	info := styleSubscriptionLine(&subscription.Line{Text: "quiet", Severity: subscription.SeverityInfo})

	if warning != ui.StyleWarning.Bold(true).Render("unmissable") {
		t.Fatalf("expected SeverityWarning line styled with ui.StyleWarning bold, got %q", warning)
	}
	if info != ui.StyleMuted.Render("quiet") {
		t.Fatalf("expected SeverityInfo line styled with ui.StyleMuted, got %q", info)
	}
	if warning == info {
		t.Fatalf("expected SeverityWarning and SeverityInfo to render with different styling, both rendered %q", warning)
	}
}

func TestRunStartBannerText_AlwaysShowsSubscriptionLineWhenPresent(t *testing.T) {
	text := runStartBannerText(config.BudgetConfig{}, config.SubscriptionConfig{})
	if text == "" {
		t.Skip("no subscription safety-check line available in this environment")
	}
}

func TestOpenRunStartModal_SingleAvailableAgentAsksYesNo(t *testing.T) {
	previous := codexOnPath
	codexOnPath = func() bool { return false }
	t.Cleanup(func() { codexOnPath = previous })

	root := testutil.TempRepo(t)
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{ticketPath(root, "alpha", "01-first.md"): true}

	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(QueueModel)

	if !m.confirm.IsOpen {
		t.Fatalf("expected a Yes/No confirmation when exactly one agent is available")
	}
	if !strings.Contains(m.confirm.View(80), "Claude") {
		t.Fatalf("expected confirm prompt to mention Claude:\n%s", m.confirm.View(80))
	}
	if m.implementAgentMenuOpen {
		t.Fatalf("expected the pick-list not to open when only one agent is available")
	}
}

func TestOpenRunStartModal_EscapeAbortsSingleAgentConfirmWithoutStartingRun(t *testing.T) {
	previous := codexOnPath
	codexOnPath = func() bool { return false }
	t.Cleanup(func() { codexOnPath = previous })

	root := testutil.TempRepo(t)
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{ticketPath(root, "alpha", "01-first.md"): true}

	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(QueueModel)

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(QueueModel)
	if m.confirm.IsOpen {
		t.Fatalf("expected escape to close the confirmation")
	}
	if cmd != nil {
		if msg := cmd(); msg != nil {
			if _, ok := msg.(runStartConfirmedMsg); ok {
				t.Fatalf("escape must not start the run")
			}
		}
	}
	if m.runningEpics["alpha"] {
		t.Fatalf("escape must not start the run")
	}
}

func TestOpenRunStartModal_MultipleAvailableAgentsShowsPickListWithCancel(t *testing.T) {
	root := testutil.TempRepo(t)
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{ticketPath(root, "alpha", "01-first.md"): true}

	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(QueueModel)

	if !m.implementAgentMenuOpen {
		t.Fatalf("expected the pick-list to open when more than one agent is available")
	}
	found := false
	for _, item := range m.implementAgentMenu.Items {
		if item.Value == "cancel" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a Cancel item in the pick-list, got %+v", m.implementAgentMenu.Items)
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(QueueModel)
	if m.implementAgentMenuOpen {
		t.Fatalf("expected escape to close the pick-list")
	}
	if cmd != nil {
		t.Fatalf("escape must not start the run")
	}
}

func TestAvailableAgents_IncludesCodexOnlyWhenOnPath(t *testing.T) {
	previous := codexOnPath
	t.Cleanup(func() { codexOnPath = previous })

	codexOnPath = func() bool { return false }
	if agents := availableAgents(); len(agents) != 1 || agents[0] != ralphloop.AgentClaude {
		t.Fatalf("expected only Claude when codex is not on PATH, got %v", agents)
	}

	codexOnPath = func() bool { return true }
	agents := availableAgents()
	if len(agents) != 2 {
		t.Fatalf("expected Claude and Codex when codex is on PATH, got %v", agents)
	}
}
