package cmd

import (
	"io"
	"os"
	"path/filepath"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// completeEpicNames lists the current epic directory names under cwd's repo,
// read live off disk, for the ensure-code-review command's shell completion.
func completeEpicNames(dirs config.Dirs, cwd string) ([]string, error) {
	root, err := ticketRoot(dirs, cwd)
	if err != nil {
		return nil, err
	}
	epics, err := tickets.Load(root)
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
// an epic path. A `project:epic` address resolves against that project, as
// `gx tickets show` does. Otherwise arg is tried as a bare epic name against
// the current repo's ticket-store root (`gx tickets root`); if that directory
// doesn't exist, arg is returned unchanged so a full path still works.
func resolveEpicArg(dirs config.Dirs, arg, cwd string) string {
	if project, epic, ok := tickets.ParseEpicAddress(arg); ok {
		if root, err := projectRoot(dirs, cwd, project); err == nil {
			return filepath.Join(root, epic)
		}
		return arg
	}
	root, err := ticketRoot(dirs, cwd)
	if err != nil {
		return arg
	}
	candidate := filepath.Join(root, arg)
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		return candidate
	}
	return arg
}

// runTicketsEnsureCodeReview checks whether epicPath already has a `type:
// code-review` ticket among its published issues. If one exists, it's a
// no-op. Otherwise it stamps out a stub ticket (next sequential ID, status
// open, type code-review, "what to review" naming the epic's spec and commit
// prefix) and
// validates it the same way `gx tickets validate` does before reporting
// success. This is the single source of truth for "does this epic have a
// code-review ticket, add one if not" per
// .scratch/gx-cleanup/issues/03-ensure-code-review.md — gx-to-tickets and the
// gx-cleanup skill call this instead of each carrying their own copy of the
// check-and-create logic.
func runTicketsEnsureCodeReview(epicPath string, w io.Writer) error {
	return ensureTypedTicket(epicPath, w, typedTicketSpec{
		ticketType: schema.TypeCodeReview,
		name:       "code-review",

		contextWindow: 30000,
		bodyArgs:      []any{epicSpecLine(epicPath)},
		bodyFormat:    "\n# %s — Code review: %s\n\n## What to review\n\n%sThe epic's commits: those whose subject starts with `%[2]s/`.\n\n## Test seams\n\nnone — review ticket, opens fix tickets as children (`parent:` pointing back here) if it finds anything.\n\n## Acceptance criteria\n\n- [ ] Full epic reviewed for correctness and cross-ticket consistency\n- [ ] Any findings opened as child fix tickets\n",
	})
}
