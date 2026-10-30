package tickets

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui/contextmenu"
	"github.com/elentok/gx/ui/notify"
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
	rendered tickets.RenderedStatus
	ticket   tickets.Ticket
	isMap    bool
	// paneLive marks a live herdr agent, which has a tab to focus.
	paneLive bool
	// queueAgent is the Agents-tab row id of a PTY run; ptyLive is whether its
	// holder is still there.
	queueAgent string
	ptyLive    bool
	herdrDown  bool
	archived   bool
	// watchReason is why Watch is dim; empty when it can open.
	watchReason string
	// serverReason is why orchestration writes are off; empty when usable.
	serverReason string
	// onQueue adds the Queue group, which only the Queue tab offers.
	onQueue, hasProposal bool
	// session is the design session Go to design session… opens; "" offers none.
	session string
	// detail is the ticket's server detail, read off the Update goroutine; nil
	// (no server, or the read failed) offers no recovery items.
	detail *server.TicketDetail
}

const (
	ticketFinished = "ticket is finished"
	alreadyPaused  = "already paused"
	notPaused      = "not paused"
	notMuted       = "not muted"
	herdrDownText  = "herdr not running"
	holderGoneText = "agent is gone"
	noAgentLog     = "no agent log"
	archivedText   = "archived"
	noProposal     = "no recovery proposal"
)

const actionApprove = "approve"

// ticketMenuItems lists every action for a ticket. Each is built once in its
// own group and moved to Suggested when it is suggested; one that doesn't
// apply is dim with the reason.
func ticketMenuItems(in ticketMenuInput) []contextmenu.Item[ticketTarget] {
	item := func(label, key string, group contextmenu.Group, disabled string, msg ticketMenuPickMsg) contextmenu.Item[ticketTarget] {
		if in.archived && !archivedAllowed(group, msg.action) {
			disabled = archivedText
		}
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
	suggest := func(suggested bool, it contextmenu.Item[ticketTarget]) contextmenu.Item[ticketTarget] {
		if suggested {
			it.Group = contextmenu.Suggested
		}
		return it
	}
	paused := isRepairPause(in.ticket)
	answer := resumeSuggested(in.rendered)
	// The first recovery item is what "enter" runs on a repair pause; an answer
	// pause's verbs need their own confirmation, so they get no hint.
	var recovery []contextmenu.Item[ticketTarget]
	for i, a := range recoveryActions(in.detail) {
		it := server(a.label, reasonIf(paused && i == 0, "enter"), recoverPrefix+a.word, "")
		it.Help = a.help
		if a.recommended {
			it.Label += recommendedMark
		}
		recovery = append(recovery, suggest(answer || a.recommended, it))
	}

	var items []contextmenu.Item[ticketTarget]
	if in.isMap && in.session != "" {
		items = append(items, goToSessionItem(in.session))
	}
	if in.isMap && in.rendered == tickets.StatusOpen {
		items = append(items, pick(startSessionLabel, "", actionStartSession, contextmenu.Suggested, ""))
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

	unmute := unmuteSuggested(in.ticket)
	items = append(items, server("Pause", "", serverVerbPause, reasonIf(paused, alreadyPaused)))
	items = append(items, recovery...)
	items = append(items,
		server("Cancel", "", serverVerbCancel, ""),
		// A mute outlives the status (see actionUnmuteReopen), so a finished
		// ticket keeps its Unmute.
		suggest(unmute, pick("Unmute & Reopen", "", actionUnmuteReopen, contextmenu.Server,
			firstReason(in.serverReason, reasonIf(!unmute, firstReason(finished, notMuted))))),
		pick("Watch", "", actionWatch, contextmenu.Agent, in.watchReason),
		pick("Transcript", "", actionTranscript, contextmenu.Agent, ""),
		suggest(investigateSuggested(in.rendered),
			pick("Investigate", "", actionInvestigate, contextmenu.Agent, reasonIf(in.herdrDown, herdrDownText))))
	switch {
	case in.queueAgent != "":
		// A PTY pane is the Agents tab's live panel, so herdr being down is moot.
		it := pick("Go to pane", "", actionGoToPane, contextmenu.Agent, reasonIf(!in.ptyLive, holderGoneText))
		if it.Disabled == "" {
			id := in.queueAgent
			it.Run = func(ticketTarget) tea.Cmd { return func() tea.Msg { return GoToSessionMsg{ID: id} } }
		}
		items = append(items, it)
	case in.paneLive:
		items = append(items, pick("Go to pane", "", actionGoToPane, contextmenu.Agent, reasonIf(in.herdrDown, herdrDownText)))
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
	return items
}

// archivedAllowed reports whether an item reads rather than writes, so an
// archived row keeps it. Edits open an archived file read-only.
func archivedAllowed(group contextmenu.Group, action string) bool {
	return group == contextmenu.EditCopy || action == actionWatch || action == actionTranscript
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
	// detailErr is the failed detail read; the menu opens without recovery items.
	detailErr error
}

// toast shows a failed detail read on the status line.
func (msg ticketMenuReadyMsg) toast() tea.Cmd {
	if msg.detailErr == nil {
		return nil
	}
	return notify.Error("ticket detail: " + msg.detailErr.Error())
}

// newTicketMenuInput is what both tabs read from a ticket row when "m" is
// pressed; liveness is left to cmdTicketMenu.
func newTicketMenuInput(epic tickets.Epic, ticket tickets.Ticket, herdrDown bool, serverReason string) ticketMenuInput {
	return ticketMenuInput{
		rendered:     ticketStatus(epic, ticket),
		ticket:       ticket,
		isMap:        epic.IsMap,
		herdrDown:    herdrDown,
		serverReason: serverReason,
	}
}

// cmdTicketMenu resolves liveness in a Cmd (it shells out to ps), so the menu
// opens once the result comes back.
func cmdTicketMenu(api ServerAPI, dirs config.Dirs, in ticketMenuInput) tea.Cmd {
	address := in.ticket.Path
	// A row's Path is its address and its epic is "project:epic"; the herdr
	// launches want the bare epic.
	epicName, _, _ := tickets.SplitTrailerValue(address)
	target := ticketTarget{address: address, epicName: epicName, ticketID: in.ticket.DisplayNumber()}
	prompt := fmt.Sprintf("Ticket %q", in.ticket.Title)
	return func() tea.Msg {
		api := snapshotIterations(api)
		it, live := serverIteration(api, address)
		in.paneLive = live && it.Tab != ""
		if live && it.QueueAgent != "" {
			in.queueAgent, in.ptyLive = it.QueueAgent, true
		}
		in.watchReason = watchReason(api, dirs, agentAddress(address, target.ticketID))
		var err error
		in.detail, err = ticketDetail(api, address)
		return ticketMenuReadyMsg{target: target, prompt: prompt, items: ticketMenuItems(in), detailErr: err}
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

// rowHasSuggestedActions keys a row's badge off the menu "m" would open on it.
func (m Model) rowHasSuggestedActions(epic tickets.Epic, ticket tickets.Ticket, archived bool) bool {
	in := newTicketMenuInput(epic, ticket, m.settings.HerdrUnavailable, serverReason(m.serverAPI, m.link))
	in.archived = archived
	return ticketHasSuggestedActions(in)
}

func (m QueueModel) rowHasSuggestedActions(epic tickets.Epic, ticket tickets.Ticket) bool {
	return ticketHasSuggestedActions(newTicketMenuInput(epic, ticket, m.settings.HerdrUnavailable, serverReason(m.serverAPI, m.link)))
}

func (m Model) openTicketMenu(epic tickets.Epic, ticket tickets.Ticket, archived bool) (tea.Model, tea.Cmd) {
	in := newTicketMenuInput(epic, ticket, m.settings.HerdrUnavailable, serverReason(m.serverAPI, m.link))
	in.archived = archived
	in.session = ticketSession(m.vm.Sessions, epic.Path, ticket)
	in.queueAgent = queueAgentOf(m.vm.Sessions, ticket.Path)
	m.ticketMenuPending = ticket.Path
	return m, cmdTicketMenu(m.serverAPI, m.settings.Dirs, in)
}

// menuAnchor is the selected sidebar row on screen, under the panel's
// header line, for the Context menu to open next to.
func (m Model) menuAnchor() contextmenu.Anchor {
	line, h, ok := m.sidebarTree.SelectedBodyLine()
	if !ok {
		return contextmenu.Anchor{}
	}
	return contextmenu.Anchor{X: 1, Y: 1 + line, H: h}
}

// menuAnchor is the selected queue row on screen, under the panel's header
// line and the queue header, for the Context menu to open next to.
func (m QueueModel) menuAnchor() contextmenu.Anchor {
	line, h, ok := m.queueTree.SelectedBodyLine()
	if !ok {
		return contextmenu.Anchor{}
	}
	return contextmenu.Anchor{X: 1, Y: 1 + queueHeaderReservedLines + line, H: h}
}

func (m Model) handleTicketMenuReady(msg ticketMenuReadyMsg) (Model, tea.Cmd) {
	pending := m.ticketMenuPending
	m.ticketMenuPending = ""
	if !ticketMenuReadyApplies(msg, pending, m.selectedTicketPath(), m.ModalOpen()) {
		return m, nil
	}
	m.ticketMenu = m.ticketMenu.Open(msg.target, msg.prompt, msg.items)
	return m, msg.toast()
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
