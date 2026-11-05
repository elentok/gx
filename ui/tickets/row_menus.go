package tickets

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/contextmenu"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/terminalrun"
	"github.com/elentok/gx/ui/tickets/tabapi"
)

// menuEnv is the tab state the menus read: the server APIs and the settings.
type menuEnv struct {
	serverAPI    tabapi.ServerAPI
	admin        tabapi.AdminAPI
	reads        tabapi.ReadsAPI
	sessions     tabapi.SessionsAPI
	tickets      tabapi.TicketsAPI
	queue        tabapi.QueueAPI
	link         tabapi.ServerLink
	settings     ui.Settings
	width        int
	worktreeRoot string
}

// menuSelection is the selected row, as the menus see it. ok is false with
// nothing selected; an epic row sets isEpic and epic, a ticket row sets epic
// and ticket.
type menuSelection struct {
	ok, isEpic bool
	epic       tickets.Epic
	ticket     tickets.Ticket
	archived   bool
	// session is the design session of the epic or ticket; queueAgent the
	// Agents-tab id of the ticket's PTY run.
	session, queueAgent string
	// queueable and actions feed the epic menu.
	queueable bool
	actions   []string
}

// ticketPath is the selected ticket row's path, or "" off a ticket.
func (s menuSelection) ticketPath() string {
	if !s.ok || s.isEpic {
		return ""
	}
	return s.ticket.Path
}

// menuTab is what the menus need from the tab that holds them. Neither tab
// model is visible to the menus, so the package they move to never imports
// the root.
type menuTab interface {
	env() menuEnv
	selection() menuSelection
	modalOpen() bool
	yankSummary() tea.Cmd
	yankFilePath() tea.Cmd
	editFile(terminalrun.SplitType) tea.Cmd
}

// pickResult is what a ticket menu pick asks of the tab: a Cmd, or the tab's
// own approve confirm, which opens a modal on the tab.
type pickResult struct {
	cmd     tea.Cmd
	approve bool
}

// rowMenus is the Context menu of a tab's rows: the ticket menu, the epic menu
// and the status writes they run. onQueue adds the Queue group.
type rowMenus struct {
	menu contextmenu.Menu[ticketTarget]
	// pending is the ticket path "m" is resolving the menu for; Esc or moving
	// the selection forgets it, so a late result is dropped.
	pending string
	onQueue bool
}

// handleKey applies the "m" keymap: the epic menu on an epic row, the ticket
// menu on a ticket row.
func (rm *rowMenus) handleKey(tab menuTab) tea.Cmd {
	sel := tab.selection()
	switch {
	case !sel.ok && rm.onQueue:
		return notify.Info("select a ticket to see its actions")
	case !sel.ok:
		return notify.Info("select a ticket or epic to see its actions")
	case sel.isEpic:
		rm.openEpic(tab.env(), sel)
		return nil
	}
	return rm.openTicket(tab.env(), sel)
}

// openEpic opens the epic menu at once: nothing in it needs liveness.
func (rm *rowMenus) openEpic(env menuEnv, sel menuSelection) {
	rm.menu = openEpicContextMenu(rm.menu, epicMenuInput{
		epic:         sel.epic,
		branch:       sel.epic.MergeTarget,
		queueable:    sel.queueable,
		serverReason: serverReason(env.serverAPI, env.link),
		archived:     sel.archived,
		session:      sel.session,
		actions:      sel.actions,
	})
}

func (rm *rowMenus) openTicket(env menuEnv, sel menuSelection) tea.Cmd {
	in := newTicketMenuInput(sel.epic, sel.ticket, env.settings.HerdrUnavailable, serverReason(env.serverAPI, env.link))
	in.archived = sel.archived
	in.session = sel.session
	in.queueAgent = sel.queueAgent
	if rm.onQueue {
		in.onQueue, in.hasProposal = true, hasPendingProposal(sel.ticket.Body)
	}
	rm.pending = sel.ticket.Path
	return cmdTicketMenu(env.reads, env.settings.Dirs, in)
}

// handleReady opens the resolved ticket menu if its ticket is still the
// selected one.
func (rm *rowMenus) handleReady(msg ticketMenuReadyMsg, tab menuTab) tea.Cmd {
	pending := rm.pending
	rm.pending = ""
	if !ticketMenuReadyApplies(msg, pending, tab.selection().ticketPath(), tab.modalOpen()) {
		return nil
	}
	rm.menu = rm.menu.Open(msg.target, msg.prompt, msg.items)
	return msg.toast()
}

// handleTicketPick runs a pick only while its ticket is still the selected
// one; the status writes and the edit and yank picks work on the selection.
func (rm *rowMenus) handleTicketPick(msg ticketMenuPickMsg, tab menuTab) pickResult {
	sel := tab.selection()
	env := tab.env()
	if msg.target.address != sel.ticketPath() {
		return pickResult{}
	}
	switch {
	case msg.status != "":
		return pickResult{cmd: statusChangeCmd(env.tickets, env.settings.TicketStore, sel.epic, sel.ticket, msg.status)}
	case rm.onQueue && msg.action == actionApprove:
		return pickResult{approve: true}
	}
	if cmd, ok := ticketMenuEditCopy(msg.action, tab.yankSummary, tab.yankFilePath, tab.editFile); ok {
		return pickResult{cmd: cmd}
	}
	return pickResult{cmd: dispatchPick(env, msg.target.result(msg.action))}
}

// handleEpicPick runs a pick only while its epic is still the selected row.
func (rm *rowMenus) handleEpicPick(msg epicMenuPickMsg, tab menuTab) tea.Cmd {
	sel := tab.selection()
	env := tab.env()
	if !sel.ok || !sel.isEpic || sel.epic.Path != msg.target.address {
		return nil
	}
	if msg.status != "" {
		return cmdApplyEpicStatus(env.tickets, env.settings.TicketStore, sel.epic, msg.status)
	}
	switch msg.action {
	case actionQueueEpic:
		return cmdQueueEpic(env.queue, sel.epic.Path)
	case actionEditEpic:
		return tab.editFile(terminalrun.InPlace)
	case actionYankEpic:
		return tab.yankSummary()
	}
	return dispatchPick(env, msg.target.result(msg.action))
}

// proposedRemedyHeading is the ticket section the server writes when a
// high-authority recovery proposes a remedy (server/recovery.go).
const proposedRemedyHeading = "Proposed Remedy"

func hasPendingProposal(body string) bool {
	return schema.Section(body, proposedRemedyHeading) != ""
}

// hasSuggestedActions keys a row's badge off the menu "m" would open on it.
func (rm *rowMenus) hasSuggestedActions(env menuEnv, epic tickets.Epic, ticket tickets.Ticket, archived bool) bool {
	in := newTicketMenuInput(epic, ticket, env.settings.HerdrUnavailable, serverReason(env.serverAPI, env.link))
	in.archived = archived
	return ticketHasSuggestedActions(in)
}

// resumeEnter is "enter" on a paused row in server mode: it runs the first
// verb of the ticket's detail.actions. handled is false for any other row so
// "enter" keeps its usual meaning.
func resumeEnter(tab menuTab) (cmd tea.Cmd, handled bool) {
	sel, env := tab.selection(), tab.env()
	if env.serverAPI == nil || !sel.ok || sel.isEpic || !isRepairPause(sel.ticket) {
		return nil, false
	}
	return cmdRunFirstAction(env.admin, env.reads, env.tickets, sel.ticket.Path), true
}

func dispatchPick(env menuEnv, result actionsMenuResult) tea.Cmd {
	return dispatchMenuAction(env.serverAPI, env.admin, env.reads, env.sessions, env.tickets, env.worktreeRoot, env.settings, env.width, result)
}
