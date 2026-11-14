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
	// Area is the epic the tickets leave; Map is the new map's name.
	Area string
	Map  string
	// Title is the person-approved name shown in the map's body.
	Title string
	// IDs are the approved tickets' ids in Area. Empty on a rerun reads them
	// from the map's Notes.
	IDs []string
	// Active refuses a ticket that is queued or live. The CLI passes the
	// `gx tickets move` check; the server passes its own.
	Active func(tickets.Address) error
}

const (
	notesAreaPrefix   = "Area: "
	notesTicketPrefix = "- "
)

// StartDesignNotes renders the map's Notes section body: the area, then one
// address per line. ParseStartDesignNotes reads it back.
func StartDesignNotes(project, area string, ids []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s%s\n\nTickets chosen from this area:\n", notesAreaPrefix, area)
	for _, id := range ids {
		fmt.Fprintf(&b, "%s%s\n", notesTicketPrefix, tickets.Address{Project: project, Epic: area, ID: id})
	}
	return b.String()
}

// ParseStartDesignNotes returns the area and ticket ids a map's body names in
// its Notes section. ok is false when the Notes hold no area line.
func ParseStartDesignNotes(body string) (area string, ids []string, ok bool) {
	for line := range strings.SplitSeq(schema.Section(body, "Notes"), "\n") {
		line = strings.TrimSpace(line)
		if rest, found := strings.CutPrefix(line, notesAreaPrefix); found {
			area, ok = strings.TrimSpace(rest), true
		} else if rest, found := strings.CutPrefix(line, notesTicketPrefix); found {
			if _, id, cut := strings.Cut(rest, "/"); cut {
				ids = append(ids, strings.TrimSpace(id))
			}
		}
	}
	return area, ids, ok && area != ""
}

// StartDesign creates a charting map from tickets of an area and moves them in.
// Every ticket is dry-run first, so a refusal writes nothing. A failure while
// moving leaves the moved tickets in the map; calling again with the same map
// name carries on and creates no second map.
func StartDesign(o StartDesignOpts) (string, error) {
	epics, err := tickets.Load(o.ProjectDir)
	if err != nil {
		return "", err
	}
	var area, existing *tickets.Epic
	for i := range epics {
		switch epics[i].Name {
		case o.Area:
			area = &epics[i]
		case o.Map:
			existing = &epics[i]
		}
	}
	if area == nil || !area.IsArea {
		return "", MoveRefusal(tickets.CodeUnknownEpic, "no area %q in project %q", o.Area, o.Project)
	}

	ids := o.IDs
	if existing != nil {
		notesIDs, err := resumableIDs(*existing, *area)
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
		t, found := ticketByID(*area, id)
		switch {
		case found:
			todo = append(todo, t)
		case existing == nil:
			return "", MoveRefusal(tickets.CodeUnknownTicket, "no ticket %s in area %q", id, o.Area)
		}
	}

	// PlanMove needs an existing target epic. The area stands in for a map that
	// is not created yet: the refusals do not depend on the target, and the
	// plan is dropped.
	for _, t := range todo {
		a := tickets.Address{Project: o.Project, Epic: o.Area, ID: t.DisplayNumber()}
		if err := o.Active(a); err != nil {
			return "", err
		}
		if _, err := PlanMove(o.ProjectDir, a, t, o.Area, "", MoveOpts{}); err != nil {
			return "", err
		}
	}

	if existing == nil {
		body := fmt.Sprintf("# %s\n\n## Destination\n\nA provisional Destination for the %q theme of the %q area. The Design confirms or replaces it.\n\n## Notes\n\n%s",
			o.Title, o.Title, o.Area, StartDesignNotes(o.Project, o.Area, ids))
		if _, err := tickets.CreateEpic(o.ConversationsDir, o.ProjectDir, o.Map,
			tickets.EpicFields{Kind: "map", Status: string(schema.StatusOpen), DestinationUnconfirmed: true}, body); err != nil {
			return "", err
		}
	}

	for _, t := range todo {
		if err := moveOne(o, tickets.Address{Project: o.Project, Epic: o.Area, ID: t.DisplayNumber()}); err != nil {
			return "", fmt.Errorf("moving %s:%s/%s into %s (rerun to carry on): %w", o.Project, o.Area, t.DisplayNumber(), o.Map, err)
		}
	}
	return o.Map, nil
}

// moveOne moves one ticket with both epics locked, re-reading it under the lock.
func moveOne(o StartDesignOpts, a tickets.Address) error {
	areaPath, mapPath := filepath.Join(o.ProjectDir, o.Area), filepath.Join(o.ProjectDir, o.Map)
	unlock, err := LockEpics(areaPath, mapPath)
	if err != nil {
		return err
	}
	defer unlock()
	if err := o.Active(a); err != nil {
		return err
	}
	area := tickets.LoadEpic(o.ProjectDir, o.Area)
	src, found := ticketByID(area, a.ID)
	if !found {
		return nil // already moved by an earlier run
	}
	plan, err := PlanMove(o.ProjectDir, a, src, o.Map, "", MoveOpts{})
	if err != nil {
		return err
	}
	return ApplyMove(plan)
}

// resumableIDs returns the Notes ids of an existing epic when it is this
// verb's own unfinished run for area: a charting map with no completed_at
// whose Notes name the area and list a ticket still in it.
func resumableIDs(e, area tickets.Epic) ([]string, error) {
	notesArea, ids, ok := ParseStartDesignNotes(e.Body)
	if e.IsMap && (e.DesignStep == "" || e.DesignStep == string(schema.StepChart)) && e.CompletedAt.IsZero() && ok && notesArea == area.Name {
		for _, id := range ids {
			if _, still := ticketByID(area, id); still {
				return ids, nil
			}
		}
	}
	return nil, fmt.Errorf("epic %q already exists and is not an unfinished move from area %q", e.Name, area.Name)
}

func ticketByID(e tickets.Epic, id string) (tickets.Ticket, bool) {
	for _, t := range e.Tickets {
		if t.DisplayNumber() == id {
			return t, true
		}
	}
	return tickets.Ticket{}, false
}
