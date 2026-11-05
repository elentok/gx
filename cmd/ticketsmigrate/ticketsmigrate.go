// Package ticketsmigrate is `gx tickets migrate`: rewrite a tracker root into
// the current ticket shape, or copy it into the ticket store.
package ticketsmigrate

import (
	"github.com/spf13/cobra"

	"github.com/elentok/gx/cmd/cmdbase"
)

// Deps is the runtime context of `gx tickets migrate`.
type Deps struct {
	cmdbase.Env
}

// New builds the `migrate` subcommand of `gx tickets`.
func New(d Deps) *cobra.Command {
	var toStore, dryRun bool
	var project string
	cmd := &cobra.Command{
		Use:   "migrate <path>",
		Short: "rewrite every ticket under a tracker root into the post-refactor frontmatter shape, or (--to-store) copy it into the ticket store",
		Long: `Rewrite every ticket under a tracker root into the post-refactor shape.

The server runs the steps it has not yet applied at start-up; run this by hand
to rerun every step. It also moves each live "## Needs Answer" and "## Handoff"
section into the ticket's log as a question event (plus an answer event when
the ticket was answered by hand and is open again), and rebuilds the token
ledger from old transcripts (days the live poll already wrote are kept). A
second run changes nothing.`,
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if !toStore {
				warnUnknownApplied(c.ErrOrStderr(), args[0])
				return runTicketsMigrate(d.Dirs(), args[0], dryRun, c.OutOrStdout())
			}
			cwd, err := d.Getwd()
			if err != nil {
				return err
			}
			return runTicketsMigrateToStore(d.Dirs(), cwd, args[0], project, dryRun, c.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&toStore, "to-store", false, "copy the old tree into the ticket store (old tree is left untouched)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "with --to-store: convert and validate without writing to the store; without it: run every step on a copy, print what would change and write nothing")
	cmd.Flags().StringVar(&project, "project", "", "project name in the store (default: the repo directory name)")
	return cmd
}
