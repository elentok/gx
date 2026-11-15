package ticketmove

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// StartDesignOpts is one run of StartDesign.
type StartDesignOpts struct {
	// ConversationsDir is the conversations dir tickets.CreateEpic stamps through.
	ConversationsDir string
	// ProjectDir is the project's ticket directory; Project is its name.
	ProjectDir string
	Project    string
	// Backlog is the epic the tickets leave; Map is the new map's name.
	Backlog string
	Map  string
	// Title is the person-approved name shown in the map's body.
	Title string
	// IDs are the approved tickets' ids in Backlog. Empty on a rerun reads them
	// from the map's Notes.
	IDs []string
	// Active refuses a ticket that is queued or live. The CLI passes the
	// `gx tickets move` check; the server passes its own.
	Active func(tickets.Address) error
}

const (
	notesBacklogPrefix   = "Backlog: "
	notesTicketPrefix = "- "
)

// StartDesignNotes renders the map's Notes section body: the backlog, then one
// address per line. ParseStartDesignNotes reads it back.
func StartDesignNotes(project, backlog string, ids []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s%s\n\nTickets chosen from this backlog:\n", notesBacklogPrefix, backlog)
	for _, id := range ids {
		fmt.Fprintf(&b, "%s%s\n", notesTicketPrefix, tickets.Address{Project: project, Epic: backlog, ID: id})
	}
	return b.String()
}

// ParseStartDesignNotes returns the backlog and ticket ids a map's body names in
// its Notes section. ok is false when the Notes hold no backlog line.
func ParseStartDesignNotes(body string) (backlog string, ids []string, ok bool) {
	for line := range strings.SplitSeq(schema.Section(body, "Notes"), "\n") {
		line = strings.TrimSpace(line)
		if rest, found := strings.CutPrefix(line, notesBacklogPrefix); found {
			backlog, ok = strings.TrimSpace(rest), true
		} else if rest, found := strings.CutPrefix(line, notesTicketPrefix); found {
			if _, id, cut := strings.Cut(rest, "/"); cut {
				ids = append(ids, strings.TrimSpace(id))
			}
		}
	}
	return backlog, ids, ok && backlog != ""
}

// StartDesign creates a charting map from tickets of a backlog and moves them in.
// Every ticket is dry-run first, so a refusal writes nothing. A failure while
// moving leaves the moved tickets in the map; calling again with the same map
// name carries on and creates no second map.
func StartDesign(o StartDesignOpts) (string, error) {
	epics, err := tickets.Load(o.ProjectDir)
	if err != nil {
		return "", err
	}
	var backlog, existing *tickets.Epic
	for i := range epics {
		switch epics[i].Name {
		case o.Backlog:
			backlog = &epics[i]
		case o.Map:
			existing = &epics[i]
		}
	}
	if backlog == nil || !backlog.IsBacklog {
		return "", MoveRefusal(tickets.CodeUnknownEpic, "no backlog %q in project %q", o.Backlog, o.Project)
	}

	ids := o.IDs
	if existing != nil {
		notesIDs, err := resumableIDs(*existing, *backlog)
		if err != nil {
			return "", err
		}
		if len(ids) == 0 {
			ids = notesIDs
		}
	}
	if len(ids) == 0 {
		return "", fmt.Errorf("no tickets to move into %q", o.Map)
	}

	var todo []tickets.Ticket
	for _, id := range ids {
		t, found := ticketByID(*backlog, id)
		switch {
		case found:
			todo = append(todo, t)
		case existing == nil:
			return "", MoveRefusal(tickets.CodeUnknownTicket, "no ticket %s in backlog %q", id, o.Backlog)
		}
	}

	// PlanMove needs an existing target epic. The backlog stands in for a map that
	// is not created yet: the refusals do not depend on the target, and the
	// plan is dropped.
	for _, t := range todo {
		a := tickets.Address{Project: o.Project, Epic: o.Backlog, ID: t.DisplayNumber()}
		if err := o.Active(a); err != nil {
			return "", err
		}
		if _, err := PlanMove(o.ProjectDir, a, t, o.Backlog, "", MoveOpts{}); err != nil {
			return "", err
		}
	}

	if existing == nil {
		body := fmt.Sprintf("# %s\n\n## Destination\n\nA provisional Destination for the %q theme of the %q backlog. The Design confirms or replaces it.\n\n## Notes\n\n%s",
			o.Title, o.Title, o.Backlog, StartDesignNotes(o.Project, o.Backlog, ids))
		if _, err := tickets.CreateEpic(o.ConversationsDir, o.ProjectDir, o.Map,
			tickets.EpicFields{Kind: "map", Status: string(schema.StatusOpen), DestinationUnconfirmed: true}, body); err != nil {
			return "", err
		}
	}

	for _, t := range todo {
		if err := moveOne(o, tickets.Address{Project: o.Project, Epic: o.Backlog, ID: t.DisplayNumber()}); err != nil {
			return "", fmt.Errorf("moving %s:%s/%s into %s (rerun to carry on): %w", o.Project, o.Backlog, t.DisplayNumber(), o.Map, err)
		}
	}
	return o.Map, nil
}

// moveOne moves one ticket with both epics locked, re-reading it under the lock.
func moveOne(o StartDesignOpts, a tickets.Address) error {
	backlogPath, mapPath := filepath.Join(o.ProjectDir, o.Backlog), filepath.Join(o.ProjectDir, o.Map)
	unlock, err := LockEpics(backlogPath, mapPath)
	if err != nil {
		return err
	}
	defer unlock()
	if err := o.Active(a); err != nil {
		return err
	}
	backlog := tickets.LoadEpic(o.ProjectDir, o.Backlog)
	src, found := ticketByID(backlog, a.ID)
	if !found {
		return nil // already moved by an earlier run
	}
	plan, err := PlanMove(o.ProjectDir, a, src, o.Map, "", MoveOpts{})
	if err != nil {
		return err
	}
	return ApplyMove(plan)
}

// UnfinishedRunIDs is the one rule for a partial Start Design run: e is a
// charting map with no completed_at whose Notes name backlog. It returns the
// Notes ids.
func UnfinishedRunIDs(e tickets.Epic, backlog string) ([]string, bool) {
	notesBacklog, ids, ok := ParseStartDesignNotes(e.Body)
	if !ok || notesBacklog != backlog || !e.IsCharting() || !e.CompletedAt.IsZero() {
		return nil, false
	}
	return ids, true
}

// resumableIDs returns the Notes ids of an existing epic when it is this
// verb's own unfinished run for backlog: a charting map with no completed_at
// whose Notes name the backlog and list a ticket still in it.
func resumableIDs(e, backlog tickets.Epic) ([]string, error) {
	if ids, ok := UnfinishedRunIDs(e, backlog.Name); ok {
		for _, id := range ids {
			if _, still := ticketByID(backlog, id); still {
				return ids, nil
			}
		}
	}
	return nil, fmt.Errorf("epic %q already exists and is not an unfinished move from backlog %q", e.Name, backlog.Name)
}

func ticketByID(e tickets.Epic, id string) (tickets.Ticket, bool) {
	for _, t := range e.Tickets {
		if t.DisplayNumber() == id {
			return t, true
		}
	}
	return tickets.Ticket{}, false
}
