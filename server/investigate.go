package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

const (
	investigateEntry    = "investigate"
	matchedEntryHeading = "Matched Entry"
)

// matchedEntryText describes the catalog entry for the investigate ticket's
// body and prompt; an unmatched failure says so.
func matchedEntryText(e recovery.Entry, matched bool) string {
	if !matched {
		return "No catalog entry matched this failure."
	}
	return fmt.Sprintf("%s (%s %s, executor %s)", e.ID, e.Type, e.Kind, e.Executor)
}

// investigatePrompt is what the agent is told on top of the skill command.
func investigatePrompt(t tickets.Ticket) string {
	if t.Type != string(schema.TypeInvestigate) {
		return ""
	}
	raw, err := os.ReadFile(t.Path)
	if err != nil {
		return ""
	}
	return "Matched catalog entry: " + strings.TrimSpace(schema.Section(schema.ParseBody(string(raw)), matchedEntryHeading))
}

// investigateRef is where an investigate ticket's detached worktree starts: the
// feature-branch tip, else the failed ticket's resolved base. ok is false when
// neither exists, so the caller falls back to the ordinary commitless ref.
func investigateRef(repo string, addr tickets.Address, t tickets.Ticket, epics []tickets.Epic) (ref string, ok bool) {
	if _, err := git.RevParse(repo, "refs/heads/"+addr.Epic); err == nil {
		return "refs/heads/" + addr.Epic, true
	}
	if t.Parent == nil {
		return "", false
	}
	for _, e := range epics {
		if e.Name != addr.Epic {
			continue
		}
		for _, p := range e.Tickets {
			if p.Identifier == *t.Parent {
				ref, _, _ = strings.Cut(p.ResolvedBase, "@")
				return ref, ref != ""
			}
		}
	}
	return "", false
}

// investigate forks a commitless investigate ticket off the failed ticket and
// queues it at the front. The parent stays parked; it waits on the child through
// the ordinary parent edge. It returns the child's address.
func (s *Server) investigate(ref ticketRef, f recovery.Failure, entry string) (string, error) {
	path, id, err := writeInvestigateTicket(ref, f, entry)
	if err != nil {
		return "", err
	}
	child := tickets.Address{Project: ref.addr.Project, Epic: ref.addr.Epic, ID: id}
	// queueAdd checks the index, so it must see the new ticket first.
	if err := s.idx.refresh(s.cfg.TicketStore); err != nil {
		return "", fmt.Errorf("refresh after writing %s: %w", path, err)
	}
	res, err := s.queueAdd(QueueRequest{Address: child.String(), Front: true, actor: recovery.ActorRecovery})
	if err != nil {
		return "", err
	}
	if res.Refused {
		return "", fmt.Errorf("queue %s: %s", child, res.Message)
	}
	return child.String(), nil
}

func writeInvestigateTicket(ref ticketRef, f recovery.Failure, entry string) (path, id string, err error) {
	epicPath := filepath.Join(ref.projectDir, ref.addr.Epic)
	epic, unlock, err := tickets.LoadLockedEpic(epicPath)
	if err != nil {
		return "", "", fmt.Errorf("load epic %s: %w", epicPath, err)
	}
	defer unlock()

	id, err = tickets.NextTicketID(*epic, ref.addr.ID)
	if err != nil {
		return "", "", fmt.Errorf("allocate investigate id: %w", err)
	}
	parent := schema.TicketID(ref.addr.ID)
	child := schema.Ticket{ID: schema.TicketID(id), Status: schema.StatusOpen, Type: schema.TypeInvestigate, Parent: &parent}
	body := fmt.Sprintf(
		"\n# %s — Investigate %s\n\n## What to build\n\nTicket %s parked with %s (%s): %s\n\nFind out why and fix it or report what a person must do.\n\n## %s\n\n%s\n\n## Acceptance criteria\n\n- [ ] Cause found and the parked ticket recovered, or the findings reported\n",
		id, parent, parent, f.Type, f.Kind, f.Reason, matchedEntryHeading, entry,
	)
	out, err := schema.MarshalTicket(child, body)
	if err != nil {
		return "", "", fmt.Errorf("marshal investigate ticket %s: %w", id, err)
	}
	path = filepath.Join(epicPath, "issues", fmt.Sprintf("%s-investigate.md", id))
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return "", "", fmt.Errorf("write %s: %w", path, err)
	}
	if _, err := schema.ParseTicket(path); err != nil {
		return "", "", fmt.Errorf("investigate ticket %s failed validation: %w", path, err)
	}
	return path, id, nil
}
