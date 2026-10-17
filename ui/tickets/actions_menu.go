package tickets

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

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
	item, _ := m.state.Selected()
	action := item.Value
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

// handleSuggestedActionsKey applies the "m" keymap: opens the Context menu
// for the selected ticket row whatever its status, or the suggested-actions
// menu for a server-mode epic row.
func (m Model) handleSuggestedActionsKey() (tea.Model, tea.Cmd) {
	r, ok := m.selectedRow()
	// Epic actions are server-only: only server-mode epic names carry the
	// "project:" prefix startSessionCmd and the merge need.
	if ok && r.isEpic() && m.serverAPI != nil && !r.archived {
		if reason := m.link.disabledReason(); reason != "" {
			return m, notify.Info("disabled: " + reason)
		}
		epic := m.epicAt(r)
		var items []components.MenuItem
		if epic.IsMap {
			items = append(items, startSessionMenuItem)
		}
		if epic.AllDone() && epic.MergedAt.IsZero() {
			items = append(items, mergeMenuItem)
		}
		if len(items) > 0 {
			prompt := fmt.Sprintf("Suggested actions for %q:", epic.Name)
			// Path carries the address startSessionCmd reads, as on a ticket row.
			m.actionsMenu = m.actionsMenu.Open(epic.Name, epic.Name, "", prompt, items)
			return m, nil
		}
	}
	if !ok || r.isEpic() {
		return m, notify.Info("select a ticket to see its suggested actions")
	}
	epic := m.epicAt(r)
	return m.openTicketMenu(epic, epic.Tickets[r.ticketIdx], r.archived)
}

// selectedTicketPath is the selected ticket row's path, or "" off a ticket.
func (m Model) selectedTicketPath() string {
	r, ok := m.selectedRow()
	if !ok || r.isEpic() {
		return ""
	}
	return m.epicAt(r).Tickets[r.ticketIdx].Path
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
	return m.runMenuAction(result)
}

// runMenuAction runs an accepted action from either menu.
func (m Model) runMenuAction(result actionsMenuResult) (tea.Model, tea.Cmd) {
	if cmd, ok := dispatchMenuAction(m.serverAPI, m.worktreeRoot, m.settings, result); ok {
		return m, cmd
	}
	if result.Action == actionStartSession {
		return m, startSessionCmd(result.Path)
	}
	if result.Action == actionInvestigate {
		return m, cmdLaunchInvestigate(m.worktreeRoot, result.EpicName, result.TicketID)
	}
	if result.Action == actionTranscript {
		file, err := ticketFile(m.settings.TicketStore, result.Path)
		if err != nil {
			return m, notify.Error("transcript: " + err.Error())
		}
		return m, cmdTranscript(file, result.Path, m.width)
	}
	if result.Action == actionWatch {
		return m, m.cmdServerWatch(result.Path)
	}
	return m, nil
}
