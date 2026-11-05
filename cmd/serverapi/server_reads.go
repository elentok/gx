package serverapi

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/server/wire"
	"github.com/spf13/cobra"
)

func newServerLocksCmd(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "locks",
		Short: "list held locks with their owners",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return cmdbase.ServerRead(d.Dial(), c.Context(), jsonOut, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) ([]wire.LockInfo, error) { return cl.Reads.Locks(ctx) },
				printLocks)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	return cmd
}

func newServerIterationsCmd(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "iterations",
		Short: "list live iterations with pane, worktree, branch, base and transcript path",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return cmdbase.ServerRead(d.Dial(), c.Context(), jsonOut, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) ([]wire.IterationInfo, error) {
					return cl.Reads.Iterations(ctx)
				},
				printIterations)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	return cmd
}

func newServerQueueCmd(d Deps) *cobra.Command {
	cmd := &cobra.Command{Use: "queue", Short: "server-wide queue", Args: cobra.NoArgs}
	var jsonOut bool
	list := &cobra.Command{
		Use:   "list",
		Short: "list every ticket with the scheduler's verdict on it",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return cmdbase.ServerRead(d.Dial(), c.Context(), jsonOut, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) ([]wire.QueueEntry, error) { return cl.Queue.List(ctx) },
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
			return cmdbase.ServerRead(d.Dial(), c.Context(), jsonOut, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) ([]wire.QueueItem, error) { return cl.Queue.Items(ctx) },
				printQueueItems)
		},
	}
	items.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	cmd.AddCommand(items)

	var agent string
	add := &cobra.Command{
		Use:   "add <project:epic/NN | project:epic>",
		Short: "append a ticket, or a whole epic, to the server-wide queue",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return cmdbase.ServerQueueWrite(d.Dial(), c, jsonOut, func(ctx context.Context, cl *apiclient.Client) (wire.QueueResult, error) {
				return cl.Queue.Add(ctx, args[0], agent)
			})
		},
	}
	add.Flags().StringVar(&agent, "agent", "", `agent to run the ticket under: "claude" or "codex" (default: the Default harness)`)
	remove := &cobra.Command{
		Use:   "remove <project:epic/NN>",
		Short: "remove a ticket from the server-wide queue",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return cmdbase.ServerQueueWrite(d.Dial(), c, jsonOut, func(ctx context.Context, cl *apiclient.Client) (wire.QueueResult, error) {
				return cl.Queue.Remove(ctx, args[0])
			})
		},
	}
	var replaceAgent string
	replace := &cobra.Command{
		Use:   "replace <project> [<project:epic/NN>...]",
		Short: "replace one project's queued tickets with the listed ones",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			items := make([]wire.QueueItem, 0, len(args)-1)
			for _, a := range args[1:] {
				items = append(items, wire.QueueItem{Address: a, Agent: replaceAgent})
			}
			return cmdbase.ServerQueueWrite(d.Dial(), c, jsonOut, func(ctx context.Context, cl *apiclient.Client) (wire.QueueResult, error) {
				return cl.Queue.Replace(ctx, args[0], items)
			})
		},
	}
	replace.Flags().StringVar(&replaceAgent, "agent", "", `agent to run the tickets under: "claude" or "codex" (default: the Default harness)`)
	move := &cobra.Command{
		Use:   "move <project:epic/NN> <position>",
		Short: "move a queued ticket to a 1-based position",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			pos, err := strconv.Atoi(args[1])
			if err != nil {
				return fmt.Errorf("position %q is not a number", args[1])
			}
			return cmdbase.ServerQueueWrite(d.Dial(), c, jsonOut, func(ctx context.Context, cl *apiclient.Client) (wire.QueueResult, error) {
				return cl.Queue.Move(ctx, args[0], pos)
			})
		},
	}
	mode := func(use, short string, do func(*apiclient.Queue, context.Context) (wire.QueueResult, error)) *cobra.Command {
		return &cobra.Command{
			Use:   use,
			Short: short,
			Args:  cobra.NoArgs,
			RunE: func(c *cobra.Command, _ []string) error {
				return cmdbase.ServerQueueWrite(d.Dial(), c, jsonOut, func(ctx context.Context, cl *apiclient.Client) (wire.QueueResult, error) {
					return do(cl.Queue, ctx)
				})
			},
		}
	}
	pause := mode("pause", "finish and pause: start no queued roots, let live runs finish, stay paused until resumed (survives a restart)", (*apiclient.Queue).Pause)
	resume := mode("resume", "start queued roots again after a pause", (*apiclient.Queue).Resume)
	drain := mode("drain", "alias of pause, removed in a later release", (*apiclient.Queue).Drain)
	drain.Hidden = true
	for _, c := range []*cobra.Command{add, remove, replace, move, pause, resume, drain} {
		c.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
		cmd.AddCommand(c)
	}
	return cmd
}

func printIterations(w io.Writer, its []wire.IterationInfo) error {
	for _, it := range its {
		if _, err := fmt.Fprintf(w, "%s\tpane %s\t%s\t%s\tbase %s\t%s\n",
			it.Address, it.Pane, it.Worktree, it.Branch, it.Base, it.Transcript); err != nil {
			return err
		}
	}
	return nil
}

func printQueue(w io.Writer, entries []wire.QueueEntry) error {
	for _, e := range entries {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\n", e.Address, e.Decision, e.Reason); err != nil {
			return err
		}
	}
	return nil
}

func printQueueItems(w io.Writer, items []wire.QueueItem) error {
	for _, it := range items {
		if _, err := fmt.Fprintf(w, "%s\t%s\n", it.Address, it.Agent); err != nil {
			return err
		}
	}
	return nil
}

func newServerTicketsCmd(d Deps) *cobra.Command {
	cmd := &cobra.Command{Use: "tickets", Short: "ticket reads and writes", Args: cobra.NoArgs}
	var jsonOut bool
	history := &cobra.Command{
		Use:   "history <project:epic/NN>",
		Short: "print a ticket's events from its epic's run log",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return cmdbase.ServerRead(d.Dial(), c.Context(), jsonOut, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) (wire.History, error) {
					return cl.Reads.History(ctx, args[0])
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
			return cmdbase.ServerRead(d.Dial(), c.Context(), explainJSON, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) (wire.Explanation, error) {
					return cl.Reads.Explain(ctx, args[0])
				},
				printExplanation)
		},
	}
	explain.Flags().BoolVar(&explainJSON, "json", false, "emit structured JSON instead of human-readable text")
	cmd.AddCommand(explain)

	var detailJSON bool
	detail := &cobra.Command{
		Use:   "detail <project:epic/NN>",
		Short: "show a ticket's pause rounds, state token and applicable actions",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return cmdbase.ServerRead(d.Dial(), c.Context(), detailJSON, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) (wire.TicketDetail, error) {
					return cl.Reads.Detail(ctx, args[0])
				},
				printDetail)
		},
	}
	detail.Flags().BoolVar(&detailJSON, "json", false, "emit structured JSON instead of human-readable text")
	cmd.AddCommand(detail)

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
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			if err := cl.Tickets.Changed(ctx, args[0]); err != nil {
				return fmt.Errorf("server ping failed: %w", err)
			}
			if changedJSON {
				return cmdbase.WriteStamped(c.OutOrStdout(), wire.ChangedRequest{Address: args[0]}, cmdbase.ViaServer, cmdbase.CallerActor(os.Getwd))
			}
			return nil
		},
	}
	changed.Flags().BoolVar(&changedJSON, "json", false, "emit the acknowledged address as JSON")
	cmd.AddCommand(changed)

	for _, name := range []string{"pause", "park"} {
		var pauseJSON bool
		var pauseReason string
		pause := &cobra.Command{
			Use:    name + " <project:epic/NN>",
			Short:  "pause a ticket with a required reason",
			Args:   cobra.ExactArgs(1),
			Hidden: name == "park",
			RunE: func(c *cobra.Command, args []string) error {
				return cmdbase.ServerQueueWrite(d.Dial(), c, pauseJSON, func(ctx context.Context, cl *apiclient.Client) (wire.QueueResult, error) {
					return cl.Tickets.Pause(ctx, args[0], pauseReason)
				})
			},
		}
		pause.Flags().StringVar(&pauseReason, "reason", "", "one-line reason written into the ticket (required)")
		pause.Flags().BoolVar(&pauseJSON, "json", false, "emit the structured result (or refusal) as JSON")
		cmd.AddCommand(pause)
	}

	var unmuteJSON bool
	unmute := &cobra.Command{
		Use:   "unmute <project:epic/NN>",
		Short: "clear a ticket's mutes and reopen it if it was paused by a fault",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return cmdbase.ServerQueueWrite(d.Dial(), c, unmuteJSON, func(ctx context.Context, cl *apiclient.Client) (wire.QueueResult, error) {
				return cl.Tickets.Unmute(ctx, args[0])
			})
		},
	}
	unmute.Flags().BoolVar(&unmuteJSON, "json", false, "emit the structured result (or refusal) as JSON")
	cmd.AddCommand(unmute)

	var cancelJSON, cancelStop bool
	cancel := &cobra.Command{
		Use:   "cancel <project:epic/NN>",
		Short: "cancel a ticket and its non-terminal descendants",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return cmdbase.ServerQueueWrite(d.Dial(), c, cancelJSON, func(ctx context.Context, cl *apiclient.Client) (wire.QueueResult, error) {
				return cl.Tickets.Cancel(ctx, args[0], cancelStop)
			})
		},
	}
	cancel.Flags().BoolVar(&cancelStop, "stop", false, "stop the live pane of a claimed ticket first")
	cancel.Flags().BoolVar(&cancelJSON, "json", false, "emit the structured result (or refusal) as JSON")
	cmd.AddCommand(cancel)

	var approveJSON bool
	approve := &cobra.Command{
		Use:   "approve <project:epic/NN>",
		Short: "run a ticket's latest recovery proposal",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return cmdbase.ServerQueueWrite(d.Dial(), c, approveJSON, func(ctx context.Context, cl *apiclient.Client) (wire.QueueResult, error) {
				return cl.Tickets.Approve(ctx, args[0])
			})
		},
	}
	approve.Flags().BoolVar(&approveJSON, "json", false, "emit the structured result (or refusal) as JSON")
	cmd.AddCommand(approve)

	// relaunch is the old name of retry, kept hidden for one release.
	for _, name := range []string{"retry", "relaunch"} {
		var retryJSON, retryStop bool
		retry := &cobra.Command{
			Use:    name + " <project:epic/NN>",
			Short:  "start a fresh agent on a ticket's branch, keeping its commits",
			Args:   cobra.ExactArgs(1),
			Hidden: name == "relaunch",
			RunE: func(c *cobra.Command, args []string) error {
				return cmdbase.ServerQueueWrite(d.Dial(), c, retryJSON, func(ctx context.Context, cl *apiclient.Client) (wire.QueueResult, error) {
					return cl.Tickets.Retry(ctx, args[0], retryStop)
				})
			},
		}
		retry.Flags().BoolVar(&retryStop, "stop", false, "stop a working leftover pane of the ticket first")
		retry.Flags().BoolVar(&retryJSON, "json", false, "emit the structured result (or refusal) as JSON")
		cmd.AddCommand(retry)
	}

	var nudgeJSON bool
	nudge := &cobra.Command{
		Use:   "nudge <project:epic/NN> <text>",
		Short: "type text into the live pane of a ticket's iteration",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			return cmdbase.ServerQueueWrite(d.Dial(), c, nudgeJSON, func(ctx context.Context, cl *apiclient.Client) (wire.QueueResult, error) {
				return cl.Tickets.Nudge(ctx, args[0], args[1])
			})
		},
	}
	nudge.Flags().BoolVar(&nudgeJSON, "json", false, "emit the structured result (or refusal) as JSON")
	cmd.AddCommand(nudge)

	var recVerb string
	var recJSON bool
	recommend := &cobra.Command{
		Use:   "recommend <project:epic/NN> --verb <verb> --diagnosis <text|->",
		Short: "store an investigate agent's diagnosis and recommended verb on its paused ticket",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			diagnosis, _ := c.Flags().GetString("diagnosis")
			if diagnosis == "-" {
				b, err := io.ReadAll(c.InOrStdin())
				if err != nil {
					return fmt.Errorf("reading diagnosis: %w", err)
				}
				diagnosis = string(b)
			}
			return cmdbase.ServerQueueWrite(d.Dial(), c, recJSON, func(ctx context.Context, cl *apiclient.Client) (wire.QueueResult, error) {
				return cl.Tickets.Recommend(ctx, args[0], recVerb, diagnosis)
			})
		},
	}
	recommend.Flags().StringVar(&recVerb, "verb", "", "the recommended action (one of the ticket's detail actions)")
	recommend.Flags().String("diagnosis", "", "the diagnosis, or - to read it from stdin")
	recommend.Flags().BoolVar(&recJSON, "json", false, "emit the structured result (or refusal) as JSON")
	cmd.AddCommand(recommend)
	cmd.AddCommand(NewAnswerCmd(d, true))

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
			client, err := d.Dial()()
			if err != nil {
				return err
			}
			return runServerTicketsFollow(ctx, client, args[0], followJSON, c.OutOrStdout())
		},
	}
	follow.Flags().BoolVar(&followJSON, "json", false, "emit one JSON object per line instead of human-readable text")
	cmd.AddCommand(follow)
	addServerRepairCmds(cmd, d)
	return cmd
}

func printExplanation(w io.Writer, e wire.Explanation) error {
	line := e.Verdict
	if e.Reason != "" {
		line += ": " + e.Reason
	}
	_, err := fmt.Fprintln(w, line)
	return err
}

func printDetail(w io.Writer, d wire.TicketDetail) error {
	if _, err := fmt.Fprintf(w, "%s\t%s\t%s\n", d.Address, d.Status, d.Title); err != nil {
		return err
	}
	for _, r := range d.Rounds {
		if _, err := fmt.Fprintf(w, "round %d (%s, %s): %s\n", r.Seq, r.By, r.Outcome, r.Summary); err != nil {
			return err
		}
	}
	if d.Latest != nil {
		if _, err := fmt.Fprintf(w, "round %d (%s, %s): %s\n", d.Latest.Seq, d.Latest.By, d.Latest.Outcome, d.Latest.Summary); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w, "actions: %s\n", strings.Join(d.Actions, ", "))
	return err
}

func printLocks(w io.Writer, locks []wire.LockInfo) error {
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

func printHistory(w io.Writer, h wire.History) error {
	for _, e := range h.Events {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\n", e.Time.Format("2006-01-02T15:04:05Z07:00"), e.Type, e.Reason); err != nil {
			return err
		}
	}
	return nil
}
