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

// actionsMenuReadyMsg carries a suggested-actions menu whose agent liveness
// was resolved off the Update goroutine.
type actionsMenuReadyMsg struct {
	path, epicName, ticketID, prompt string
	items                            []components.MenuItem
}

// handleSuggestedActionsKey applies the "m" keymap: opens the suggested-
// actions menu for the selected ticket row, whatever its status. Liveness
// shells out to ps, so the menu opens once a Cmd has resolved it.
func (m Model) handleSuggestedActionsKey() (tea.Model, tea.Cmd) {
	r, ok := m.selectedRow()
	// Epic actions are server-only: only server-mode epic names carry the
	// "project:" prefix startSessionCmd and the merge need.
	if ok && r.isEpic() && m.serverAPI != nil && !r.archived {
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
	ticket := epic.Tickets[r.ticketIdx]
	status := epic.RenderedStatus(ticket)
	// A row's Path is its address and its epic is "project:epic"; the herdr
	// launches want the bare epic.
	epicName, _, _ := tickets.SplitTrailerValue(ticket.Path)
	prompt := fmt.Sprintf("Suggested actions for %q:", ticket.Title)
	herdrUnavailable := m.settings.HerdrUnavailable
	startSession := epic.IsMap && status == tickets.StatusOpen
	m.actionsMenuPending = ticket.Path
	return m, func() tea.Msg {
		it, live := m.serverIteration(ticket.Path)
		paneLive := live && it.Tab != ""
		items := suggestedActionItems(status, ticket, paneLive, nativeAgentLive(ticket.Path), herdrUnavailable)
		if startSession {
			items = append([]components.MenuItem{startSessionMenuItem}, items...)
		}
		return actionsMenuReadyMsg{path: ticket.Path, epicName: epicName, ticketID: ticket.DisplayNumber(), prompt: prompt, items: items}
	}
}

// handleActionsMenuReady opens the menu only for the request still pending:
// the user may have moved on, or opened another overlay, while ps ran.
func (m Model) handleActionsMenuReady(msg actionsMenuReadyMsg) Model {
	pending := m.actionsMenuPending
	m.actionsMenuPending = ""
	if pending != msg.path || m.selectedTicketPath() != msg.path || m.ModalOpen() {
		return m
	}
	m.actionsMenu = m.actionsMenu.Open(msg.path, msg.epicName, msg.ticketID, msg.prompt, msg.items)
	return m
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
	if result.Action == actionMerge {
		return m, cmdEpicMerge(m.serverAPI, result.EpicName)
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
	if cmd, ok := m.serverAnswerActionCmd(result); ok {
		return m, cmd
	}
	return m, nil
}
