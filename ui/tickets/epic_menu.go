package tickets

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui/contextmenu"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/terminalrun"
)

// epicMenuPickMsg carries an epic Context menu pick back to Update. status is
// set for a Status pick.
type epicMenuPickMsg struct {
	target ticketTarget
	action string
	status schema.Status
}

const (
	ticketsNotDone        = "tickets not done"
	alreadyMerged         = "already merged"
	nothingToQueue        = "nothing to queue"
	followUpsNotQueueable = "follow-ups are not schedulable work"
	actionQueueEpic       = "queue-epic"
	actionEditEpic        = "edit-epic"
	actionYankEpic        = "yank-epic"
)

var epicDirectStatuses = []schema.Status{schema.StatusDraft, schema.StatusOpen}

// epicMenuInput is what epicMenuItems reads from an epic row.
type epicMenuInput struct {
	epic tickets.Epic
	// branch is the branch the epic merges into; "" when unknown.
	branch string
	// queueable is whether Add epic to queue has anything to send.
	queueable    bool
	serverReason string
	archived     bool
	// session is the design session Go to design session… opens; "" offers none.
	session string
	// actions are the verbs of the epic's blocked merge, from its server
	// summary; empty when the merge is not blocked.
	actions []string
}

// epicMenuItems lists every action for an epic, in the shape of
// ticketMenuItems: an action that applies is Suggested, one that doesn't is
// dim with the reason.
func epicMenuItems(in epicMenuInput) []contextmenu.Item[ticketTarget] {
	item := func(label, key string, group contextmenu.Group, disabled string, msg epicMenuPickMsg) contextmenu.Item[ticketTarget] {
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
		return item(label, key, group, disabled, epicMenuPickMsg{action: action})
	}
	epic := in.epic

	var items []contextmenu.Item[ticketTarget]
	if epic.IsMap {
		if in.session != "" {
			items = append(items, goToSessionItem(in.session))
		}
		items = append(items, pick(startSessionLabel, "", actionStartSession, contextmenu.Suggested, ""))
	}
	mergeWhy := firstReason(reasonIf(!epic.AllDone(), ticketsNotDone), reasonIf(!epic.MergedAt.IsZero(), alreadyMerged))
	mergeGroup := contextmenu.Server
	if mergeWhy == "" {
		mergeGroup = contextmenu.Suggested
	}
	mergeLabel := "Merge"
	if in.branch != "" {
		mergeLabel += " into " + in.branch
	}
	mergeItem := pick(mergeLabel, "", actionMerge, mergeGroup, firstReason(in.serverReason, mergeWhy))
	if len(in.actions) == 0 {
		items = append(items, mergeItem)
	}
	// A blocked merge offers the verbs of its pause in place of plain Merge. The
	// server only sends them for a finished epic, so unfinished tickets in the
	// row (it may lag) don't dim them.
	blockedWhy := firstReason(in.serverReason, reasonIf(!epic.MergedAt.IsZero(), alreadyMerged))
	if len(in.actions) > 0 {
		mergeItem.Disabled, mergeItem.Group = blockedWhy, contextmenu.Suggested
	}
	for _, word := range in.actions {
		v, ok := schema.VerbOfWord(word)
		switch word {
		case server.ActionStashMerge:
			it := pick(v.Label(), "", actionStashMerge, contextmenu.Suggested, blockedWhy)
			it.Help = v.Tooltip()
			items = append(items, it)
		case server.ActionMerge:
			if ok {
				mergeItem.Label = v.Label()
				mergeItem.Help = v.Tooltip()
			}
			items = append(items, mergeItem)
		}
	}

	current := schema.Status(strings.ToLower(strings.TrimSpace(epic.Status)))
	for _, status := range epicDirectStatuses {
		items = append(items, item("Set "+string(status), "", contextmenu.Status,
			reasonIf(status == current, "already "+string(status)), epicMenuPickMsg{status: status}))
	}

	items = append(items,
		pick("Add epic to queue", "", actionQueueEpic, contextmenu.Queue,
			firstReason(in.serverReason, queueWhy(epic, in.queueable))),
		pick("Edit ticket.md", "e e", actionEditEpic, contextmenu.EditCopy, ""),
		pick("Yank", "y y", actionYankEpic, contextmenu.EditCopy, ""))

	if in.archived {
		for i, it := range items {
			if it.Group != contextmenu.EditCopy {
				items[i].Disabled = archivedText
			}
		}
	}
	return items
}

// openEpicMenu opens the Context menu on epic. Nothing in it needs liveness,
// so it opens at once.
func (m Model) openEpicMenu(epic tickets.Epic, archived bool) Model {
	m.ticketMenu = openEpicContextMenu(m.ticketMenu, epicMenuInput{
		epic:         epic,
		branch:       epic.MergeTarget,
		queueable:    epicQueueable(epic, m.vm.Queue),
		serverReason: serverReason(m.serverAPI, m.link),
		archived:     archived,
		session:      epicSession(m.vm.Sessions, epic.Path),
		actions:      epicSummaryActions(m.vm.Epics, epic.Name),
	})
	return m
}

// epicSummaryActions is the Actions of the summary for address, nil when the
// server sent none.
func epicSummaryActions(sums []server.EpicSummary, address string) []string {
	for _, s := range sums {
		if s.Address == address {
			return s.Actions
		}
	}
	return nil
}

func openEpicContextMenu(menu contextmenu.Menu[ticketTarget], in epicMenuInput) contextmenu.Menu[ticketTarget] {
	// In server mode Name is the "project:epic" address the merge and start
	// session want; Path is what the selection is matched against.
	target := ticketTarget{address: in.epic.Path, epicName: in.epic.Name}
	return menu.Open(target, fmt.Sprintf("Epic %q", in.epic.Name), epicMenuItems(in))
}

// selectedEpic is the selected epic row's epic; ok is false off an epic row.
func (m Model) selectedEpic() (tickets.Epic, bool) {
	r, ok := m.selectedRow()
	if !ok || !r.isEpic() {
		return tickets.Epic{}, false
	}
	return m.epicAt(r), true
}

// handleEpicMenuPick runs a pick only while its epic is still the selected
// row; the edit and yank picks work on the selection.
func (m Model) handleEpicMenuPick(msg epicMenuPickMsg) (tea.Model, tea.Cmd) {
	epic, ok := m.selectedEpic()
	if !ok || epic.Path != msg.target.address {
		return m, nil
	}
	if msg.status != "" {
		return m, cmdApplyEpicStatus(m.serverAPI, m.settings.TicketStore, epic, msg.status)
	}
	switch msg.action {
	case actionQueueEpic:
		return m, cmdQueueEpic(m.serverAPI, epic.Path)
	case actionEditEpic:
		return m, m.cmdEditSelectedFile(terminalrun.InPlace)
	case actionYankEpic:
		return m, m.yankTicketSummary()
	}
	return m, dispatchMenuAction(m.serverAPI, m.worktreeRoot, m.settings, m.width, msg.target.result(msg.action))
}

// queueWhy is the reason Add epic to queue is dim, or "" when it applies.
func queueWhy(epic tickets.Epic, queueable bool) string {
	switch epic.QueueRefusal() {
	case tickets.QueueRefusalFollowUps:
		return followUpsNotQueueable
	case tickets.QueueRefusalNotOpen:
		return "epic is " + strings.TrimSpace(epic.Status) + ", not open"
	}
	return reasonIf(!queueable, nothingToQueue)
}

// epicQueueable reports whether epic has an open ticket and no queue entry
// yet. It goes by the frontmatter status, not the rendered one, so blocked
// tickets count and the server decides on them.
func epicQueueable(epic tickets.Epic, queue []string) bool {
	if epic.QueueRefusal() != tickets.QueueRefusalNone || slices.Contains(queue, epic.Path) {
		return false
	}
	return slices.ContainsFunc(epic.Tickets, func(t tickets.Ticket) bool {
		open := schema.Status(strings.ToLower(strings.TrimSpace(t.Status))) == schema.StatusOpen
		return open && !slices.Contains(queue, t.Path)
	})
}

// epicQueuedMsg reports a finished Add epic to queue: how many addresses the
// server took and the refusals as "address: reason".
type epicQueuedMsg struct {
	queued   int
	refusals []string
}

func (msg epicQueuedMsg) toast() tea.Cmd {
	return QueuedToast(msg.queued, msg.refusals)
}

// QueuedToast is the toast for a finished queue run: a warning listing the
// refusals, else a success.
func QueuedToast(queued int, refusals []string) tea.Cmd {
	text := fmt.Sprintf("queued %d, refused %d", queued, len(refusals))
	if len(refusals) == 0 {
		return notify.Success(text)
	}
	return notify.Warning(text + " (" + strings.Join(refusals, "; ") + ")")
}

// cmdQueueEpic queues the epic as one entry and reports the outcome as an
// epicQueuedMsg.
func cmdQueueEpic(api ServerAPI, epicAddress string) tea.Cmd {
	return func() tea.Msg {
		queued, refusals := QueueAddresses(context.Background(), api.QueueAdd, []string{epicAddress})
		return epicQueuedMsg{queued: queued, refusals: refusals}
	}
}

// epicFile is epic's ticket.md: resolved by address under store in server
// mode, else from its directory.
func epicFile(store string, epic tickets.Epic) (string, error) {
	if project, name, ok := tickets.ParseEpicAddress(epic.Path); ok {
		return tickets.EpicTicketMDPath(filepath.Join(store, project), project, name)
	}
	if path, ok := epicTicketMDPath(epic); ok {
		return path, nil
	}
	return "", fmt.Errorf("epic %s has no ticket.md", epic.Name)
}

// cmdApplyEpicStatus is an Author write to the epic's own file, then a ping to
// a running server, like cmdApplyTicketStatus.
func cmdApplyEpicStatus(api ServerAPI, store string, epic tickets.Epic, status schema.Status) tea.Cmd {
	return func() tea.Msg {
		file, err := epicFile(store, epic)
		if err == nil {
			s := string(status)
			err = tickets.UpdateEpicFields(file, tickets.EpicUpdate{Status: &s})
		}
		if err != nil {
			return notify.Error(err.Error())()
		}
		if api != nil {
			_ = api.TicketChanged(context.Background(), epic.Path)
		}
		return statusChangedMsg{}
	}
}
