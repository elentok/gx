package tickets

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

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
	ticketsNotDone  = "tickets not done"
	alreadyMerged   = "already merged"
	nothingToQueue  = "nothing to queue"
	actionCheckAll  = "check-all"
	actionQueueEpic = "queue-epic"
	actionEditEpic  = "edit-epic"
	actionYankEpic  = "yank-epic"
)

var epicDirectStatuses = []schema.Status{schema.StatusDraft, schema.StatusOpen}

// epicMenuInput is what epicMenuItems reads from an epic row.
type epicMenuInput struct {
	epic tickets.Epic
	// branch is the branch the epic merges into; "" when unknown.
	branch string
	// queueable is what Add epic to queue would send.
	queueable    []string
	serverReason string
	archived     bool
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
		items = append(items, pick(startSessionLabel, "", actionStartSession, contextmenu.Suggested, in.serverReason))
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
	items = append(items, pick(mergeLabel, "", actionMerge, mergeGroup, firstReason(in.serverReason, mergeWhy)))

	current := schema.Status(strings.ToLower(strings.TrimSpace(epic.Status)))
	for _, status := range epicDirectStatuses {
		items = append(items, item("Set "+string(status), "", contextmenu.Status,
			reasonIf(status == current, "already "+string(status)), epicMenuPickMsg{status: status}))
	}

	items = append(items,
		pick("Add epic to queue", "", actionQueueEpic, contextmenu.Queue,
			firstReason(in.serverReason, reasonIf(len(in.queueable) == 0, nothingToQueue))),
		pick("Check all", "", actionCheckAll, contextmenu.Queue, ""),
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
		queueable:    queueableAddresses(epic, m.vm.Queue),
		serverReason: serverReason(m.serverAPI, m.link),
		archived:     archived,
	})
	return m
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
	case actionCheckAll:
		m.setPathsChecked(eligibleEpicPaths(epic), true)
		return m, nil
	case actionQueueEpic:
		return m, cmdQueueEpic(m.serverAPI, queueableAddresses(epic, m.vm.Queue))
	case actionEditEpic:
		return m, m.cmdEditSelectedFile(terminalrun.InPlace)
	case actionYankEpic:
		return m, m.yankTicketSummary()
	}
	return m, dispatchMenuAction(m.serverAPI, m.worktreeRoot, m.settings, m.width, msg.target.result(msg.action))
}

// queueableAddresses is every open, not-yet-queued ticket of epic in ticket
// order. It goes by the frontmatter status, not the rendered one, so blocked
// tickets are sent and the server decides on them.
func queueableAddresses(epic tickets.Epic, queue []string) []string {
	var addrs []string
	for _, t := range epic.Tickets {
		open := schema.Status(strings.ToLower(strings.TrimSpace(t.Status))) == schema.StatusOpen
		if open && !slices.Contains(queue, t.Path) {
			addrs = append(addrs, t.Path)
		}
	}
	return addrs
}

// epicQueuedMsg reports a finished Add epic to queue: how many tickets the
// server took and the refusals as "address: reason".
type epicQueuedMsg struct {
	queued   int
	refusals []string
}

func (msg epicQueuedMsg) toast() tea.Cmd {
	text := fmt.Sprintf("queued %d, refused %d", msg.queued, len(msg.refusals))
	if len(msg.refusals) == 0 {
		return notify.Success(text)
	}
	return notify.Warning(text + " (" + strings.Join(msg.refusals, "; ") + ")")
}

// cmdQueueEpic sends one queue-add per address, carrying on past refusals so
// one blocked ticket doesn't keep the rest out.
func cmdQueueEpic(api ServerAPI, addrs []string) tea.Cmd {
	return func() tea.Msg {
		var out epicQueuedMsg
		for _, addr := range addrs {
			res, err := api.QueueAdd(context.Background(), addr, string(serverEnqueueDefaultAgent))
			switch {
			case err != nil:
				out.refusals = append(out.refusals, addr+": "+err.Error())
			case res.Refused:
				out.refusals = append(out.refusals, addr+": "+res.Message)
			default:
				out.queued++
			}
		}
		return out
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
