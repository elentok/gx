package tickets

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui/contextmenu"
	"github.com/elentok/gx/ui/terminalrun"
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
	archived             bool
	// serverReason is why orchestration writes are off; empty when usable.
	serverReason string
	// onQueue adds the Queue group, which only the Queue tab offers.
	onQueue, hasProposal bool
}

const (
	ticketFinished = "ticket is finished"
	alreadyParked  = "already parked"
	notParked      = "not parked"
	notMuted       = "not muted"
	herdrDownText  = "herdr not running"
	noAgentLog     = "no agent log"
	archivedText   = "archived"
	noProposal     = "no recovery proposal"
)

const actionApprove = "approve"

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
	watchable := in.nativeLive || (in.paneLive && !in.herdrDown)
	items = append(items,
		pick("Watch", "", actionWatch, contextmenu.Agent, reasonIf(!watchable, noAgentLog)),
		pick("Transcript", "", actionTranscript, contextmenu.Agent, ""))
	if !investigateSuggested(in.rendered) {
		items = append(items, pick("Investigate", "", actionInvestigate, contextmenu.Agent, reasonIf(in.herdrDown, herdrDownText)))
	}

	if in.onQueue {
		items = append(items, pick("Approve recovery proposal", "A", actionApprove, contextmenu.Queue,
			firstReason(in.serverReason, reasonIf(!in.hasProposal, noProposal))))
	}

	for _, e := range ticketMenuEdits {
		items = append(items, pick(e.label, e.key, e.action, contextmenu.EditCopy, ""))
	}
	items = append(items,
		pick("Yank epic - ticket", "y y", actionYankSummary, contextmenu.EditCopy, ""),
		pick("Yank file path", "y f", actionYankFilePath, contextmenu.EditCopy, ""))

	if in.archived {
		for i, it := range items {
			if !archivedAllowed(it) {
				items[i].Disabled = archivedText
			}
		}
	}
	return items
}

// archivedAllowed reports whether an item reads rather than writes, so an
// archived row keeps it.
func archivedAllowed(it contextmenu.Item[ticketTarget]) bool {
	return it.Group == contextmenu.EditCopy || it.Label == "Watch" || it.Label == "Transcript"
}

const (
	actionYankSummary  = "yank-summary"
	actionYankFilePath = "yank-file-path"
)

// ticketMenuEdits mirrors the e-prefix chords in newTicketsManager.
var ticketMenuEdits = []struct {
	label, key, action string
	split              terminalrun.SplitType
}{
	{"Edit file", "e e", "edit", terminalrun.InPlace},
	{"Edit file (split)", "e s", "edit-hsplit", terminalrun.HSplit},
	{"Edit file (vsplit)", "e v", "edit-vsplit", terminalrun.VSplit},
	{"Edit file (tab)", "e t", "edit-tab", terminalrun.Tab},
}

// ticketMenuEditCopy runs an Edit/copy pick through the tab's chord handlers,
// so the menu and the chord can't drift; handled is false for other picks.
func ticketMenuEditCopy(action string, yankSummary, yankFilePath func() tea.Cmd, edit func(terminalrun.SplitType) tea.Cmd) (cmd tea.Cmd, handled bool) {
	switch action {
	case actionYankSummary:
		return yankSummary(), true
	case actionYankFilePath:
		return yankFilePath(), true
	}
	for _, e := range ticketMenuEdits {
		if e.action == action {
			return edit(e.split), true
		}
	}
	return nil, false
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
func serverReason(api ServerAPI, link ServerLink) string {
	if api == nil || link == ServerLinkDown {
		return serverNotRunning
	}
	return link.disabledReason()
}

// ticketMenuReadyMsg carries a ticket's Context menu whose agent liveness was
// resolved off the Update goroutine.
type ticketMenuReadyMsg struct {
	target ticketTarget
	prompt string
	items  []contextmenu.Item[ticketTarget]
}

// newTicketMenuInput is what both tabs read from a ticket row when "m" is
// pressed; liveness is left to cmdTicketMenu.
func newTicketMenuInput(epic tickets.Epic, ticket tickets.Ticket, herdrDown bool, serverReason string) ticketMenuInput {
	return ticketMenuInput{
		rendered:     epic.RenderedStatus(ticket),
		ticket:       ticket,
		isMap:        epic.IsMap,
		herdrDown:    herdrDown,
		serverReason: serverReason,
	}
}

// cmdTicketMenu resolves liveness in a Cmd (it shells out to ps), so the menu
// opens once the result comes back.
func cmdTicketMenu(api ServerAPI, in ticketMenuInput) tea.Cmd {
	address := in.ticket.Path
	// A row's Path is its address and its epic is "project:epic"; the herdr
	// launches want the bare epic.
	epicName, _, _ := tickets.SplitTrailerValue(address)
	target := ticketTarget{address: address, epicName: epicName, ticketID: in.ticket.DisplayNumber()}
	prompt := fmt.Sprintf("Ticket %q", in.ticket.Title)
	return func() tea.Msg {
		it, live := serverIteration(api, address)
		in.paneLive = live && it.Tab != ""
		in.nativeLive = nativeAgentLive(address)
		return ticketMenuReadyMsg{target: target, prompt: prompt, items: ticketMenuItems(in)}
	}
}

// ticketMenuReadyApplies reports whether a resolved menu may open: the user
// may have moved on, or opened another overlay, while ps ran.
func ticketMenuReadyApplies(msg ticketMenuReadyMsg, pending, selected string, modalOpen bool) bool {
	address := msg.target.address
	return pending == address && selected == address && !modalOpen
}

func (t ticketTarget) result(action string) actionsMenuResult {
	return actionsMenuResult{
		Done: true, Accepted: true,
		Path: t.address, EpicName: t.epicName, TicketID: t.ticketID,
		Action: action,
	}
}

func (m Model) openTicketMenu(epic tickets.Epic, ticket tickets.Ticket, archived bool) (tea.Model, tea.Cmd) {
	in := newTicketMenuInput(epic, ticket, m.settings.HerdrUnavailable, serverReason(m.serverAPI, m.link))
	in.archived = archived
	m.ticketMenuPending = ticket.Path
	return m, cmdTicketMenu(m.serverAPI, in)
}

func (m Model) handleTicketMenuReady(msg ticketMenuReadyMsg) Model {
	pending := m.ticketMenuPending
	m.ticketMenuPending = ""
	if ticketMenuReadyApplies(msg, pending, m.selectedTicketPath(), m.ModalOpen()) {
		m.ticketMenu = m.ticketMenu.Open(msg.target, msg.prompt, msg.items)
	}
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
	if cmd, ok := ticketMenuEditCopy(msg.action, m.yankTicketSummary, m.yankTicketFilePath, m.cmdEditSelectedFile); ok {
		return m, cmd
	}
	return m, dispatchMenuAction(m.serverAPI, m.worktreeRoot, m.settings, m.width, msg.target.result(msg.action))
}
