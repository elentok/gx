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
//
// setup is the worktree's setup list (see setupFor) and onSetupStart runs once
// before its first command; both may be empty.
func installDepsIn(d deps.Deps, o OneIteration, path string, setup []string, onSetupStart func()) error {
	ctx := o.Context
	if ctx == nil {
		ctx = context.Background()
	}
	if o.OnDepsStart != nil {
		o.OnDepsStart()
	}
	commands, installTime, setupTime, err := d.InstallDeps(ctx, path, o.ScratchDir, setup, onSetupStart)
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

		DepsStep:  installTime,
		SetupStep: setupTime,
	})
	return nil
}

// setupFor loads the setup list for the worktree at path. A nil loader means no
// list; a list that cannot be read is an error, so a broken config is not
// skipped silently.
func setupFor(d deps.Deps, o OneIteration, path string) ([]string, error) {
	if d.LoadSetup == nil {
		return nil, nil
	}
	return d.LoadSetup(path, o.ScratchDir)
}

// installWithSetupIn is installDepsIn with the worktree's setup list and the
// launch's OnSetupStart hook. A list that cannot be read fails before any install.
func installWithSetupIn(d deps.Deps, o OneIteration, path string) error {
	setup, err := setupFor(d, o, path)
	if err != nil {
		return err
	}
	return installDepsIn(d, o, path, setup, o.OnSetupStart)
}

// discardAfterInstallFailure joins a failed discard onto the install error, so a
// retry that trips over the leftover worktree can be traced back.
func discardAfterInstallFailure(installErr, discardErr error) error {
	if discardErr != nil {
		return errors.Join(installErr, fmt.Errorf("discard worktree: %w", discardErr))
	}
	return installErr
}
