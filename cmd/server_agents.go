package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/agentlog"
	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
)

type agentsWatchOpts struct {
	Follow bool
	Tail   int
	JSON   bool
}

func newServerAgentsCmd() *cobra.Command {
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
			a, _, err := findTicket(cwd, args[0])
			if err != nil {
				return err
			}
			stateDir, err := config.StateDir()
			if err != nil {
				return err
			}
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
			defer stop()
			loc := agentlog.Locator{StateDir: stateDir, Herdr: herdrIteration}
			log, err := loc.AgentLog(ctx, a)
			if err != nil {
				return err
			}
			return runAgentsWatch(ctx, log, opts, c.OutOrStdout())
		},
	}
	watch.Flags().BoolVarP(&opts.Follow, "follow", "f", false, "keep printing new events until the agent ends")
	watch.Flags().IntVar(&opts.Tail, "tail", 0, "show only the last N lines (N events with --json)")
	watch.Flags().BoolVar(&opts.JSON, "json", false, "print raw JSONL events instead of rendered lines")
	cmd.AddCommand(watch)
	cmd.AddCommand(agentsWriteCmd("prompt <project:epic/NN> <text>", "send a prompt to a ticket's live agent", cobra.ExactArgs(2),
		func(ctx context.Context, cl *apiclient.Client, args []string) (server.QueueResult, error) {
			return cl.AgentPrompt(ctx, args[0], args[1])
		}))
	cmd.AddCommand(agentsWriteCmd("interrupt <project:epic/NN>", "stop the current turn of a ticket's live agent", cobra.ExactArgs(1),
		func(ctx context.Context, cl *apiclient.Client, args []string) (server.QueueResult, error) {
			return cl.AgentInterrupt(ctx, args[0])
		}))
	cmd.AddCommand(agentsWriteCmd("answer <project:epic/NN> <allow|deny> [reason]", "answer the permission request a ticket's agent waits on", cobra.RangeArgs(2, 3),
		func(ctx context.Context, cl *apiclient.Client, args []string) (server.QueueResult, error) {
			var text string
			if len(args) == 3 {
				text = args[2]
			}
			return cl.AgentAnswer(ctx, args[0], args[1], text)
		}))
	return cmd
}

// agentsWriteCmd is one `gx server agents` verb that writes through the server.
func agentsWriteCmd(use, short string, args cobra.PositionalArgs, do func(context.Context, *apiclient.Client, []string) (server.QueueResult, error)) *cobra.Command {
	var jsonOut bool
	c := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  args,
		RunE: func(c *cobra.Command, args []string) error {
			return serverQueueWrite(c, jsonOut, func(ctx context.Context, cl *apiclient.Client) (server.QueueResult, error) {
				return do(ctx, cl, args)
			})
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return c
}

// herdrIteration looks address up among the server's live iterations.
func herdrIteration(ctx context.Context, address string) (agentlog.HerdrIteration, bool, error) {
	cl, err := serverClient()
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

// runAgentsWatch prints log from its offset. With Follow it keeps polling
// until ctx is done or the agent has ended and no new data is left.
func runAgentsWatch(ctx context.Context, log agentlog.Log, opts agentsWatchOpts, w io.Writer) error {
	if log.Pruned {
		_, err := fmt.Fprintf(w, "log pruned; transcript: %s\n", log.Transcript)
		return err
	}
	f, err := os.Open(log.Path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Seek(log.Offset, io.SeekStart); err != nil {
		return err
	}

	format := func(line string) []string {
		if line == "" {
			return nil
		}
		if opts.JSON {
			return []string{line}
		}
		return agentlog.Render([]byte(line))
	}
	write := func(lines []string) error {
		for _, l := range lines {
			if _, err := fmt.Fprintln(w, l); err != nil {
				return err
			}
		}
		return nil
	}

	// The backlog is buffered so --tail can cut it; later lines stream.
	var backlog []string
	inBacklog := true
	flushBacklog := func() error {
		if opts.Tail > 0 && len(backlog) > opts.Tail {
			backlog = backlog[len(backlog)-opts.Tail:]
		}
		inBacklog = false
		return write(backlog)
	}

	r := bufio.NewReader(f)
	var partial string
	// lastDrain: the agent was seen ended, so the next EOF is final. Reading
	// once more after that catches what it wrote just before exiting.
	lastDrain := false
	for {
		chunk, err := r.ReadString('\n')
		if err == nil {
			lines := format(partial + strings.TrimSuffix(chunk, "\n"))
			partial = ""
			if inBacklog {
				backlog = append(backlog, lines...)
			} else if werr := write(lines); werr != nil {
				return werr
			}
			continue
		}
		if err != io.EOF {
			return err
		}
		partial += chunk
		if !opts.Follow || lastDrain {
			if inBacklog {
				backlog = append(backlog, format(partial)...)
				return flushBacklog()
			}
			return write(format(partial))
		}
		if inBacklog {
			if err := flushBacklog(); err != nil {
				return err
			}
		}
		if log.Ended != nil && log.Ended() {
			lastDrain = true
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(logFollowInterval):
		}
	}
}
