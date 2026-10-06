package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/config"
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
				return runServerRepair(c.Context(), cl, c.OutOrStdout(), jsonOut, name, req, directRepair)
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

const directNotice = "server not running — ran directly under land lock"

// directRepair runs a verb in-process against the configured ticket store.
func directRepair(verb string, req server.RepairRequest) (server.RepairResult, error) {
	cfg, err := config.Load()
	if err != nil {
		return server.RepairResult{}, err
	}
	return server.RunRepairDirect(cfg.TicketStore.Path, verb, req)
}

// runServerRepair never starts the server. The repair verbs are the only
// server writes with a fallback: a dead socket runs them directly via `direct`,
// since they must work when the server is what's broken.
func runServerRepair(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool, verb string, req server.RepairRequest,
	direct func(verb string, req server.RepairRequest) (server.RepairResult, error)) error {
	if ctx == nil {
		ctx = context.Background()
	}
	res, err := cl.Repair(ctx, verb, req)
	switch {
	case apiclient.IsNotRunning(err):
		if res, err = direct(verb, req); err != nil {
			return fmt.Errorf("direct %s failed: %w", verb, err)
		}
		if !jsonOut {
			fmt.Fprintln(w, directNotice)
		}
	case err != nil:
		return fmt.Errorf("server write failed: %w", err)
	}
	if jsonOut {
		stamped, err := stampProvenance(res, viaServer, actorRecovery)
		if err != nil {
			return err
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(stamped)
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
