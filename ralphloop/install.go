package ralphloop

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/deps"
	"github.com/elentok/gx/ralphloop/runlog"
)

// installDepsIn installs dependencies in the directory the agent will work in
// and logs it. The error is returned as is: the install step already names the
// directory. It runs outside worktreeLock so a slow install does not hold up
// another ticket's git worktree add.
func installDepsIn(d deps.Deps, o OneIteration, path string) error {
	ctx := o.Context
	if ctx == nil {
		ctx = context.Background()
	}
	if o.OnDepsStart != nil {
		o.OnDepsStart()
	}
	commands, err := d.InstallDeps(ctx, path, o.ScratchDir)
	if err != nil {
		return err
	}
	if o.OnDepsDone != nil {
		o.OnDepsDone()
	}
	_ = runlog.AppendEvent(o.ScratchDir, o.Epic, runlog.Event{
		Type:   string(events.DepsInstalled),
		Ticket: o.Ticket.Identifier,
		Agent:  o.Agent,
		Cwd:    path,
		Reason: strings.Join(commands, "; "),
	})
	return nil
}

// discardAfterInstallFailure joins a failed discard onto the install error, so a
// retry that trips over the leftover worktree can be traced back.
func discardAfterInstallFailure(installErr, discardErr error) error {
	if discardErr != nil {
		return errors.Join(installErr, fmt.Errorf("discard worktree: %w", discardErr))
	}
	return installErr
}
