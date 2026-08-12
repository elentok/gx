package ralphloop

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// forkConflictResolutionTicket forks p.Ticket into a claimed conflict-resolution
// child ticket (ADR 0016's parent/fork protocol), replacing the untracked tab a
// conflict used to launch under. The parent ticket cannot reach done while this
// child is unfinished — Epic.Blocking already walks parent edges generically,
// so no new blocking mechanism is needed (see ADR 0016).
func forkConflictResolutionTicket(p iterationParams) (childPath string, err error) {
	epicPath := filepath.Join(p.ScratchDir, p.FeatureBranch)
	epic, unlock, err := tickets.LoadLockedEpic(epicPath)
	if err != nil {
		return "", fmt.Errorf("loading epic %s to fork conflict-resolution ticket: %w", epicPath, err)
	}
	defer unlock()

	parentID := schema.TicketID(p.Ticket.Identifier)
	id, err := tickets.NextTicketID(*epic, p.Ticket.Identifier)
	if err != nil {
		return "", fmt.Errorf("allocating conflict-resolution ticket id: %w", err)
	}

	stubPath := filepath.Join(epicPath, "issues", fmt.Sprintf("%s-conflict-resolution.md", id))
	stub := schema.Ticket{
		ID:     schema.TicketID(id),
		Status: schema.StatusOpen,
		Type:   schema.TypeConflictResolution,
		Parent: &parentID,
	}
	body := fmt.Sprintf(
		"\n# %s — Conflict resolution for %s\n\n## What to build\n\nResolve the cherry-pick conflict blocking ticket %s and let the sequencer complete.\n\n## Acceptance criteria\n\n- [ ] Conflict resolved and `git cherry-pick --continue` completes cleanly\n",
		id, parentID, parentID,
	)

	out, err := schema.MarshalTicket(stub, body)
	if err != nil {
		return "", fmt.Errorf("marshaling conflict-resolution ticket %s: %w", id, err)
	}
	if err := os.MkdirAll(filepath.Dir(stubPath), 0755); err != nil {
		return "", fmt.Errorf("creating issues dir for conflict-resolution ticket %s: %w", id, err)
	}
	if err := os.WriteFile(stubPath, out, 0644); err != nil {
		return "", fmt.Errorf("writing conflict-resolution ticket %s: %w", stubPath, err)
	}
	if _, err := schema.ParseTicket(stubPath); err != nil {
		return "", fmt.Errorf("conflict-resolution ticket %s failed validation: %w", stubPath, err)
	}

	if err := Claim(stubPath); err != nil {
		return "", fmt.Errorf("claiming conflict-resolution ticket %s: %w", stubPath, err)
	}

	return stubPath, nil
}
