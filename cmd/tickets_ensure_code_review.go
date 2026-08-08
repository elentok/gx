package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// completeEpicNames lists the current epic directory names under cwd's repo,
// read live off disk, for the ensure-code-review command's shell completion.
func completeEpicNames(cwd string) ([]string, error) {
	repo, err := git.FindRepo(cwd)
	if err != nil {
		return nil, err
	}
	epics, err := tickets.Load(repo.ScratchRoot())
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(epics))
	for _, epic := range epics {
		names = append(names, epic.Name)
	}
	return names, nil
}

// resolveEpicArg resolves the ensure-code-review command's single argument to
// an epic path. arg is tried first as a bare epic name against the current
// repo's default scratch root (`gx tickets root`); if that directory doesn't
// exist, arg is returned unchanged so a full scratch-relative path (the
// command's original argument form) still works.
func resolveEpicArg(arg, cwd string) string {
	repo, err := git.FindRepo(cwd)
	if err != nil {
		return arg
	}
	candidate := filepath.Join(repo.ScratchRoot(), arg)
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		return candidate
	}
	return arg
}

// runTicketsEnsureCodeReview checks whether epicPath already has a `type:
// code-review` ticket among its published issues. If one exists, it's a
// no-op. Otherwise it stamps out a stub ticket (next sequential ID, status
// ready-for-agent, type code-review, empty "what to review" body) and
// validates it the same way `gx tickets validate` does before reporting
// success. This is the single source of truth for "does this epic have a
// code-review ticket, add one if not" per
// .scratch/gx-cleanup/issues/03-ensure-code-review.md — gx-to-tickets and the
// gx-cleanup skill call this instead of each carrying their own copy of the
// check-and-create logic.
func runTicketsEnsureCodeReview(epicPath string, w io.Writer) error {
	epicPath = filepath.Clean(epicPath)

	epic, unlock, err := tickets.LoadLockedEpic(epicPath)
	if err != nil {
		return err
	}
	defer unlock()

	for _, t := range epic.Tickets {
		if t.Type == string(schema.TypeCodeReview) {
			fmt.Fprintf(w, "%s: already has a code-review ticket (%s)\n", epicPath, t.DisplayNumber())
			return nil
		}
	}

	id, err := tickets.NextTicketID(*epic, "")
	if err != nil {
		return err
	}
	stubPath := filepath.Join(epicPath, "issues", fmt.Sprintf("%s-code-review.md", id))

	stub := schema.Ticket{
		ID:                    schema.TicketID(id),
		Status:                schema.StatusReadyForAgent,
		Type:                  schema.TypeCodeReview,
		ExpectedContextWindow: 30000,
	}
	body := fmt.Sprintf(
		"\n# %s — Code review: %s\n\n## What to review\n\n\n## Test seams\n\nnone — review ticket, opens fix tickets as `children` if it finds anything.\n\n## Acceptance criteria\n\n- [ ] Full epic reviewed for correctness and cross-ticket consistency\n- [ ] Any findings opened as child fix tickets\n",
		id, epic.Name,
	)

	out, err := schema.MarshalTicket(stub, body)
	if err != nil {
		return fmt.Errorf("marshaling stub ticket: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(stubPath), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(stubPath, out, 0644); err != nil {
		return fmt.Errorf("writing stub ticket %s: %w", stubPath, err)
	}

	if _, err := schema.ParseTicket(stubPath); err != nil {
		return fmt.Errorf("stub ticket %s failed validation: %w", stubPath, err)
	}

	fmt.Fprintf(w, "%s: created code-review stub\n", stubPath)
	return nil
}
