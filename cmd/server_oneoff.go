package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
	"github.com/spf13/cobra"
)

// exitServerNotRunning is the one-off submit's exit code when the server is down.
const exitServerNotRunning = 8

// exitDuplicateLive is the one-off submit's exit code when a live ticket has the
// same dedupe key.
const exitDuplicateLive = 7

func newServerOneOffCmd() *cobra.Command {
	var req server.OneOffRequest
	var jsonOut, wait bool
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   `one-off ["<prompt>"] [--file <path>]`,
		Short: "create a top-level ticket from a prompt and queue it",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) == 0 && req.File == "" {
				return errors.New("a one-off needs a prompt argument or --file")
			}
			cl, err := serverClient()
			if err != nil {
				return err
			}
			if len(args) == 1 {
				req.Prompt = args[0]
			}
			if req.Cwd, err = os.Getwd(); err != nil {
				return err
			}
			if req.File != "" {
				// The server reads the file, so it needs a path valid from its side.
				if req.File, err = filepath.Abs(req.File); err != nil {
					return err
				}
			}
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			return runServerOneOff(ctx, cl, c.OutOrStdout(), c.ErrOrStderr(), jsonOut, wait, timeout, req)
		},
	}
	cmd.Flags().StringVar(&req.Project, "project", "", "project to create the ticket in (default: the project owning the current directory, else scratch)")
	cmd.Flags().StringVar(&req.Name, "name", "", "epic name (default: the prompt's first words plus a short unique suffix)")
	cmd.Flags().StringVar(&req.Agent, "agent", "", `agent to run the ticket under: "claude" or "codex" (default claude)`)
	cmd.Flags().BoolVar(&req.Commits, "commits", false, "create an implement ticket that lands commits (default: a commitless prompt ticket)")
	cmd.Flags().StringVar(&req.Base, "base", "", "branch to base the work on (needs --commits)")
	cmd.Flags().StringArrayVar(&req.BlockedBy, "blocked-by", nil, "address of a ticket in the same project that must land first (repeatable)")
	cmd.Flags().IntVar(&req.ExpectedContextWindow, "expected-context-window", 0, "expected context window in tokens")
	cmd.Flags().BoolVar(&req.Front, "front", false, "queue it at the head instead of the tail")
	cmd.Flags().BoolVar(&req.Notify, "notify", false, "send the ticket's Result to chat when it succeeds (a park always notifies)")
	cmd.Flags().StringVar(&req.File, "file", "", "markdown payload (frontmatter + body); flags override its frontmatter. Read by the server, so localhost only")
	cmd.Flags().StringVar(&req.Unique, "unique", "", "dedupe key: refuse while a live ticket of the project has it (--file defaults to its resolved path)")
	cmd.Flags().BoolVar(&req.NoUnique, "no-unique", false, "do not dedupe, even with --file")
	cmd.MarkFlagsMutuallyExclusive("unique", "no-unique")
	cmd.Flags().BoolVar(&wait, "wait", false, "block until the ticket is done or parked, print its Result; exit 0 done, 3 needs-answer, 4 needs-repair, 5 cancelled, 6 timeout; a duplicate-live refusal waits on the existing ticket")
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "with --wait, stop waiting after this long and exit 6; the ticket keeps running")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of the address")
	return cmd
}

// runServerOneOff never starts the server: a dead socket is a refusal with a
// `gx server start` hint, exit code 8, and nothing spooled.
func runServerOneOff(ctx context.Context, cl *apiclient.Client, w, errW io.Writer, jsonOut, wait bool, timeout time.Duration, req server.OneOffRequest) error {
	res, err := cl.OneOff(ctx, req)
	switch {
	case apiclient.IsNotRunning(err):
		res = server.OneOffResult{Refused: true, Reason: server.ReasonServerNotRunning, Message: "no server is running; start it with `gx server start`"}
	case err != nil:
		return fmt.Errorf("server write failed: %w", err)
	}
	d := decideOneOff(res, wait)
	if d.notice != "" {
		fmt.Fprintln(errW, d.notice)
	}
	resultW := w
	if jsonOut {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(res); err != nil {
			return err
		}
		// stdout stays pure JSON, so the waited Result is not printed.
		resultW = io.Discard
	} else if d.printAddress {
		fmt.Fprintln(w, res.Address)
	}
	if d.err != nil || !d.follow {
		return d.err
	}
	return runOneOffWait(ctx, cl, resultW, errW, res.Address, timeout)
}

// oneOffDecision is what a submit result means for the process, independent of
// how the result is rendered.
type oneOffDecision struct {
	err          error  // non-nil: the command fails with it, nothing is waited on
	notice       string // stderr line, if any
	follow       bool   // wait on res.Address
	printAddress bool   // text mode prints the address on stdout
}

func decideOneOff(res server.OneOffResult, wait bool) oneOffDecision {
	refusal := fmt.Sprintf("refused (%s): %s", res.Reason, res.Message)
	switch {
	case !res.Refused:
		return oneOffDecision{follow: wait, printAddress: true}
	case res.Reason == server.ReasonServerNotRunning:
		return oneOffDecision{err: &ExitError{Code: exitServerNotRunning}, notice: refusal}
	case res.Reason == server.ReasonDuplicateLive && wait:
		return oneOffDecision{follow: true, printAddress: true, notice: "duplicate: waiting on " + res.Address}
	case res.Reason == server.ReasonDuplicateLive:
		return oneOffDecision{err: &ExitError{Code: exitDuplicateLive}, notice: refusal, printAddress: true}
	default:
		return oneOffDecision{err: errors.New(refusal)}
	}
}
