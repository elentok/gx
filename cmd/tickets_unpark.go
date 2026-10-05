package cmd

import (
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/spf13/cobra"
)

// unparkResult is the --json success payload of `gx tickets unpark`.
type unparkResult struct {
	Ticket string `json:"ticket"`
	Status string `json:"status"`
}

func newTicketsUnparkCmd(d deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "unpark <epic> <id>",
		Short: "reopen a needs-answer ticket and retire its Needs Answer section into Comments",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			cwd, err := d.getwd()
			if err != nil {
				return err
			}
			return runTicketsUnpark(resolveEpicArg(args[0], cwd), args[1], jsonOut, time.Now(), c.OutOrStdout(), c.ErrOrStderr())
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	return cmd
}

// runTicketsUnpark is a thin wrapper over ralphloop.UnparkTicket, the same
// write the tickets/queue tabs' suggested-actions menu performs, so both
// produce the identical ticket file.
func runTicketsUnpark(epicPath, id string, jsonMode bool, now time.Time, stdout, stderr io.Writer) error {
	path, err := unparkTarget(epicPath, id)
	if err == nil {
		err = ralphloop.UnparkTicket(path, now)
	}
	return finishRecovery(stdout, stderr, jsonMode,
		unparkResult{Ticket: id, Status: "open"},
		fmt.Sprintf("%s: unparked", id), err)
}

// unparkTarget resolves id in epicPath to a ticket path, refusing unless the
// ticket is currently parked at needs-answer.
func unparkTarget(epicPath, id string) (string, error) {
	epicPath = filepath.Clean(epicPath)
	epics, err := tickets.Load(filepath.Dir(epicPath))
	if err != nil {
		return "", fmt.Errorf("loading epics under %s: %w", filepath.Dir(epicPath), err)
	}
	for _, epic := range epics {
		if epic.Name != filepath.Base(epicPath) {
			continue
		}
		for _, t := range epic.Tickets {
			if t.DisplayNumber() != id {
				continue
			}
			if epic.RenderedStatus(t) != tickets.StatusNeedsAnswer {
				return "", &RefusalError{Reason: ReasonNotParked, Message: fmt.Sprintf("ticket %s is %v, not needs-answer", id, epic.RenderedStatus(t))}
			}
			return t.Path, nil
		}
		return "", fmt.Errorf("ticket %s not found in epic %s", id, epic.Name)
	}
	return "", fmt.Errorf("epic not found: %s", epicPath)
}
