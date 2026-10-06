package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
	"github.com/spf13/cobra"
)

// serverRead fetches one read payload from the server and prints it: as
// indented JSON, or through text when jsonOut is false.
func serverRead[T any](ctx context.Context, jsonOut bool, w io.Writer, fetch func(context.Context, *apiclient.Client) (T, error), text func(io.Writer, T) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	c, err := serverClient()
	if err != nil {
		return err
	}
	v, err := fetch(ctx, c)
	if err != nil {
		return fmt.Errorf("server read failed (is `gx server` running?): %w", err)
	}
	if jsonOut {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	}
	return text(w, v)
}

func newProjectCmd(_ deps) *cobra.Command {
	cmd := &cobra.Command{Use: "project", Short: "ticket-store projects", Args: cobra.NoArgs}
	var jsonOut bool
	list := &cobra.Command{
		Use:   "list",
		Short: "list projects with their ticket counts by status (asks the server)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return serverRead(c.Context(), jsonOut, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) ([]server.ProjectInfo, error) { return cl.Projects(ctx) },
				printProjects)
		},
	}
	list.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	cmd.AddCommand(list)
	return cmd
}

func newServerLocksCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "locks",
		Short: "list held locks with their owners",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return serverRead(c.Context(), jsonOut, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) ([]server.LockInfo, error) { return cl.Locks(ctx) },
				printLocks)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	return cmd
}

func newServerTicketsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "tickets", Short: "ticket reads", Args: cobra.NoArgs}
	var jsonOut bool
	history := &cobra.Command{
		Use:   "history <project:epic/NN>",
		Short: "print a ticket's events from its epic's run log",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return serverRead(c.Context(), jsonOut, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) (server.History, error) {
					return cl.History(ctx, args[0])
				},
				printHistory)
		},
	}
	history.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	cmd.AddCommand(history)

	var explainJSON bool
	explain := &cobra.Command{
		Use:   "explain <project:epic/NN>",
		Short: "explain why the scheduler would or wouldn't pick a ticket",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return serverRead(c.Context(), explainJSON, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) (server.Explanation, error) {
					return cl.Explain(ctx, args[0])
				},
				printExplanation)
		},
	}
	explain.Flags().BoolVar(&explainJSON, "json", false, "emit structured JSON instead of human-readable text")
	cmd.AddCommand(explain)
	return cmd
}

func printExplanation(w io.Writer, e server.Explanation) error {
	line := e.Verdict
	if e.Reason != "" {
		line += ": " + e.Reason
	}
	_, err := fmt.Fprintln(w, line)
	return err
}

func printProjects(w io.Writer, projects []server.ProjectInfo) error {
	for _, p := range projects {
		counts := make([]string, 0, len(p.Tickets))
		for status, n := range p.Tickets {
			counts = append(counts, fmt.Sprintf("%s=%d", status, n))
		}
		sort.Strings(counts)
		if _, err := fmt.Fprintf(w, "%s\t%s\n", p.Name, strings.Join(counts, " ")); err != nil {
			return err
		}
	}
	return nil
}

func printLocks(w io.Writer, locks []server.LockInfo) error {
	for _, l := range locks {
		state := "alive"
		if !l.Alive {
			state = "stale"
		}
		if _, err := fmt.Fprintf(w, "%s\t%s\tpid %d (%s)\tsince %s\t%s\n",
			l.Kind, l.Epic, l.Pid, state, l.Since.Format("2006-01-02T15:04:05Z07:00"), l.Ticket); err != nil {
			return err
		}
	}
	return nil
}

func printHistory(w io.Writer, h server.History) error {
	for _, e := range h.Events {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\n", e.Time.Format("2006-01-02T15:04:05Z07:00"), e.Type, e.Reason); err != nil {
			return err
		}
	}
	return nil
}
