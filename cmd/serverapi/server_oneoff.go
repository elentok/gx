package serverapi

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
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server/wire"
	"github.com/spf13/cobra"
)

// exitServerNotRunning is the one-off submit's exit code when the server is down.
const exitServerNotRunning = 8

// exitDuplicateLive is the one-off submit's exit code when a live ticket has the
// same dedupe key.
const exitDuplicateLive = 7

func newServerOneOffCmd(d Deps) *cobra.Command {
	var req wire.OneOffRequest
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
			cl, err := d.Dial()()
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
			return runServerOneOff(ctx, d.Dirs(), cl, c.OutOrStdout(), c.ErrOrStderr(), jsonOut, wait, timeout, req)
		},
	}
	cmd.Flags().StringVar(&req.Project, "project", "", "project to create the ticket in (default: the project owning the current directory, else scratch)")
	cmd.Flags().StringVar(&req.Name, "name", "", "epic name (default: the prompt's first words plus a short unique suffix)")
	cmd.Flags().StringVar(&req.Agent, "agent", "", `agent to run the ticket under: "claude" or "codex" (default: the Default harness)`)
	cmd.Flags().StringVar(&req.Type, "type", "", `ticket type: "prompt" (default), "evolve" or "diagnose"; a one-off never commits`)
	cmd.Flags().StringArrayVar(&req.BlockedBy, "blocked-by", nil, "address of a ticket in the same project that must merge first (repeatable)")
	cmd.Flags().IntVar(&req.ExpectedContextWindow, "expected-context-window", 0, "expected context window in tokens")
	cmd.Flags().BoolVar(&req.Front, "front", false, "queue it at the head instead of the tail")
	cmd.Flags().BoolVar(&req.Notify, "notify", false, "send the ticket's Result to chat when it succeeds (a pause always notifies)")
	cmd.Flags().StringVar(&req.File, "file", "", "markdown payload (frontmatter + body); flags override its frontmatter. Read by the server, so localhost only")
	cmd.Flags().StringVar(&req.Unique, "unique", "", "dedupe key: refuse while a live ticket of the project has it (--file defaults to its resolved path)")
	cmd.Flags().BoolVar(&req.NoUnique, "no-unique", false, "do not dedupe, even with --file")
	cmd.MarkFlagsMutuallyExclusive("unique", "no-unique")
	cmd.Flags().BoolVar(&wait, "wait", false, "block until the ticket is done or paused, print its Result; a pause waits out pending recovery; exit 0 done, 3 paused for an answer, 4 paused by a fault, 5 cancelled, 6 timeout, 9 recovery escalated; a duplicate-live refusal waits on the existing ticket")
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "with --wait, stop waiting after this long and exit 6; the ticket keeps running")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of the address")
	return cmd
}

// runServerOneOff never starts the server: a dead socket is a refusal with a
// `gx server start` hint, exit code 8, and nothing spooled.
func runServerOneOff(ctx context.Context, dirs config.Dirs, cl *apiclient.Client, w, errW io.Writer, jsonOut, wait bool, timeout time.Duration, req wire.OneOffRequest) error {
	res, err := cl.Queue.OneOff(ctx, req)
	switch {
	case apiclient.IsNotRunning(err):
		res = wire.OneOffResult{Refused: true, Reason: wire.ReasonServerNotRunning, Message: "no server is running; start it with `gx server start`"}
	case err != nil:
		return fmt.Errorf("server write failed: %w", err)
	}
	d := DecideOneOff(res, wait)
	if d.Notice != "" {
		fmt.Fprintln(errW, d.Notice)
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
	} else if d.PrintAddress {
		fmt.Fprintln(w, res.Address)
	}
	if d.Err != nil || !d.Follow {
		return d.Err
	}
	cwd, _ := os.Getwd()
	return runOneOffWait(ctx, cl, archivedLookup(dirs, cwd), resultW, errW, res.Address, timeout)
}

// OneOffDecision is what a submit result means for the process, independent of
// how the result is rendered.
type OneOffDecision struct {
	Err          error  // non-nil: the command fails with it, nothing is waited on
	Notice       string // stderr line, if any
	Follow       bool   // wait on res.Address
	PrintAddress bool   // text mode prints the address on stdout
}

// DecideOneOff is shared with `gx evolve`, which submits a one-off too.
func DecideOneOff(res wire.OneOffResult, wait bool) OneOffDecision {
	refusal := fmt.Sprintf("refused (%s): %s", res.Reason, res.Message)
	switch {
	case !res.Refused:
		return OneOffDecision{Follow: wait, PrintAddress: true}
	case res.Reason == wire.ReasonServerNotRunning:
		return OneOffDecision{Err: &cmdbase.ExitError{Code: exitServerNotRunning}, Notice: refusal}
	case res.Reason == wire.ReasonDuplicateLive && wait:
		return OneOffDecision{Follow: true, PrintAddress: true, Notice: "duplicate: waiting on " + res.Address}
	case res.Reason == wire.ReasonDuplicateLive:
		return OneOffDecision{Err: &cmdbase.ExitError{Code: exitDuplicateLive}, Notice: refusal, PrintAddress: true}
	default:
		return OneOffDecision{Err: errors.New(refusal)}
	}
}
