package cmd

import (
	"fmt"
	"io"
	"time"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/repair"
	"github.com/spf13/cobra"
)

// resetResult is the --json success payload of `gx tickets reset`.
type resetResult = repair.ResetResult

// resetInput is everything runTicketsReset needs from the command line.
type resetInput = repair.ResetInput

const resetLongHelp = `Send a stuck ticket back to open instead of landing it.

Requires --reason, which is written into the ticket's Comments framed as
unverified partial work. Accepts claimed, needs-repair and needs-answer
tickets; done only with --force, which silences the already-landed refusal
and never reverts the landing. Refuses a ticket with fork children (reset a
child instead) and a ticket whose agent is still alive on its herdr tab.`

func newTicketsResetCmd(d deps) *cobra.Command {
	in := resetInput{Subjects: repair.GitCommitSubjects}
	cmd := &cobra.Command{
		Use:   "reset <epic> <id>",
		Short: "send a stuck ticket back to open, keeping its commits reachable",
		Long:  resetLongHelp,
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			cwd, err := d.getwd()
			if err != nil {
				return err
			}
			in.Cwd = cwd
			in.EpicPath = resolveEpicArg(args[0], cwd)
			in.ID = args[1]
			in.Now = time.Now()
			return runTicketsReset(in, ralphloop.DefaultDeps(), c.OutOrStdout(), c.ErrOrStderr())
		},
	}
	cmd.Flags().StringVar(&in.Reason, "reason", "", "why the ticket is being reset (required)")
	cmd.Flags().BoolVar(&in.Force, "force", false, "allow resetting a done ticket")
	cmd.Flags().BoolVar(&in.DeleteBranch, "delete-branch", false, "delete the iteration branch instead of keeping it under ralph-loop/attic/")
	cmd.Flags().BoolVar(&in.JSON, "json", false, "emit structured JSON instead of human-readable text")
	return cmd
}

func runTicketsReset(in resetInput, d ralphloop.Deps, stdout, stderr io.Writer) error {
	res, err := repair.Reset(in, d)
	text := fmt.Sprintf("%s: reset to open (attic: %s)", epicTicketLabel(in.EpicPath, in.ID), repair.AtticLabel(res.AtticRef))
	if err == nil && !in.JSON {
		fmt.Fprintln(stderr, "warning: "+repair.ResetLiveRunWarning)
	}
	return finishRecovery(stdout, stderr, in.JSON, res, text, err)
}
