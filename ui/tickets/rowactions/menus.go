package rowactions

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

// Env is the tab state the menus read: the server APIs and the settings.
type Env struct {
	ServerAPI    tabapi.ServerAPI
	Admin        tabapi.AdminAPI
	Reads        tabapi.ReadsAPI
	Sessions     tabapi.SessionsAPI
	Tickets      tabapi.TicketsAPI
	Queue        tabapi.QueueAPI
	Link         tabapi.ServerLink
	Settings     ui.Settings
	Width        int
	WorktreeRoot string
}

// Selection is the selected row, as the menus see it. OK is false with
// nothing selected; an epic row sets IsEpic and Epic, a ticket row sets Epic
// and Ticket.
type Selection struct {
	OK, IsEpic bool
	Epic       tickets.Epic
	Ticket     tickets.Ticket
	Archived   bool
	// Session is the design session of the epic or ticket; QueueAgent the
	// Agents-tab id of the ticket's PTY run.
	Session, QueueAgent string
	// Queueable and Actions feed the epic menu.
	Queueable bool
	Actions   []string
}

// TicketPath is the selected ticket row's path, or "" off a ticket.
func (s Selection) TicketPath() string {
	if !s.OK || s.IsEpic {
		return ""
	}
	return s.Ticket.Path
}

// Tab is what the menus need from the tab that holds them. Neither tab model
// is visible to the menus, so this package never imports the root.
type Tab interface {
	Env() Env
	Selection() Selection
	ModalOpen() bool
	YankSummary() tea.Cmd
	YankFilePath() tea.Cmd
	EditFile(terminalrun.SplitType) tea.Cmd
}

// PickResult is what a ticket menu pick asks of the tab: a Cmd, or the tab's
// own approve confirm, which opens a modal on the tab.
type PickResult struct {
	Cmd     tea.Cmd
	Approve bool
}

// Menus is the Context menu of a tab's rows: the ticket menu, the epic menu
// and the status writes they run. OnQueue adds the Queue group.
type Menus struct {
	Menu contextmenu.Menu[TicketTarget]
	// Pending is the ticket path "m" is resolving the menu for; Esc or moving
	// the selection forgets it, so a late result is dropped.
	Pending string
	OnQueue bool
}

// HandleKey applies the "m" keymap: the epic menu on an epic row, the ticket
// menu on a ticket row.
func (rm *Menus) HandleKey(tab Tab) tea.Cmd {
	sel := tab.Selection()
	switch {
	case !sel.OK && rm.OnQueue:
		return notify.Info("select a ticket to see its actions")
	case !sel.OK:
		return notify.Info("select a ticket or epic to see its actions")
	case sel.IsEpic:
		rm.openEpic(tab.Env(), sel)
		return nil
	}
	return rm.openTicket(tab.Env(), sel)
}

// openEpic opens the epic menu at once: nothing in it needs liveness.
func (rm *Menus) openEpic(env Env, sel Selection) {
	rm.Menu = openEpicContextMenu(rm.Menu, epicMenuInput{
		epic:         sel.Epic,
		branch:       sel.Epic.MergeTarget,
		queueable:    sel.Queueable,
		serverReason: serverReason(env.ServerAPI, env.Link),
		archived:     sel.Archived,
		session:      sel.Session,
		actions:      sel.Actions,
	})
}

func (rm *Menus) openTicket(env Env, sel Selection) tea.Cmd {
	in := newTicketMenuInput(sel.Epic, sel.Ticket, env.Settings.HerdrUnavailable, serverReason(env.ServerAPI, env.Link))
	in.archived = sel.Archived
	in.session = sel.Session
	in.queueAgent = sel.QueueAgent
	if rm.OnQueue {
		in.onQueue, in.hasProposal = true, HasPendingProposal(sel.Ticket.Body)
	}
	rm.Pending = sel.Ticket.Path
	return cmdTicketMenu(env.Reads, env.Settings.Dirs, in)
}

// HandleReady opens the resolved ticket menu if its ticket is still the
// selected one.
func (rm *Menus) HandleReady(msg TicketReadyMsg, tab Tab) tea.Cmd {
	pending := rm.Pending
	rm.Pending = ""
	if !ticketMenuReadyApplies(msg, pending, tab.Selection().TicketPath(), tab.ModalOpen()) {
		return nil
	}
	rm.Menu = rm.Menu.Open(msg.Target, msg.Prompt, msg.Items)
	return msg.Toast()
}

// HandleTicketPick runs a pick only while its ticket is still the selected
// one; the status writes and the edit and yank picks work on the selection.
func (rm *Menus) HandleTicketPick(msg TicketPickMsg, tab Tab) PickResult {
	sel := tab.Selection()
	env := tab.Env()
	if msg.Target.Address != sel.TicketPath() {
		return PickResult{}
	}
	switch {
	case msg.Status != "":
		return PickResult{Cmd: statusChangeCmd(env.Tickets, env.Settings.TicketStore, sel.Epic, sel.Ticket, msg.Status)}
	case rm.OnQueue && msg.Action == actionApprove:
		return PickResult{Approve: true}
	}
	if cmd, ok := ticketMenuEditCopy(msg.Action, tab.YankSummary, tab.YankFilePath, tab.EditFile); ok {
		return PickResult{Cmd: cmd}
	}
	return PickResult{Cmd: dispatchPick(env, targetResult(msg.Target, msg.Action))}
}

// HandleEpicPick runs a pick only while its epic is still the selected row.
func (rm *Menus) HandleEpicPick(msg EpicPickMsg, tab Tab) tea.Cmd {
	sel := tab.Selection()
	env := tab.Env()
	if !sel.OK || !sel.IsEpic || sel.Epic.Path != msg.Target.Address {
		return nil
	}
	if msg.Status != "" {
		return cmdApplyEpicStatus(env.Tickets, env.Settings.TicketStore, sel.Epic, msg.Status)
	}
	switch msg.Action {
	case actionQueueEpic:
		return cmdQueueEpic(env.Queue, sel.Epic.Path)
	case actionEditEpic:
		return tab.EditFile(terminalrun.InPlace)
	case actionYankEpic:
		return tab.YankSummary()
	}
	return dispatchPick(env, targetResult(msg.Target, msg.Action))
}

// proposedRemedyHeading is the ticket section the server writes when a
// high-authority recovery proposes a remedy (server/recovery.go).
const proposedRemedyHeading = "Proposed Remedy"

// HasPendingProposal reports whether body carries a recovery proposal.
func HasPendingProposal(body string) bool {
	return schema.Section(body, proposedRemedyHeading) != ""
}

// HasSuggestedActions keys a row's badge off the menu "m" would open on it.
func (rm *Menus) HasSuggestedActions(env Env, epic tickets.Epic, ticket tickets.Ticket, archived bool) bool {
	in := newTicketMenuInput(epic, ticket, env.Settings.HerdrUnavailable, serverReason(env.ServerAPI, env.Link))
	in.archived = archived
	return ticketHasSuggestedActions(in)
}

// ResumeEnter is "enter" on a paused row in server mode: it runs the first
// verb of the ticket's detail.actions. handled is false for any other row so
// "enter" keeps its usual meaning.
func ResumeEnter(tab Tab) (cmd tea.Cmd, handled bool) {
	sel, env := tab.Selection(), tab.Env()
	if env.ServerAPI == nil || !sel.OK || sel.IsEpic || !isRepairPause(sel.Ticket) {
		return nil, false
	}
	return cmdRunFirstAction(env.Admin, env.Reads, env.Tickets, sel.Ticket.Path), true
}

func dispatchPick(env Env, result actionsMenuResult) tea.Cmd {
	return dispatchMenuAction(env.ServerAPI, env.Admin, env.Reads, env.Sessions, env.Tickets, env.WorktreeRoot, env.Settings, env.Width, result)
}
