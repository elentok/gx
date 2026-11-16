package ticketsflow

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/server/wire"
)

// pausedEventLimit and pausedReasonLines keep the summary under 30 lines.
const (
	pausedEventLimit  = 5
	pausedReasonLines = 6
)

// pausedSummary is what `tickets paused` prints: the server's ticket detail,
// the ticket's last real run-log events and its live iteration, if any.
type pausedSummary struct {
	Detail    wire.TicketDetail
	Events    []runlog.Event
	Iteration *wire.IterationInfo
}

func newTicketsPausedCmd(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "paused <project:epic/NN>",
		Short: "summarize why a ticket paused: pause kind and reason, last run-log events, worktree and pane",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return cmdbase.ServerRead(d.Dial(), c.Context(), jsonOut, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) (pausedSummary, error) {
					return fetchPausedSummary(ctx, cl, args[0])
				},
				printPausedSummary)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	return cmd
}

func fetchPausedSummary(ctx context.Context, cl *apiclient.Client, address string) (pausedSummary, error) {
	detail, err := cl.Reads.Detail(ctx, address)
	if err != nil {
		return pausedSummary{}, err
	}
	if detail.PauseReason == "" {
		return pausedSummary{}, fmt.Errorf("%s is not paused (status %s)", detail.Address, detail.Status)
	}
	history, err := cl.Reads.History(ctx, address)
	if err != nil {
		return pausedSummary{}, err
	}
	iterations, err := cl.Reads.Iterations(ctx)
	if err != nil {
		return pausedSummary{}, err
	}

	s := pausedSummary{Detail: detail}
	for _, ev := range history.Events {
		if ev.Type != string(events.SchedulerScan) {
			s.Events = append(s.Events, ev)
		}
	}
	if len(s.Events) > pausedEventLimit {
		s.Events = s.Events[len(s.Events)-pausedEventLimit:]
	}
	for i := range iterations {
		if iterations[i].Address == detail.Address {
			s.Iteration = &iterations[i]
		}
	}
	return s, nil
}

func printPausedSummary(w io.Writer, s pausedSummary) error {
	d := s.Detail
	fmt.Fprintf(w, "%s\t%s\n", d.Address, d.Title)
	fmt.Fprintf(w, "pause: %s (%s)\n", d.PauseLabel, d.PauseReason)
	if text := strings.TrimSpace(d.PauseText); text != "" {
		lines := strings.Split(text, "\n")
		if len(lines) > pausedReasonLines {
			lines = append(lines[:pausedReasonLines], "...")
		}
		fmt.Fprintf(w, "reason:\n  %s\n", strings.Join(lines, "\n  "))
	}
	fmt.Fprintln(w, "last events:")
	if len(s.Events) == 0 {
		fmt.Fprintln(w, "  none")
	}
	for _, ev := range s.Events {
		line := fmt.Sprintf("  %s %s", ev.Time.Format("2006-01-02T15:04:05Z07:00"), ev.Type)
		if ev.Reason != "" {
			line += ": " + ev.Reason
		}
		fmt.Fprintln(w, line)
	}
	if s.Iteration == nil {
		fmt.Fprintln(w, "worktree: none (no live iteration)")
		_, err := fmt.Fprintln(w, "pane: none")
		return err
	}
	fmt.Fprintf(w, "worktree: %s (%s)\n", s.Iteration.Worktree, s.Iteration.Branch)
	pane := s.Iteration.Pane
	if pane == "" {
		pane = "none"
	}
	_, err := fmt.Fprintf(w, "pane: %s\n", pane)
	return err
}
