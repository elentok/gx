package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"

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

func newBudgetCmd(_ deps) *cobra.Command {
	cmd := &cobra.Command{Use: "budget", Short: "daily budget", Args: cobra.NoArgs}
	var jsonOut bool
	status := &cobra.Command{
		Use:   "status",
		Short: "show today's total spend and the budget limits (asks the server)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return serverRead(c.Context(), jsonOut, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) (server.BudgetStatus, error) { return cl.Budget(ctx) },
				printBudget)
		},
	}
	status.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	cmd.AddCommand(status)
	return cmd
}

func printBudget(w io.Writer, b server.BudgetStatus) error {
	_, err := fmt.Fprintf(w, "%s\t$%.2f\tsoft $%.2f\thard $%.2f\n", b.Day, b.Total, b.SoftLimit, b.HardLimit)
	return err
}

func newServerLocksCmd()*cobra.Command {
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

func newServerIterationsCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "iterations",
		Short: "list live iterations with pane, worktree, branch, base and transcript path",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return serverRead(c.Context(), jsonOut, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) ([]server.IterationInfo, error) {
					return cl.Iterations(ctx)
				},
				printIterations)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	return cmd
}

func newServerQueueCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "queue", Short: "server-wide queue", Args: cobra.NoArgs}
	var jsonOut bool
	list := &cobra.Command{
		Use:   "list",
		Short: "list every ticket with the scheduler's verdict on it",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return serverRead(c.Context(), jsonOut, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) ([]server.QueueEntry, error) { return cl.Queue(ctx) },
				printQueue)
		},
	}
	list.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	cmd.AddCommand(list)

	items := &cobra.Command{
		Use:   "items",
		Short: "list the queued tickets in order",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return serverRead(c.Context(), jsonOut, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) ([]server.QueueItem, error) { return cl.QueueItems(ctx) },
				printQueueItems)
		},
	}
	items.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	cmd.AddCommand(items)

	var agent string
	add := &cobra.Command{
		Use:   "add <project:epic/NN>",
		Short: "append a ticket to the server-wide queue",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return serverQueueWrite(c, jsonOut, func(ctx context.Context, cl *apiclient.Client) (server.QueueResult, error) {
				return cl.QueueAdd(ctx, args[0], agent)
			})
		},
	}
	add.Flags().StringVar(&agent, "agent", "", `agent to run the ticket under: "claude" or "codex" (default claude)`)
	remove := &cobra.Command{
		Use:   "remove <project:epic/NN>",
		Short: "remove a ticket from the server-wide queue",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return serverQueueWrite(c, jsonOut, func(ctx context.Context, cl *apiclient.Client) (server.QueueResult, error) {
				return cl.QueueRemove(ctx, args[0])
			})
		},
	}
	var replaceAgent string
	replace := &cobra.Command{
		Use:   "replace <project> [<project:epic/NN>...]",
		Short: "replace one project's queued tickets with the listed ones",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			items := make([]server.QueueItem, 0, len(args)-1)
			for _, a := range args[1:] {
				items = append(items, server.QueueItem{Address: a, Agent: replaceAgent})
			}
			return serverQueueWrite(c, jsonOut, func(ctx context.Context, cl *apiclient.Client) (server.QueueResult, error) {
				return cl.QueueReplace(ctx, args[0], items)
			})
		},
	}
	replace.Flags().StringVar(&replaceAgent, "agent", "", `agent to run the tickets under: "claude" or "codex" (default claude)`)
	move := &cobra.Command{
		Use:   "move <project:epic/NN> <position>",
		Short: "move a queued ticket to a 1-based position",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			pos, err := strconv.Atoi(args[1])
			if err != nil {
				return fmt.Errorf("position %q is not a number", args[1])
			}
			return serverQueueWrite(c, jsonOut, func(ctx context.Context, cl *apiclient.Client) (server.QueueResult, error) {
				return cl.QueueMove(ctx, args[0], pos)
			})
		},
	}
	mode := func(use, short string, do func(*apiclient.Client, context.Context) (server.QueueResult, error)) *cobra.Command {
		return &cobra.Command{
			Use:   use,
			Short: short,
			Args:  cobra.NoArgs,
			RunE: func(c *cobra.Command, _ []string) error {
				return serverQueueWrite(c, jsonOut, func(ctx context.Context, cl *apiclient.Client) (server.QueueResult, error) {
					return do(cl, ctx)
				})
			},
		}
	}
	pause := mode("pause", "stop starting queued roots until resumed (survives a restart)", (*apiclient.Client).QueuePause)
	resume := mode("resume", "start queued roots again after a pause or drain", (*apiclient.Client).QueueResume)
	drain := mode("drain", "stop starting queued roots and let live runs finish", (*apiclient.Client).QueueDrain)
	for _, c := range []*cobra.Command{add, remove, replace, move, pause, resume, drain} {
		c.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
		cmd.AddCommand(c)
	}
	return cmd
}

// serverQueueWrite runs one queue write. With --json the result or refusal is
// the output; otherwise a refusal is an error.
func serverQueueWrite(c *cobra.Command, jsonOut bool, do func(context.Context, *apiclient.Client) (server.QueueResult, error)) error {
	ctx := c.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	cl, err := serverClient()
	if err != nil {
		return err
	}
	return runServerQueueWrite(ctx, cl, c.OutOrStdout(), jsonOut, do)
}

// runServerQueueWrite never starts the server: a dead socket is a refusal
// with a `gx server start` hint.
func runServerQueueWrite(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool, do func(context.Context, *apiclient.Client) (server.QueueResult, error)) error {
	res, err := do(ctx, cl)
	switch {
	case apiclient.IsNotRunning(err):
		res = server.QueueResult{Refused: true, Reason: server.ReasonServerNotRunning, Message: "no server is running; start it with `gx server start`"}
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
	for i, it := range res.Queue {
		if _, err := fmt.Fprintf(w, "%d\t%s\t%s\n", i+1, it.Address, it.Agent); err != nil {
			return err
		}
	}
	return nil
}

func printIterations(w io.Writer, its []server.IterationInfo) error {
	for _, it := range its {
		if _, err := fmt.Fprintf(w, "%s\tpane %s\t%s\t%s\tbase %s\t%s\n",
			it.Address, it.Pane, it.Worktree, it.Branch, it.Base, it.Transcript); err != nil {
			return err
		}
	}
	return nil
}

func printQueue(w io.Writer, entries []server.QueueEntry) error {
	for _, e := range entries {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\n", e.Address, e.Decision, e.Reason); err != nil {
			return err
		}
	}
	return nil
}

func printQueueItems(w io.Writer, items []server.QueueItem) error {
	for _, it := range items {
		if _, err := fmt.Fprintf(w, "%s\t%s\n", it.Address, it.Agent); err != nil {
			return err
		}
	}
	return nil
}

func newServerTicketsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "tickets", Short: "ticket reads and writes", Args: cobra.NoArgs}
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

	var changedJSON bool
	changed := &cobra.Command{
		Use:   "changed <project:epic/NN>",
		Short: "tell the server a ticket file changed so the stream updates at once",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			cl, err := serverClient()
			if err != nil {
				return err
			}
			if err := cl.TicketChanged(ctx, args[0]); err != nil {
				return fmt.Errorf("server ping failed: %w", err)
			}
			if changedJSON {
				return json.NewEncoder(c.OutOrStdout()).Encode(server.ChangedRequest{Address: args[0]})
			}
			return nil
		},
	}
	changed.Flags().BoolVar(&changedJSON, "json", false, "emit the acknowledged address as JSON")
	cmd.AddCommand(changed)

	var parkJSON bool
	var parkReason string
	park := &cobra.Command{
		Use:   "park <project:epic/NN>",
		Short: "park a ticket needs-repair with a required reason",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return serverQueueWrite(c, parkJSON, func(ctx context.Context, cl *apiclient.Client) (server.QueueResult, error) {
				return cl.TicketPark(ctx, args[0], parkReason)
			})
		},
	}
	park.Flags().StringVar(&parkReason, "reason", "", "one-line reason written into the ticket (required)")
	park.Flags().BoolVar(&parkJSON, "json", false, "emit the structured result (or refusal) as JSON")
	cmd.AddCommand(park)

	var cancelJSON, cancelStop bool
	cancel := &cobra.Command{
		Use:   "cancel <project:epic/NN>",
		Short: "cancel a ticket and its non-terminal descendants",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return serverQueueWrite(c, cancelJSON, func(ctx context.Context, cl *apiclient.Client) (server.QueueResult, error) {
				return cl.TicketCancel(ctx, args[0], cancelStop)
			})
		},
	}
	cancel.Flags().BoolVar(&cancelStop, "stop", false, "stop the live pane of a claimed ticket first")
	cancel.Flags().BoolVar(&cancelJSON, "json", false, "emit the structured result (or refusal) as JSON")
	cmd.AddCommand(cancel)

	var relaunchJSON bool
	relaunch := &cobra.Command{
		Use:   "relaunch <project:epic/NN>",
		Short: "start a fresh iteration of a ticket",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return serverQueueWrite(c, relaunchJSON, func(ctx context.Context, cl *apiclient.Client) (server.QueueResult, error) {
				return cl.TicketRelaunch(ctx, args[0])
			})
		},
	}
	relaunch.Flags().BoolVar(&relaunchJSON, "json", false, "emit the structured result (or refusal) as JSON")
	cmd.AddCommand(relaunch)

	var followJSON bool
	follow := &cobra.Command{
		Use:   "follow <addr>",
		Short: "stream changes to one ticket until interrupted",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
			defer stop()
			client, err := serverClient()
			if err != nil {
				return err
			}
			return runServerTicketsFollow(ctx, client, args[0], followJSON, c.OutOrStdout())
		},
	}
	follow.Flags().BoolVar(&followJSON, "json", false, "emit one JSON object per line instead of human-readable text")
	cmd.AddCommand(follow)
	addServerRepairCmds(cmd)
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
