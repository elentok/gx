package rowactions

import (
	"fmt"
	"slices"
	"strings"

	"github.com/elentok/gx/server/wire"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui/contextmenu"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/terminalrun"
	"github.com/elentok/gx/ui/tickets/rowfmt"
	"github.com/elentok/gx/ui/tickets/tabapi"
)

// TicketPickMsg carries a menu pick back to Update: the actions need the
// model, which an item's Run cannot reach. Status is set for a Status pick.
type TicketPickMsg struct {
	Target TicketTarget
	Action string
	Status schema.Status
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
	detail *wire.TicketDetail
}

const (
	ticketFinished = "ticket is finished"
	alreadyPaused  = "already paused"
	notPaused      = "not paused"
	notMuted       = "not muted"
	herdrDownText  = "herdr not running"
	holderGoneText = "agent is gone"
	archivedText   = "archived"
	noProposal     = "no recovery proposal"
)

const actionApprove = "approve"

// ticketMenuItems lists every action for a ticket. Each is built once in its
// own group and moved to Suggested when it is suggested; one that doesn't
// apply is dim with the reason.
func ticketMenuItems(in ticketMenuInput) []contextmenu.Item[TicketTarget] {
	item := func(label, key string, group contextmenu.Group, disabled string, msg TicketPickMsg) contextmenu.Item[TicketTarget] {
		if in.archived && !archivedAllowed(group, msg.Action) {
			disabled = archivedText
		}
		return contextmenu.Item[TicketTarget]{
			Label: label, Key: key, Group: group, Disabled: disabled,
			Run: func(t TicketTarget) tea.Cmd {
				msg := msg
				msg.Target = t
				return func() tea.Msg { return msg }
			},
		}
	}
	pick := func(label, key, action string, group contextmenu.Group, disabled string) contextmenu.Item[TicketTarget] {
		return item(label, key, group, disabled, TicketPickMsg{Action: action})
	}
	finished := ""
	if in.rendered.Terminal() {
		finished = ticketFinished
	}
	server := func(label, key, action, why string) contextmenu.Item[TicketTarget] {
		return pick(label, key, action, contextmenu.Server, firstReason(in.serverReason, finished, why))
	}
	suggest := func(suggested bool, it contextmenu.Item[TicketTarget]) contextmenu.Item[TicketTarget] {
		if suggested {
			it.Group = contextmenu.Suggested
		}
		return it
	}
	paused := isRepairPause(in.ticket)
	answer := ResumeSuggested(in.rendered)
	// The first recovery item is what "enter" runs on a repair pause; an answer
	// pause's verbs need their own confirmation, so they get no hint.
	var recovery []contextmenu.Item[TicketTarget]
	for i, a := range RecoveryActions(in.detail) {
		it := server(a.Label, reasonIf(paused && i == 0, "enter"), RecoverPrefix+a.Word, "")
		it.Help = a.Help
		if a.Recommended {
			it.Label += RecommendedMark
		}
		recovery = append(recovery, suggest(answer || a.Recommended, it))
	}

	var items []contextmenu.Item[TicketTarget]
	if in.isMap && in.session != "" {
		items = append(items, GoToSessionItem(in.session))
	}
	if in.isMap && in.rendered == tickets.StatusOpen {
		items = append(items, pick(StartSessionLabel, "", ActionStartSession, contextmenu.Suggested, ""))
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
		items = append(items, item("Set "+string(status), "", contextmenu.Status, why, TicketPickMsg{Status: status}))
	}

	unmute := UnmuteSuggested(in.ticket)
	items = append(items, server("Pause", "", serverVerbPause, reasonIf(paused, alreadyPaused)))
	items = append(items, recovery...)
	items = append(items,
		server("Cancel", "", serverVerbCancel, ""),
		// A mute outlives the status (see actionUnmuteReopen), so a finished
		// ticket keeps its Unmute.
		suggest(unmute, pick("Unmute & Reopen", "", ActionUnmuteReopen, contextmenu.Server,
			firstReason(in.serverReason, reasonIf(!unmute, firstReason(finished, notMuted))))),
		pick("Watch", "", ActionWatch, contextmenu.Agent, in.watchReason),
		pick("Transcript", "", ActionTranscript, contextmenu.Agent, ""),
		suggest(InvestigateSuggested(in.rendered),
			pick("Investigate", "", ActionInvestigate, contextmenu.Agent, "")))
	switch {
	case in.queueAgent != "":
		// A PTY pane is the Agents tab's live panel, so herdr being down is moot.
		it := pick("Go to pane", "", ActionGoToPane, contextmenu.Agent, reasonIf(!in.ptyLive, holderGoneText))
		if it.Disabled == "" {
			id := in.queueAgent
			it.Run = func(TicketTarget) tea.Cmd { return func() tea.Msg { return tabapi.GoToSessionMsg{ID: id} } }
		}
		items = append(items, it)
	case in.paneLive:
		items = append(items, pick("Go to pane", "", ActionGoToPane, contextmenu.Agent, reasonIf(in.herdrDown, herdrDownText)))
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
	return group == contextmenu.EditCopy || action == ActionWatch || action == ActionTranscript
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
func serverReason(api tabapi.ServerAPI, link tabapi.ServerLink) string {
	if api == nil || link == tabapi.ServerLinkDown {
		return ServerNotRunning
	}
	return link.DisabledReason()
}

// TicketReadyMsg carries a ticket's Context menu whose agent liveness was
// resolved off the Update goroutine.
type TicketReadyMsg struct {
	Target TicketTarget
	Prompt string
	Items  []contextmenu.Item[TicketTarget]
	// DetailErr is the failed detail read; the menu opens without recovery items.
	DetailErr error
}

// Toast shows a failed detail read on the status line.
func (msg TicketReadyMsg) Toast() tea.Cmd {
	if msg.DetailErr == nil {
		return nil
	}
	return notify.Error("ticket detail: " + msg.DetailErr.Error())
}

// newTicketMenuInput is what both tabs read from a ticket row when "m" is
// pressed; liveness is left to cmdTicketMenu.
func newTicketMenuInput(epic tickets.Epic, ticket tickets.Ticket, herdrDown bool, serverReason string) ticketMenuInput {
	return ticketMenuInput{
		rendered:     rowfmt.TicketStatus(epic, ticket),
		ticket:       ticket,
		isMap:        epic.IsMap,
		herdrDown:    herdrDown,
		serverReason: serverReason,
	}
}

// cmdTicketMenu resolves liveness in a Cmd (it shells out to ps), so the menu
// opens once the result comes back.
func cmdTicketMenu(api tabapi.ReadsAPI, dirs config.Dirs, in ticketMenuInput) tea.Cmd {
	address := in.ticket.Path
	// A row's Path is its address and its epic is "project:epic"; the herdr
	// launches want the bare epic.
	epicName, _, _ := tickets.SplitTrailerValue(address)
	target := TicketTarget{Address: address, EpicName: epicName, TicketID: in.ticket.DisplayNumber()}
	prompt := fmt.Sprintf("Ticket %q", in.ticket.Title)
	return func() tea.Msg {
		api := SnapshotIterations(api)
		it, live := ServerIteration(api, address)
		in.paneLive = live && it.Tab != ""
		if live && it.QueueAgent != "" {
			in.queueAgent, in.ptyLive = it.QueueAgent, true
		}
		in.watchReason = WatchReason(api, dirs, AgentAddress(address, target.TicketID))
		var err error
		in.detail, err = TicketDetail(api, address)
		return TicketReadyMsg{Target: target, Prompt: prompt, Items: ticketMenuItems(in), DetailErr: err}
	}
}

// ticketMenuReadyApplies reports whether a resolved menu may open: the user
// may have moved on, or opened another overlay, while ps ran.
func ticketMenuReadyApplies(msg TicketReadyMsg, pending, selected string, modalOpen bool) bool {
	address := msg.Target.Address
	return pending == address && selected == address && !modalOpen
}

// ticketHasSuggestedActions reports whether a row carries the "m" badge
// (ui.IconSet.SuggestedAction): its Context menu has an enabled Suggested
// item.
func ticketHasSuggestedActions(in ticketMenuInput) bool {
	// The row has no detail yet, so an answer pause's recovery items (suggested
	// once the menu opens) are counted here.
	if ResumeSuggested(in.rendered) && in.serverReason == "" && !in.archived {
		return true
	}
	return slices.ContainsFunc(ticketMenuItems(in), func(it contextmenu.Item[TicketTarget]) bool {
		return it.Group == contextmenu.Suggested && it.Disabled == ""
	})
}

func targetResult(t TicketTarget, action string) actionsMenuResult {
	return actionsMenuResult{
		Done: true, Accepted: true,
		Path: t.Address, EpicName: t.EpicName, TicketID: t.TicketID,
		Action: action,
	}
}
