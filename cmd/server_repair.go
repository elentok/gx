package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/server"
	"github.com/spf13/cobra"
)

// addServerRepairCmds adds the four repair verbs to `gx server tickets`. They
// send the caller's cwd and branch; the server evaluates the guards.
func addServerRepairCmds(parent *cobra.Command) {
	var req server.RepairRequest
	var jsonOut bool
	verb := func(name, use, short string, args cobra.PositionalArgs, flags func(*cobra.Command)) {
		c := &cobra.Command{
			Use:   use,
			Short: short,
			Args:  args,
			RunE: func(c *cobra.Command, args []string) error {
				req.Address = args[0]
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				req.Cwd = cwd
				// An unreadable branch (not a repo) means no guard applies.
				req.Branch, _ = git.CurrentBranch(cwd)
				cl, err := serverClient()
				if err != nil {
					return err
				}
				return runServerRepair(c.Context(), cl, c.OutOrStdout(), jsonOut, name, req)
			},
		}
		c.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
		if flags != nil {
			flags(c)
		}
		parent.AddCommand(c)
	}

	verb("land", "land <project:epic/NN>", "land a stuck ticket's commits onto the feature branch and mark it done", cobra.ExactArgs(1), func(c *cobra.Command) {
		c.Flags().StringVar(&req.From, "from", "", "exclusive base of an explicit commit range (requires --to)")
		c.Flags().StringVar(&req.To, "to", "", "tip of an explicit commit range (requires --from)")
		c.Flags().BoolVar(&req.IgnoreLiveTab, "ignore-live-tab", false, "land even though the iteration's herdr tab exists")
		c.Flags().BoolVar(&req.Continue, "continue", false, "finish a conflicted land after resolving it")
		c.Flags().BoolVar(&req.Abort, "abort", false, "abandon a conflicted land")
	})
	verb("reset", "reset <project:epic/NN>", "send a stuck ticket back to open, keeping its commits reachable", cobra.ExactArgs(1), func(c *cobra.Command) {
		c.Flags().StringVar(&req.Reason, "reason", "", "why the ticket is being reset (required)")
		c.Flags().BoolVar(&req.Force, "force", false, "allow resetting a done ticket")
		c.Flags().BoolVar(&req.DeleteBranch, "delete-branch", false, "delete the iteration branch instead of keeping it under ralph-loop/attic/")
	})
	verb("unpark", "unpark <project:epic/NN>", "reopen a needs-answer ticket and retire its Needs Answer section into Comments", cobra.ExactArgs(1), nil)
	verb("verify", "verify <project:epic[/NN]>", "report whether each ticket's commits landed on the feature branch", cobra.ExactArgs(1), nil)
}

// runServerRepair never starts the server: a dead socket is a refusal with a
// `gx server start` hint, like the queue writes.
func runServerRepair(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool, verb string, req server.RepairRequest) error {
	if ctx == nil {
		ctx = context.Background()
	}
	res, err := cl.Repair(ctx, verb, req)
	switch {
	case apiclient.IsNotRunning(err):
		res = server.RepairResult{Refused: true, Reason: server.ReasonServerNotRunning, Message: "no server is running; start it with `gx server start`"}
	case err != nil:
		return fmt.Errorf("server write failed: %w", err)
	}
	if jsonOut {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(res)
	}
	if res.Refused {
		return fmt.Errorf("refused (%s): %s", res.Reason, res.Message)
	}
	var out bytes.Buffer
	if err := json.Indent(&out, res.Data, "", "  "); err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, out.String())
	return err
}
