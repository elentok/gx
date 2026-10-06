package cmd

import (
	"io"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/repair"
	"github.com/spf13/cobra"
)

// landResult is the --json success payload of `gx tickets land`.
type landResult = repair.LandResult

// landInput is everything runTicketsLand needs from the command line.
type landInput struct {
	EpicPath      string
	ID            string
	From, To      string
	IgnoreLiveTab bool
	Continue      bool
	Abort         bool
	JSON          bool
	Cwd           string
	Getwd         func() (string, error)
}

const landLongHelp = `Land a stuck ticket's commits onto the epic's feature branch, the way the
orchestrator would, and leave the ticket at status: done.

Accepts done, claimed and needs-repair tickets. Refuses draft, open and
needs-answer tickets, and tickets explicitly flagged commitless.

The commit range comes from the ticket's iteration branch; --from/--to land an
explicit range instead (--from is exclusive). Already-landed work is detected
and only the status is repaired. A conflict exits 0, leaves the cherry-pick in
progress and writes a land marker; nothing is written to the ticket.

After resolving a conflict, run land --continue: it refuses while the
cherry-pick is still in progress or the branch has not moved, then stamps the
commit, marks the ticket done and clears the marker and lock. land --abort
aborts the cherry-pick and clears the marker and lock; no ticket is touched.
With no marker, land --abort clears a lock a crashed land left behind (refused
while the lock's owning process is still running).

Refuses to run from a ralph-loop/* working directory. This is a guard rail,
not a boundary: changing directory evades it. It also refuses while the
iteration's herdr tab exists (--ignore-live-tab overrides, since a tab can
outlive its agent).

Metrics are recovered from the run log. Recovered elapsed time is the least
trustworthy of the three metrics. Worktree, branch and tab leftovers are not
cleaned; a later live run sweeps them.`

func newTicketsLandCmd(d deps) *cobra.Command {
	in := landInput{Getwd: d.getwd}
	cmd := &cobra.Command{
		Use:   "land <epic> <id>",
		Short: "land a stuck ticket's commits onto the feature branch and mark it done",
		Long:  landLongHelp,
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			cwd, err := d.getwd()
			if err != nil {
				return err
			}
			in.Cwd = cwd
			in.EpicPath = resolveEpicArg(args[0], cwd)
			in.ID = args[1]
			return runTicketsLand(in, ralphloop.DefaultDeps(), c.OutOrStdout(), c.ErrOrStderr())
		},
	}
	cmd.Flags().StringVar(&in.From, "from", "", "exclusive base of an explicit commit range (requires --to)")
	cmd.Flags().StringVar(&in.To, "to", "", "tip of an explicit commit range (requires --from)")
	cmd.Flags().BoolVar(&in.IgnoreLiveTab, "ignore-live-tab", false, "land even though the iteration's herdr tab exists")
	cmd.Flags().BoolVar(&in.Continue, "continue", false, "finish a conflicted land after resolving it: stamp, mark done, clear the marker and lock")
	cmd.Flags().BoolVar(&in.Abort, "abort", false, "abandon a conflicted land: abort the cherry-pick, clear the marker and lock")
	cmd.Flags().BoolVar(&in.JSON, "json", false, "emit structured JSON instead of human-readable text")
	return cmd
}

func runTicketsLand(in landInput, d ralphloop.Deps, stdout, stderr io.Writer) error {
	res, text, err := landStuckTicket(in, d)
	return finishRecovery(stdout, stderr, in.JSON, res, text, err)
}

func landStuckTicket(in landInput, d ralphloop.Deps) (landResult, string, error) {
	return repair.Land(repair.LandInput{
		EpicPath:      in.EpicPath,
		ID:            in.ID,
		From:          in.From,
		To:            in.To,
		IgnoreLiveTab: in.IgnoreLiveTab,
		Continue:      in.Continue,
		Abort:         in.Abort,
		Cwd:           in.Cwd,
		Getwd:         in.Getwd,
	}, d)
}
