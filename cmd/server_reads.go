package cmd

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/spf13/cobra"
)

func newProjectCmd(d deps) *cobra.Command {
	cmd := &cobra.Command{Use: "project", Short: "ticket-store projects", Args: cobra.NoArgs}
	var jsonOut bool
	list := &cobra.Command{
		Use:   "list",
		Short: "list projects with their ticket counts by status (asks the server)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return cmdbase.ServerRead(d.dial(), c.Context(), jsonOut, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) ([]wire.ProjectInfo, error) {
					return cl.Reads.Projects(ctx)
				},
				printProjects)
		},
	}
	list.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	cmd.AddCommand(list)

	var worktreesJSON bool
	worktrees := &cobra.Command{
		Use:   "worktrees",
		Short: "list each project's repo root and worktrees (asks the server)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return cmdbase.ServerRead(d.dial(), c.Context(), worktreesJSON, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) ([]wire.ProjectWorktrees, error) {
					return cl.Reads.ProjectWorktrees(ctx)
				},
				printProjectWorktrees)
		},
	}
	worktrees.Flags().BoolVar(&worktreesJSON, "json", false, "emit structured JSON instead of human-readable text")
	cmd.AddCommand(worktrees)

	var addJSON bool
	var name, vcs string
	add := &cobra.Command{
		Use:   "add [path]",
		Short: "register a project (path defaults to the current directory; asks the server)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			path := "."
			if len(args) == 1 {
				path = args[0]
			}
			abs, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			cl, err := d.dial()()
			if err != nil {
				return err
			}
			return runProjectAdd(c.Context(), cl, d, c.OutOrStdout(), addJSON,
				wire.AddProjectRequest{Path: abs, Name: name, VCS: vcs})
		},
	}
	add.Flags().StringVar(&name, "name", "", "project name (default: the repo directory's name)")
	add.Flags().StringVar(&vcs, "vcs", "", `"none" registers a directory that is not a git repo`)
	add.Flags().BoolVar(&addJSON, "json", false, "emit structured JSON instead of human-readable text")
	cmd.AddCommand(add)
	cmd.AddCommand(newProjectRetryMigrationCmd(d))

	var rmJSON bool
	remove := &cobra.Command{
		Use:   "remove <name>",
		Short: "unregister a project (its tickets stay in the store; refused while any are queued or running)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cl, err := d.dial()()
			if err != nil {
				return err
			}
			return runProjectEdit(c.Context(), c.OutOrStdout(), rmJSON, "removed", cl.Reads.RemoveProject,
				wire.ProjectRequest{Name: args[0]})
		},
	}
	remove.Flags().BoolVar(&rmJSON, "json", false, "emit structured JSON instead of human-readable text")
	cmd.AddCommand(remove)

	var setJSON bool
	setPath := &cobra.Command{
		Use:   "set-path <name> [path]",
		Short: "point a project at another repo (path defaults to the current directory)",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(c *cobra.Command, args []string) error {
			path := "."
			if len(args) == 2 {
				path = args[1]
			}
			abs, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			cl, err := d.dial()()
			if err != nil {
				return err
			}
			return runProjectEdit(c.Context(), c.OutOrStdout(), setJSON, "updated", cl.Reads.SetProjectPath,
				wire.ProjectRequest{Name: args[0], Path: abs})
		},
	}
	setPath.Flags().BoolVar(&setJSON, "json", false, "emit structured JSON instead of human-readable text")
	cmd.AddCommand(setPath, newProjectEditConfigCmd(d), newProjectOnboardCmd(d))
	return cmd
}

type projectEditCall func(context.Context, wire.ProjectRequest) (wire.ProjectResult, error)

func runProjectEdit(ctx context.Context, w io.Writer, jsonOut bool, verb string, call projectEditCall, req wire.ProjectRequest) error {
	if ctx == nil {
		ctx = context.Background()
	}
	res, err := call(ctx, req)
	switch {
	case apiclient.IsNotRunning(err):
		res = wire.ProjectResult{Refused: true, Reason: wire.ReasonServerNotRunning, Message: "no server is running; start it with `gx server start`"}
	case err != nil:
		return fmt.Errorf("server write failed: %w", err)
	}
	if jsonOut {
		return cmdbase.EncodeProvenance(w, res, cmdbase.ViaServer, cmdbase.CallerActor(os.Getwd))
	}
	if res.Refused {
		return fmt.Errorf("refused (%s): %s", res.Reason, res.Message)
	}
	_, err = fmt.Fprintf(w, "%s project %s\n", verb, res.Name)
	return err
}

// addProjectJSON is the `project add --json` object: the add result, plus the
// unset check kinds and the command that sets them up when config is missing.
type addProjectJSON struct {
	wire.AddProjectResult
	ChecksMissing  []string `json:"checks_missing,omitempty"`
	OnboardCommand string   `json:"onboard_command,omitempty"`
}

func runProjectAdd(ctx context.Context, cl *apiclient.Client, d deps, w io.Writer, jsonOut bool, req wire.AddProjectRequest) error {
	if ctx == nil {
		ctx = context.Background()
	}
	res, err := cl.Reads.AddProject(ctx, req)
	switch {
	case apiclient.IsNotRunning(err):
		res = wire.AddProjectResult{Refused: true, Reason: wire.ReasonServerNotRunning, Message: "no server is running; start it with `gx server start`"}
	case err != nil:
		return fmt.Errorf("server write failed: %w", err)
	}
	if jsonOut {
		out := addProjectJSON{AddProjectResult: res}
		if !res.Refused {
			if pc, err := missingChecks(ctx, cl, res.Name); err != nil {
				return err
			} else if pc != nil {
				out.ChecksMissing, out.OnboardCommand = missingKinds(*pc), onboardCommand(res.Name)
			}
		}
		return cmdbase.EncodeProvenance(w, out, cmdbase.ViaServer, cmdbase.CallerActor(os.Getwd))
	}
	if res.Refused {
		return fmt.Errorf("refused (%s): %s", res.Reason, res.Message)
	}
	verb := "added"
	if res.Existing {
		verb = "already registered as"
	}
	if _, err = fmt.Fprintf(w, "%s project %s\n", verb, res.Name); err != nil {
		return err
	}
	_, err = onboardChecks(ctx, cl, d, w, res.Name)
	return err
}

func newBudgetCmd(d deps) *cobra.Command {
	cmd := &cobra.Command{Use: "budget", Short: "daily budget", Args: cobra.NoArgs}
	var jsonOut bool
	status := &cobra.Command{
		Use:   "status",
		Short: "show today's total spend and the budget limits (asks the server)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return cmdbase.ServerRead(d.dial(), c.Context(), jsonOut, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) (wire.BudgetStatus, error) {
					return cl.Reads.Budget(ctx)
				},
				printBudget)
		},
	}
	status.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	cmd.AddCommand(status)

	var overrideJSON bool
	override := &cobra.Command{
		Use:   "override",
		Short: "lift the budget latches so new work can start (asks the server)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return runBudgetOverride(d.dial(), c, overrideJSON)
		},
	}
	override.Flags().BoolVar(&overrideJSON, "json", false, "emit the structured result (or refusal) as JSON")
	cmd.AddCommand(override)
	cmd.AddCommand(newBudgetIncreaseCmd(d))
	return cmd
}

func newUsageCmd(d deps) *cobra.Command {
	cmd := &cobra.Command{Use: "usage", Short: "usage gate", Args: cobra.NoArgs}
	var jsonOut bool
	override := &cobra.Command{
		Use:       "override [claude|codex]",
		Short:     "let new claims of a harness start until its gating usage window resets (asks the server; default harness when none named)",
		Args:      cobra.MatchAll(cobra.MaximumNArgs(1), cobra.OnlyValidArgs),
		ValidArgs: []string{"claude", "codex"},
		RunE: func(c *cobra.Command, args []string) error {
			harness := ""
			if len(args) > 0 {
				harness = args[0]
			}
			return runUsageOverride(d.dial(), c, harness, jsonOut)
		},
	}
	override.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	cmd.AddCommand(override)

	var summaryJSON bool
	summary := &cobra.Command{
		Use:   "summary",
		Short: "show the usage mode, the Claude limit windows and the spend totals (asks the server)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return cmdbase.ServerRead(d.dial(), c.Context(), summaryJSON, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) (wire.UsageSummary, error) {
					return cl.Reads.UsageSummary(ctx)
				},
				printUsageSummary)
		},
	}
	summary.Flags().BoolVar(&summaryJSON, "json", false, "emit structured JSON instead of human-readable text")
	cmd.AddCommand(summary)
	return cmd
}

func printUsageSummary(w io.Writer, u wire.UsageSummary) error {
	fmt.Fprintf(w, "mode: %s\n", u.Mode)
	for _, row := range []struct {
		name string
		win  *wire.UsageLimitWindow
	}{{"five_hour", u.Limits.FiveHour}, {"seven_day", u.Limits.SevenDay}} {
		if row.win != nil {
			fmt.Fprintf(w, "%s: %.0f%% used, resets at %d\n", row.name, row.win.UsedPercentage, row.win.ResetsAt)
		}
	}
	if u.Codex != nil {
		for _, row := range []struct {
			name string
			win  *wire.UsageLimitWindow
		}{{"codex_five_hour", u.Codex.FiveHour}, {"codex_seven_day", u.Codex.SevenDay}} {
			if row.win != nil {
				fmt.Fprintf(w, "%s: %.0f%% used, resets at %d\n", row.name, row.win.UsedPercentage, row.win.ResetsAt)
			}
		}
	}
	s := u.Spend
	since := func(short bool) string {
		if short {
			return " (since " + s.FirstDay + ")"
		}
		return ""
	}
	fmt.Fprintf(w, "today: $%.2f\nsince sunday: $%.2f%s\nsince the 1st: $%.2f%s\n",
		s.Today, s.SinceSunday, since(s.SinceSundayShort), s.SinceFirst, since(s.SinceFirstShort))
	return nil
}

// runUsageOverride mirrors runBudgetWrite.
func runUsageOverride(dial func() (*apiclient.Client, error), c *cobra.Command, harness string, jsonOut bool) error {
	ctx := c.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	cl, err := dial()
	if err != nil {
		return err
	}
	res, err := cl.Admin.UsageOverride(ctx, harness)
	switch {
	case apiclient.IsNotRunning(err):
		res = wire.UsageOverrideResult{Refused: true, Reason: wire.ReasonServerNotRunning, Message: "no server is running; start it with `gx server start`"}
	case err != nil:
		return fmt.Errorf("server write failed: %w", err)
	}
	w := c.OutOrStdout()
	if jsonOut {
		return cmdbase.EncodeProvenance(w, res, cmdbase.ViaServer, cmdbase.CallerActor(os.Getwd))
	}
	if res.Refused {
		return fmt.Errorf("refused (%s): %s", res.Reason, res.Message)
	}
	_, err = fmt.Fprintf(w, "usage gate lifted for the %s window until %s\n", strings.Join(res.Windows, " and "), res.Until.UTC().Format(time.RFC3339))
	return err
}

func newBudgetIncreaseCmd(d deps) *cobra.Command {
	var soft, hard float64
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "increase --soft|--hard <dollars>",
		Short: "raise a daily limit on the running server until it restarts (edit config.json to keep it)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			var req wire.BudgetIncreaseRequest
			if c.Flags().Changed("soft") {
				req.Soft = &soft
			}
			if c.Flags().Changed("hard") {
				req.Hard = &hard
			}
			if req.Soft == nil && req.Hard == nil {
				return fmt.Errorf("pass --soft or --hard")
			}
			return runBudgetWrite(d.dial(), c, jsonOut, func(ctx context.Context, cl *apiclient.Client) (wire.BudgetResult, error) {
				return cl.Admin.BudgetIncrease(ctx, req)
			})
		},
	}
	cmd.Flags().Float64Var(&soft, "soft", 0, "new soft limit in dollars (pauses new starts)")
	cmd.Flags().Float64Var(&hard, "hard", 0, "new hard limit in dollars (stops live agents)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return cmd
}

// runBudgetOverride mirrors cmdbase.RunServerQueueWrite.
func runBudgetOverride(dial func() (*apiclient.Client, error), c *cobra.Command, jsonOut bool) error {
	return runBudgetWrite(dial, c, jsonOut, func(ctx context.Context, cl *apiclient.Client) (wire.BudgetResult, error) {
		return cl.Admin.BudgetOverride(ctx)
	})
}

// runBudgetWrite runs one budget write: with --json the result or refusal is
// the output; otherwise a refusal is an error.
func runBudgetWrite(dial func() (*apiclient.Client, error), c *cobra.Command, jsonOut bool, write func(context.Context, *apiclient.Client) (wire.BudgetResult, error)) error {
	ctx := c.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	cl, err := dial()
	if err != nil {
		return err
	}
	res, err := write(ctx, cl)
	switch {
	case apiclient.IsNotRunning(err):
		res = wire.BudgetResult{Refused: true, Reason: wire.ReasonServerNotRunning, Message: "no server is running; start it with `gx server start`"}
	case err != nil:
		return fmt.Errorf("server write failed: %w", err)
	}
	w := c.OutOrStdout()
	if jsonOut {
		return cmdbase.EncodeProvenance(w, res, cmdbase.ViaServer, cmdbase.CallerActor(os.Getwd))
	}
	if res.Refused {
		return fmt.Errorf("refused (%s): %s", res.Reason, res.Message)
	}
	return printBudget(w, *res.Budget)
}

func printBudget(w io.Writer, b wire.BudgetStatus) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "budget %s: %s\n", b.Day, budgetState(b))
	fmt.Fprintf(tw, "  spent\t%s\n", tickets.FormatCost(b.Total))
	if b.Override > 0 {
		fmt.Fprintf(tw, "  overridden at\t%s\t%s counts toward the limits\n", tickets.FormatCost(b.Override), tickets.FormatCost(b.Total-b.Override))
	}
	fmt.Fprintf(tw, "  soft limit\t%s\n", budgetLimit(b.SoftLimit, b.Total-b.Override, "pauses new starts"))
	fmt.Fprintf(tw, "  hard limit\t%s\n", budgetLimit(b.HardLimit, b.Total-b.Override, "stops live agents"))
	if len(b.Projects) > 0 {
		fmt.Fprintln(tw, "\n  by project")
		tracked := 0.0
		for _, project := range slices.Sorted(maps.Keys(b.Projects)) {
			fmt.Fprintf(tw, "    %s\t%s\n", project, tickets.FormatCost(b.Projects[project]))
			tracked += b.Projects[project]
		}
		if untracked := b.Total - tracked; untracked >= 0.005 {
			fmt.Fprintf(tw, "    (untracked)\t%s\n", tickets.FormatCost(untracked))
		}
	}
	return tw.Flush()
}

// budgetLimit renders one limit and what is left of it; counted is the spend
// since the override, the only spend a limit counts.
func budgetLimit(limit, counted float64, does string) string {
	if limit <= 0 {
		return "off"
	}
	left := max(limit-counted, 0)
	return fmt.Sprintf("%s\t%s left\t(%s)", tickets.FormatCost(limit), tickets.FormatCost(left), does)
}

// budgetLine is the one-line budget summary `gx server status` prints.
func budgetLine(b wire.BudgetStatus) string {
	line := fmt.Sprintf("%s spent today, %s", tickets.FormatCost(b.Total), budgetState(b))
	if b.Override > 0 {
		line += fmt.Sprintf(", overridden at %s", tickets.FormatCost(b.Override))
	}
	return line
}

// budgetState is the budget's effect on the queue right now.
func budgetState(b wire.BudgetStatus) string {
	switch {
	case b.HardLatched:
		return "STOPPED: hard limit reached (gx budget override)"
	case b.BudgetPaused:
		return "PAUSED: soft limit reached (gx budget override)"
	}
	return "ok"
}

func newServerLocksCmd(d deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "locks",
		Short: "list held locks with their owners",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return cmdbase.ServerRead(d.dial(), c.Context(), jsonOut, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) ([]wire.LockInfo, error) { return cl.Reads.Locks(ctx) },
				printLocks)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	return cmd
}

func newServerIterationsCmd(d deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "iterations",
		Short: "list live iterations with pane, worktree, branch, base and transcript path",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return cmdbase.ServerRead(d.dial(), c.Context(), jsonOut, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) ([]wire.IterationInfo, error) {
					return cl.Reads.Iterations(ctx)
				},
				printIterations)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	return cmd
}

func newServerQueueCmd(d deps) *cobra.Command {
	cmd := &cobra.Command{Use: "queue", Short: "server-wide queue", Args: cobra.NoArgs}
	var jsonOut bool
	list := &cobra.Command{
		Use:   "list",
		Short: "list every ticket with the scheduler's verdict on it",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return cmdbase.ServerRead(d.dial(), c.Context(), jsonOut, c.OutOrStdout(),
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
			return cmdbase.ServerRead(d.dial(), c.Context(), jsonOut, c.OutOrStdout(),
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
			return cmdbase.ServerQueueWrite(d.dial(), c, jsonOut, func(ctx context.Context, cl *apiclient.Client) (wire.QueueResult, error) {
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
			return cmdbase.ServerQueueWrite(d.dial(), c, jsonOut, func(ctx context.Context, cl *apiclient.Client) (wire.QueueResult, error) {
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
			return cmdbase.ServerQueueWrite(d.dial(), c, jsonOut, func(ctx context.Context, cl *apiclient.Client) (wire.QueueResult, error) {
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
			return cmdbase.ServerQueueWrite(d.dial(), c, jsonOut, func(ctx context.Context, cl *apiclient.Client) (wire.QueueResult, error) {
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
				return cmdbase.ServerQueueWrite(d.dial(), c, jsonOut, func(ctx context.Context, cl *apiclient.Client) (wire.QueueResult, error) {
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

func newServerTicketsCmd(d deps) *cobra.Command {
	cmd := &cobra.Command{Use: "tickets", Short: "ticket reads and writes", Args: cobra.NoArgs}
	var jsonOut bool
	history := &cobra.Command{
		Use:   "history <project:epic/NN>",
		Short: "print a ticket's events from its epic's run log",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return cmdbase.ServerRead(d.dial(), c.Context(), jsonOut, c.OutOrStdout(),
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
			return cmdbase.ServerRead(d.dial(), c.Context(), explainJSON, c.OutOrStdout(),
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
			return cmdbase.ServerRead(d.dial(), c.Context(), detailJSON, c.OutOrStdout(),
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
			cl, err := d.dial()()
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
				return cmdbase.ServerQueueWrite(d.dial(), c, pauseJSON, func(ctx context.Context, cl *apiclient.Client) (wire.QueueResult, error) {
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
			return cmdbase.ServerQueueWrite(d.dial(), c, unmuteJSON, func(ctx context.Context, cl *apiclient.Client) (wire.QueueResult, error) {
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
			return cmdbase.ServerQueueWrite(d.dial(), c, cancelJSON, func(ctx context.Context, cl *apiclient.Client) (wire.QueueResult, error) {
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
			return cmdbase.ServerQueueWrite(d.dial(), c, approveJSON, func(ctx context.Context, cl *apiclient.Client) (wire.QueueResult, error) {
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
				return cmdbase.ServerQueueWrite(d.dial(), c, retryJSON, func(ctx context.Context, cl *apiclient.Client) (wire.QueueResult, error) {
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
			return cmdbase.ServerQueueWrite(d.dial(), c, nudgeJSON, func(ctx context.Context, cl *apiclient.Client) (wire.QueueResult, error) {
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
			return cmdbase.ServerQueueWrite(d.dial(), c, recJSON, func(ctx context.Context, cl *apiclient.Client) (wire.QueueResult, error) {
				return cl.Tickets.Recommend(ctx, args[0], recVerb, diagnosis)
			})
		},
	}
	recommend.Flags().StringVar(&recVerb, "verb", "", "the recommended action (one of the ticket's detail actions)")
	recommend.Flags().String("diagnosis", "", "the diagnosis, or - to read it from stdin")
	recommend.Flags().BoolVar(&recJSON, "json", false, "emit the structured result (or refusal) as JSON")
	cmd.AddCommand(recommend)
	cmd.AddCommand(newTicketsAnswerCmd(d, true))

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
			client, err := d.dial()()
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

func printProjects(w io.Writer, projects []wire.ProjectInfo) error {
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

func printProjectWorktrees(w io.Writer, projects []wire.ProjectWorktrees) error {
	for _, p := range projects {
		if _, err := fmt.Fprintf(w, "%s\t%s\n", p.Project, p.Root); err != nil {
			return err
		}
		for _, wt := range p.Worktrees {
			if _, err := fmt.Fprintf(w, "%s\t%s\n", p.Project, wt); err != nil {
				return err
			}
		}
	}
	return nil
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
