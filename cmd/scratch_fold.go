package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/tickets"
)

// warnOnScratchFoldFailure runs the startup stray-`.scratch` fold (see
// tickets.FoldStrayScratchDirs) and, if it fails, writes a non-fatal warning
// to w instead of returning the error - like config.WarnOnMigrateFailure,
// callers should proceed with startup regardless of the outcome. It's a
// no-op outside a git repo, where there's nothing to fold. confirmFn is
// deps.confirmForce, threaded through so tests can fake the interactive
// prompt.
func warnOnScratchFoldFailure(w io.Writer, confirmFn func(string) (bool, error)) {
	cwd, err := os.Getwd()
	if err != nil {
		return
	}
	info, err := git.IdentifyDir(cwd)
	if err != nil {
		return
	}
	resolve := func(action tickets.FoldAction) (tickets.CollisionResolution, error) {
		return resolveScratchCollision(confirmFn, action)
	}
	if err := tickets.FoldStrayScratchDirs(info.Repo, resolve); err != nil {
		fmt.Fprintf(w, "warning: failed to fold stray .scratch directories: %v\n", err)
	}
}

// resolveScratchCollision is FoldStrayScratchDirs's real ResolveCollision:
// it prompts once per colliding epic slug via confirmFn, offering merge
// (default) or auto-rename.
func resolveScratchCollision(confirmFn func(string) (bool, error), action tickets.FoldAction) (tickets.CollisionResolution, error) {
	prompt := fmt.Sprintf(
		"Epic %q from worktree %q already exists in the canonical .scratch — merge the two together?",
		action.EpicSlug, action.WorktreeName,
	)
	merge, err := confirmFn(prompt)
	if err != nil {
		return 0, err
	}
	if merge {
		return tickets.ResolveMerge, nil
	}
	return tickets.ResolveAutoRename, nil
}
