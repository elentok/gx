package tickets

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui/contextmenu"
)

// ticketTarget is the ticket row a Context menu was opened on. epicName is
// the bare epic the herdr launches want.
type ticketTarget struct{ address, epicName, ticketID string }

// ticketMenuPickMsg carries a menu pick back to Update: the actions need the
// model, which an item's Run cannot reach. status is set for a Status pick.
type ticketMenuPickMsg struct {
	target ticketTarget
	action string
	status schema.Status
}

// ticketMenuInput is what ticketMenuItems needs; liveness is resolved off the
// Update goroutine, the rest is read from the model when "m" is pressed.
type ticketMenuInput struct {
	rendered             tickets.RenderedStatus
	ticket               tickets.Ticket
	isMap                bool
	paneLive, nativeLive bool
	herdrDown            bool
	// serverReason is why orchestration writes are off; empty when usable.
	serverReason string
}

const (
	ticketFinished = "ticket is finished"
	alreadyParked  = "already parked"
	notParked      = "not parked"
	notMuted       = "not muted"
	herdrDownText  = "herdr not running"
)

// ticketMenuItems lists every action for a ticket. An action shown under
// Suggested is not repeated in its own group; one that doesn't apply is dim
// with the reason.
func ticketMenuItems(in ticketMenuInput) []contextmenu.Item[ticketTarget] {
	item := func(label, key string, group contextmenu.Group, disabled string, msg ticketMenuPickMsg) contextmenu.Item[ticketTarget] {
		return contextmenu.Item[ticketTarget]{
			Label: label, Key: key, Group: group, Disabled: disabled,
			Run: func(t ticketTarget) tea.Cmd {
				msg := msg
				msg.target = t
				return func() tea.Msg { return msg }
			},
		}
	}
	pick := func(label, key, action string, group contextmenu.Group, disabled string) contextmenu.Item[ticketTarget] {
		return item(label, key, group, disabled, ticketMenuPickMsg{action: action})
	}
	finished := ""
	if in.rendered.Terminal() {
		finished = ticketFinished
	}
	server := func(label, key, action, why string) contextmenu.Item[ticketTarget] {
		return pick(label, key, action, contextmenu.Server, firstReason(in.serverReason, finished, why))
	}
	parked := isParkedStatus(in.ticket.Status)
	answer, unmute := answerSuggested(in.rendered), unmuteSuggested(in.ticket)

	var items []contextmenu.Item[ticketTarget]
	if in.isMap && in.rendered == tickets.StatusOpen {
		items = append(items, pick(startSessionMenuItem.Label, "", actionStartSession, contextmenu.Suggested, ""))
	}
	if answer {
		items = append(items,
			pick("Answer…", "", actionAnswer, contextmenu.Suggested, ""),
			pick(resumeLabel, "enter", actionResume, contextmenu.Suggested, in.serverReason))
	}
	if investigateSuggested(in.rendered) {
		items = append(items, pick("Investigate", "", actionInvestigate, contextmenu.Suggested, reasonIf(in.herdrDown, herdrDownText)))
	}
	if unmute {
		items = append(items, pick("Unmute & Reopen", "", actionUnmuteReopen, contextmenu.Suggested, in.serverReason))
	}

	current := schema.Status(strings.ToLower(strings.TrimSpace(in.ticket.Status)))
	for _, status := range serverDirectStatuses {
		why := ""
		switch {
		case status == current:
			why = "already " + string(status)
		case status == schema.StatusDone && !in.isMap:
			why = doneIsSetByServer
		}
		items = append(items, item("Set "+string(status), "", contextmenu.Status, why, ticketMenuPickMsg{status: status}))
	}

	items = append(items, server("Park", "", serverVerbPark, reasonIf(parked, alreadyParked)))
	if !answer {
		items = append(items, server(resumeLabel, "enter", actionResume, reasonIf(!parked, notParked)))
	}
	items = append(items,
		server("Cancel", "", serverVerbCancel, ""),
		server("Relaunch", "", serverVerbRelaunch, ""))
	if !unmute {
		items = append(items, server("Unmute & Reopen", "", actionUnmuteReopen, notMuted))
	}

	if in.nativeLive || (in.paneLive && !in.herdrDown) {
		items = append(items, pick("Watch", "", actionWatch, contextmenu.Agent, ""))
	}
	return append(items, pick("Transcript", "", actionTranscript, contextmenu.Agent, ""))
}

func reasonIf(applies bool, why string) string {
	if applies {
		return why
	}
	return ""
}

// firstReason is the highest-priority reason: the server's, then the
// ticket's, then the item's own.
func firstReason(reasons ...string) string {
	for _, r := range reasons {
		if r != "" {
			return r
		}
	}
	return ""
}

// serverReason is why orchestration writes are off right now; empty when the
// server takes them.
func (m Model) serverReason() string {
	if m.serverAPI == nil || m.link == ServerLinkDown {
		return serverNotRunning
	}
	return m.link.disabledReason()
}

// ticketMenuReadyMsg carries a ticket's Context menu whose agent liveness was
// resolved off the Update goroutine.
type ticketMenuReadyMsg struct {
	target ticketTarget
	prompt string
	items  []contextmenu.Item[ticketTarget]
}

// openTicketMenu resolves liveness in a Cmd (it shells out to ps), so the
// menu opens once the result comes back.
func (m Model) openTicketMenu(epic tickets.Epic, ticket tickets.Ticket) (tea.Model, tea.Cmd) {
	// A row's Path is its address and its epic is "project:epic"; the herdr
	// launches want the bare epic.
	epicName, _, _ := tickets.SplitTrailerValue(ticket.Path)
	target := ticketTarget{address: ticket.Path, epicName: epicName, ticketID: ticket.DisplayNumber()}
	in := ticketMenuInput{
		rendered:     epic.RenderedStatus(ticket),
		ticket:       ticket,
		isMap:        epic.IsMap,
		herdrDown:    m.settings.HerdrUnavailable,
		serverReason: m.serverReason(),
	}
	prompt := fmt.Sprintf("Ticket %q", ticket.Title)
	m.ticketMenuPending = ticket.Path
	return m, func() tea.Msg {
		if m.serverAPI != nil {
			it, live := m.serverIteration(ticket.Path)
			in.paneLive = live && it.Tab != ""
		}
		in.nativeLive = nativeAgentLive(ticket.Path)
		return ticketMenuReadyMsg{target: target, prompt: prompt, items: ticketMenuItems(in)}
	}
}

// handleTicketMenuReady opens the menu only for the request still pending:
// the user may have moved on, or opened another overlay, while ps ran.
func (m Model) handleTicketMenuReady(msg ticketMenuReadyMsg) Model {
	pending := m.ticketMenuPending
	m.ticketMenuPending = ""
	address := msg.target.address
	if pending != address || m.selectedTicketPath() != address || m.ModalOpen() {
		return m
	}
	m.ticketMenu = m.ticketMenu.Open(msg.target, msg.prompt, msg.items)
	return m
}

// handleTicketMenuPick runs a pick only while its ticket is still the
// selected one; the status writes work on the selection.
func (m Model) handleTicketMenuPick(msg ticketMenuPickMsg) (tea.Model, tea.Cmd) {
	if msg.target.address != m.selectedTicketPath() {
		return m, nil
	}
	if msg.status != "" {
		return m.applyStatusChange(msg.status)
	}
	return m.runMenuAction(actionsMenuResult{
		Done: true, Accepted: true,
		Path: msg.target.address, EpicName: msg.target.epicName, TicketID: msg.target.ticketID,
		Action: msg.action,
	})
}
