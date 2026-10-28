package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/elentok/gx/agentlog"
	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server"
)

type agentsWatchOpts struct {
	Follow bool
	Tail   int
	JSON   bool
	Screen bool
	Raw    bool
}

var errNotPTY = errors.New("not a pty agent: --screen and --raw need one")

// runAgentsScreen prints a pty agent's screen: the holder's live one, else
// the one it saved when the agent ended.
func runAgentsScreen(log agentlog.Log, w io.Writer) error {
	if log.PtyDir == "" {
		return errNotPTY
	}
	if h, err := nativerunner.DialHolder(log.PtyDir); err == nil {
		if rows, _, err := h.Screen(0); err == nil {
			_, err = fmt.Fprintln(w, strings.Join(rows, "\n"))
			return err
		}
	}
	data, err := os.ReadFile(filepath.Join(log.PtyDir, nativerunner.ScreenFile))
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("no live holder and no saved screen in %s", log.PtyDir)
	}
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// runAgentsRaw copies a pty agent's raw.log to w. The bytes only render
// right on a terminal as wide as the pty, so a narrower one (width > 0) gets
// a warning on warn first.
func runAgentsRaw(log agentlog.Log, w, warn io.Writer, width int) error {
	if log.PtyDir == "" {
		return errNotPTY
	}
	f, err := os.Open(filepath.Join(log.PtyDir, nativerunner.RawLogFile))
	if err != nil {
		return err
	}
	defer f.Close()
	if width > 0 && width < nativerunner.HolderCols {
		fmt.Fprintf(warn, "warning: the terminal is %d columns wide; raw.log needs %d to render right\n", width, nativerunner.HolderCols)
	}
	_, err = io.Copy(w, f)
	return err
}

// stdoutWidth is stdout's terminal width, 0 when it is not a terminal.
func stdoutWidth() int {
	w, _, err := term.GetSize(os.Stdout.Fd())
	if err != nil {
		return 0
	}
	return w
}

func newServerAgentsCmd(d deps) *cobra.Command {
	cmd := &cobra.Command{Use: "agents", Short: "inspect ralph-loop agents", Args: cobra.NoArgs}
	var opts agentsWatchOpts
	watch := &cobra.Command{
		Use:   "watch <epic>/<id>",
		Short: "print a ticket agent's log",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			a, _, err := findTicket(d.dirs(), cwd, args[0])
			if err != nil {
				return err
			}
			stateDir, err := d.dirs().StateDir()
			if err != nil {
				return err
			}
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
			defer stop()
			loc := agentlog.Locator{StateDir: stateDir, Herdr: herdrIteration(d.dial())}
			log, err := loc.AgentLog(ctx, a)
			if err != nil {
				return err
			}
			switch {
			case opts.Screen:
				err = runAgentsScreen(log, c.OutOrStdout())
			case opts.Raw:
				err = runAgentsRaw(log, c.OutOrStdout(), c.ErrOrStderr(), stdoutWidth())
			default:
				return runAgentsWatch(ctx, log, opts, c.OutOrStdout())
			}
			if errors.Is(err, errNotPTY) {
				return fmt.Errorf("%s: %w", a, err)
			}
			return err
		},
	}
	watch.Flags().BoolVarP(&opts.Follow, "follow", "f", false, "keep printing new events until the agent ends")
	watch.Flags().IntVar(&opts.Tail, "tail", 0, "show only the last N lines (N events with --json)")
	watch.Flags().BoolVar(&opts.JSON, "json", false, "print raw JSONL events instead of rendered lines")
	watch.Flags().BoolVar(&opts.Screen, "screen", false, "print a pty agent's current screen (the saved one once it ended)")
	watch.Flags().BoolVar(&opts.Raw, "raw", false, fmt.Sprintf("replay a pty agent's raw terminal output (needs a %d-column terminal)", nativerunner.HolderCols))
	watch.MarkFlagsMutuallyExclusive("screen", "raw", "json")
	watch.MarkFlagsMutuallyExclusive("screen", "raw", "follow")
	watch.MarkFlagsMutuallyExclusive("screen", "raw", "tail")
	cmd.AddCommand(watch)
	cmd.AddCommand(agentsWriteCmd(d, "prompt <project:epic/NN> <text>", "send a prompt to a ticket's live agent", cobra.ExactArgs(2),
		func(ctx context.Context, cl *apiclient.Client, args []string) (server.QueueResult, error) {
			return cl.AgentPrompt(ctx, args[0], args[1])
		}))
	cmd.AddCommand(agentsWriteCmd(d, "interrupt <project:epic/NN>", "stop the current turn of a ticket's live agent", cobra.ExactArgs(1),
		func(ctx context.Context, cl *apiclient.Client, args []string) (server.QueueResult, error) {
			return cl.AgentInterrupt(ctx, args[0])
		}))
	var requestID string
	answer := agentsWriteCmd(d, "answer <project:epic/NN> <allow|deny> [reason]", "answer the permission request a ticket's agent waits on", cobra.RangeArgs(2, 3),
		func(ctx context.Context, cl *apiclient.Client, args []string) (server.QueueResult, error) {
			req := server.QueueRequest{Address: args[0], Decision: args[1], RequestID: requestID, Source: "cli"}
			if len(args) == 3 {
				req.Text = args[2]
			}
			return cl.AgentAnswerRequest(ctx, req)
		})
	answer.Flags().StringVar(&requestID, "request-id", "", "answer only this permission request; refused if the open one differs")
	cmd.AddCommand(answer)
	return cmd
}

// agentsWriteCmd is one `gx server agents` verb that writes through the server.
func agentsWriteCmd(d deps, use, short string, args cobra.PositionalArgs, do func(context.Context, *apiclient.Client, []string) (server.QueueResult, error)) *cobra.Command {
	var jsonOut bool
	c := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  args,
		RunE: func(c *cobra.Command, args []string) error {
			return serverQueueWrite(d.dial(), c, jsonOut, func(ctx context.Context, cl *apiclient.Client) (server.QueueResult, error) {
				return do(ctx, cl, args)
			})
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return c
}

// herdrIteration looks address up among the server's live iterations.
func herdrIteration(dial func() (*apiclient.Client, error)) func(ctx context.Context, address string) (agentlog.HerdrIteration, bool, error) {
	return func(ctx context.Context, address string) (agentlog.HerdrIteration, bool, error) {
		cl, err := dial()
		if err != nil {
			return agentlog.HerdrIteration{}, false, err
		}
		its, err := cl.Iterations(ctx)
		if err != nil {
			return agentlog.HerdrIteration{}, false, err
		}
		for _, it := range its {
			if it.Address == address {
				return agentlog.HerdrIteration{Agent: it.Agent, Transcript: it.Transcript}, true, nil
			}
		}
		return agentlog.HerdrIteration{}, false, nil
	}
}

// runAgentsWatch prints log from its offset. With Follow it keeps polling
// until ctx is done or the agent has ended and no new data is left.
func runAgentsWatch(ctx context.Context, log agentlog.Log, opts agentsWatchOpts, w io.Writer) error {
	if log.Pruned {
		_, err := fmt.Fprintf(w, "log pruned; transcript: %s\n", log.Transcript)
		return err
	}
	if !opts.Follow {
		log.Ended = nil
	}
	var render func([]byte) []string
	if opts.JSON {
		render = func(raw []byte) []string { return []string{string(raw)} }
	}
	t, err := agentlog.OpenTail(log, render)
	if err != nil {
		return err
	}
	defer t.Close()
	write := func(lines []string) error {
		for _, l := range lines {
			if _, err := fmt.Fprintln(w, l); err != nil {
				return err
			}
		}
		return nil
	}

	// The backlog is the first step, so --tail can cut it; later lines stream.
	backlog, ended, err := t.Step()
	if err != nil {
		return err
	}
	if !opts.Follow {
		backlog = append(backlog, t.Flush()...)
	}
	if opts.Tail > 0 && len(backlog) > opts.Tail {
		backlog = backlog[len(backlog)-opts.Tail:]
	}
	if err := write(backlog); err != nil {
		return err
	}
	for opts.Follow && !ended {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(logFollowInterval):
		}
		var lines []string
		if lines, ended, err = t.Step(); err != nil {
			return err
		}
		if err := write(lines); err != nil {
			return err
		}
	}
	return nil
}
