package tickets

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui/confirm"
	"github.com/elentok/gx/ui/notify"
)

// cascadeClearUpdate is a done ticket (reached via the cascade, per
// tickets.Epic.CascadeDelete) surviving deletion, with its dangling Blocked
// by: entry pointing at a deleted ticket already stripped out.
type cascadeClearUpdate struct {
	path      string
	blockedBy []string
}

// cascadeDeleteConfirmedMsg carries the delete-confirmation modal's
// acceptance (see handleQueueDeleteKey): deletePaths are the cascade's
// non-done tickets, removed outright; clearUpdates are its done tickets,
// which survive with their Blocked by: entry pointing at a deleted ticket
// cleared instead.
type cascadeDeleteConfirmedMsg struct {
	deletePaths  []string
	clearUpdates []cascadeClearUpdate
}

// handleQueueDeleteKey answers "x" on the selected ticket row: computes the
// full cascade (tickets.Epic.CascadeDelete) and opens a confirmation modal
// listing it before anything is removed.
func (m QueueModel) handleQueueDeleteKey() (tea.Model, tea.Cmd) {
	rows := m.rows()
	if m.selected < 0 || m.selected >= len(rows) {
		return m, nil
	}
	row := rows[m.selected]
	toDelete, toClear := row.epic.CascadeDelete(row.ticket)

	items := make([]string, 0, len(toDelete)+len(toClear))
	deletePaths := make([]string, len(toDelete))
	for i, t := range toDelete {
		deletePaths[i] = t.Path
		items = append(items, fmt.Sprintf("%s %s", t.DisplayNumber(), t.Title))
	}
	clearUpdates := make([]cascadeClearUpdate, len(toClear))
	for i, t := range toClear {
		clearUpdates[i] = cascadeClearUpdate{path: t.Path, blockedBy: keptBlockedBy(t, toDelete)}
		items = append(items, fmt.Sprintf("%s %s (done, kept)", t.DisplayNumber(), t.Title))
	}

	prompt := fmt.Sprintf(
		"Delete %s %s and every ticket it transitively blocks?",
		row.ticket.DisplayNumber(), row.ticket.Title,
	)
	m.confirm = m.confirm.Open(confirm.Options{
		Prompt:    prompt,
		Items:     items,
		AcceptCmd: cmdConfirmCascadeDelete(deletePaths, clearUpdates),
	})
	return m, nil
}

// keptBlockedBy filters t's Blocked by: tokens down to the ones that don't
// refer to any ticket in deleted — the entries a done ticket keeps once the
// cascade's deleted tickets are gone.
func keptBlockedBy(t tickets.Ticket, deleted []tickets.Ticket) []string {
	var kept []string
	for _, token := range t.BlockedBy {
		refersToDeleted := false
		for _, d := range deleted {
			if tickets.TokenRefersToTicket(token, d) {
				refersToDeleted = true
				break
			}
		}
		if !refersToDeleted {
			kept = append(kept, token)
		}
	}
	return kept
}

// cmdConfirmCascadeDelete returns the tea.Cmd run when the cascade-delete
// confirmation modal is accepted (see confirm.Options.AcceptCmd).
func cmdConfirmCascadeDelete(deletePaths []string, clearUpdates []cascadeClearUpdate) tea.Cmd {
	return func() tea.Msg {
		return cascadeDeleteConfirmedMsg{deletePaths: deletePaths, clearUpdates: clearUpdates}
	}
}

// handleCascadeDeleteConfirmed applies cascadeDeleteConfirmedMsg: every
// deletePaths ticket file is removed, every clearUpdates (done) ticket has
// its Blocked by: rewritten to the precomputed surviving entries, then the
// queue reloads from disk.
func (m QueueModel) handleCascadeDeleteConfirmed(msg cascadeDeleteConfirmedMsg) (tea.Model, tea.Cmd) {
	for _, path := range msg.deletePaths {
		if err := os.Remove(path); err != nil {
			return m, notify.Error("delete ticket: " + err.Error())
		}
	}
	for _, update := range msg.clearUpdates {
		err := schema.UpdateTicket(update.path, func(t *schema.Ticket) {
			t.BlockedBy = toTicketIDs(update.blockedBy)
		})
		if err != nil {
			return m, notify.Error("update ticket: " + err.Error())
		}
	}
	return m, m.cmdLoadQueue()
}

// toTicketIDs lifts plain blocked_by token strings back to schema.TicketID,
// the type schema.Ticket.BlockedBy expects (see tickets.idsToStrings, the
// loader's inverse conversion).
func toTicketIDs(tokens []string) []schema.TicketID {
	if len(tokens) == 0 {
		return nil
	}
	ids := make([]schema.TicketID, len(tokens))
	for i, token := range tokens {
		ids[i] = schema.TicketID(token)
	}
	return ids
}
