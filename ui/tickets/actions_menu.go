package tickets

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/notify"
)

// actionsMenuModel is an embeddable "Suggested Actions" menu, mirroring
// ui/confirm's self-contained Model/Open/Update/View shape (m.confirm.View)
// so both Model and QueueModel can hold one m.actionsMenu field instead of
// duplicating open-bool/state-field pairs and free helper functions.
type actionsMenuModel struct {
	IsOpen bool

	state    components.MenuState
	path     string
	epicName string
	ticketID string
	title    string
}

// Open opens the menu for ticketID (part of epicName) at path, titled for
// its prompt line. epicName/ticketID are only consumed by actions that need
// more than a filesystem path — e.g. actionInvestigate's herdr launch.
func (m actionsMenuModel) Open(path, epicName, ticketID, title string, items []components.MenuItem) actionsMenuModel {
	m.IsOpen = true
	m.state = components.MenuState{Items: items, Cursor: 0}
	m.path = path
	m.epicName = epicName
	m.ticketID = ticketID
	m.title = title
	return m
}

// actionsMenuResult reports the outcome of a key event handled while the
// menu was open.
type actionsMenuResult struct {
	Done     bool // the menu was cancelled or accepted this event
	Accepted bool
	Path     string
	EpicName string
	TicketID string
	Action   string
}

// Update drives the open menu's navigation/cancel/accept handling, mirroring
// confirm.Model.Update. handled reports whether msg was consumed at all.
func (m actionsMenuModel) Update(msg tea.KeyPressMsg) (actionsMenuModel, actionsMenuResult, bool) {
	next, decided, accepted, handled := components.UpdateMenu(msg, m.state)
	if !handled {
		return m, actionsMenuResult{}, false
	}
	m.state = next
	if !decided {
		return m, actionsMenuResult{}, true
	}
	m.IsOpen = false
	if !accepted {
		return m, actionsMenuResult{Done: true}, true
	}
	action := m.state.Items[m.state.Cursor].Value
	return m, actionsMenuResult{Done: true, Accepted: true, Path: m.path, EpicName: m.epicName, TicketID: m.ticketID, Action: action}, true
}

// View renders the menu modal.
func (m actionsMenuModel) View() string {
	prompt := "Suggested actions:"
	if m.title != "" {
		prompt = m.title
	}
	return components.RenderMenuModal(
		"Suggested Actions",
		prompt,
		m.state,
		"",
		ui.ColorBorder,
		ui.ColorBlue,
		ui.ColorSubtle,
		ui.ColorText,
		48,
	)
}

// cmdApplySuggestedAction performs the write in a Cmd (not inline in the
// caller) so a filesystem error surfaces as a toast rather than a panic on
// the Update goroutine, mirroring cmdApplyTicketStatus.
func cmdApplySuggestedAction(path, action string, onSuccess func() tea.Msg) tea.Cmd {
	return func() tea.Msg {
		if err := applySuggestedAction(path, action, time.Now()); err != nil {
			return notify.Error(err.Error())()
		}
		return onSuccess()
	}
}

// handleSuggestedActionsKey applies the "m" keymap: opens the suggested-
// actions menu for the selected ticket row, or toasts when its status has
// none (see suggestedActionItems).
func (m Model) handleSuggestedActionsKey() (tea.Model, tea.Cmd) {
	r, ok := m.selectedRow()
	if !ok || r.isEpic() {
		return m, notify.Info("select a ticket to see its suggested actions")
	}
	epic := m.epicAt(r)
	ticket := epic.Tickets[r.ticketIdx]
	status := epic.RenderedStatus(ticket)
	epicName := epic.Name
	var paneLive bool
	if m.serverMode() {
		// A row's Path is its address and its epic is "project:epic"; the
		// herdr launches want the bare epic.
		epicName, _, _ = tickets.SplitTrailerValue(ticket.Path)
		_, paneLive = m.serverIteration(ticket.Path)
		paneLive = paneLive && status == tickets.StatusNeedsAnswer
	} else {
		paneLive = ticketPaneLive(status, epic.Name, ticket.DisplayNumber())
	}
	items := suggestedActionItems(status, ticket, paneLive)
	if len(items) == 0 {
		return m, notify.Info("no suggested actions for this ticket")
	}
	prompt := fmt.Sprintf("Suggested actions for %q:", ticket.Title)
	m.actionsMenu = m.actionsMenu.Open(ticket.Path, epicName, ticket.DisplayNumber(), prompt, items)
	return m, nil
}

// handleActionsMenuKey drives the open suggested-actions menu.
func (m Model) handleActionsMenuKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	next, result, handled := m.actionsMenu.Update(msg)
	if !handled {
		return m, nil
	}
	m.actionsMenu = next
	if !result.Done || !result.Accepted {
		return m, nil
	}
	if result.Action == actionInvestigate {
		return m, cmdLaunchInvestigate(m.worktreeRoot, result.EpicName, result.TicketID)
	}
	if m.serverMode() {
		if cmd, ok := m.serverAnswerActionCmd(result); ok {
			return m, cmd
		}
		return m, nil
	}
	if cmd, ok := answerActionCmd(m.worktreeRoot, m.settings, result); ok {
		return m, cmd
	}
	return m, cmdApplySuggestedAction(result.Path, result.Action, func() tea.Msg { return statusChangedMsg{} })
}
