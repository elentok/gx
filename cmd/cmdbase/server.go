package cmdbase

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/wire"
	"github.com/spf13/cobra"
)

// LogFollowInterval is how often a followed log (server or agent) is polled.
const LogFollowInterval = 200 * time.Millisecond

// ServerClient returns the real client for the server's socket under dirs.
func ServerClient(dirs config.Dirs) (*apiclient.Client, error) {
	stateDir, err := dirs.StateDir()
	if err != nil {
		return nil, err
	}
	return apiclient.New(server.SocketPath(stateDir)), nil
}

// ServerRead fetches one read payload from the server and prints it: as
// indented JSON, or through text when jsonOut is false.
func ServerRead[T any](dial func() (*apiclient.Client, error), ctx context.Context, jsonOut bool, w io.Writer, fetch func(context.Context, *apiclient.Client) (T, error), text func(io.Writer, T) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	c, err := dial()
	if err != nil {
		return err
	}
	v, err := fetch(ctx, c)
	if err != nil {
		return fmt.Errorf("server read failed (is `gx server` running?): %w", err)
	}
	if jsonOut {
		return EncodeProvenance(w, v, ViaServer, CallerActor(os.Getwd))
	}
	return text(w, v)
}

// ServerQueueWrite runs one queue write. With --json the result or refusal is
// the output; otherwise a refusal is an error.
func ServerQueueWrite(dial func() (*apiclient.Client, error), c *cobra.Command, jsonOut bool, do func(context.Context, *apiclient.Client) (wire.QueueResult, error)) error {
	ctx := c.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	cl, err := dial()
	if err != nil {
		return err
	}
	return RunServerQueueWrite(ctx, cl, c.OutOrStdout(), jsonOut, do)
}

// RunServerQueueWrite never starts the server: a dead socket is a refusal
// with a `gx server start` hint.
func RunServerQueueWrite(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool, do func(context.Context, *apiclient.Client) (wire.QueueResult, error)) error {
	res, err := do(ctx, cl)
	switch {
	case apiclient.IsNotRunning(err):
		res = wire.QueueResult{Refused: true, Reason: wire.ReasonServerNotRunning, Message: "no server is running; start it with `gx server start`"}
	case err != nil:
		return fmt.Errorf("server write failed: %w", err)
	}
	if jsonOut {
		return EncodeProvenance(w, res, ViaServer, CallerActor(os.Getwd))
	}
	if res.Refused {
		return fmt.Errorf("refused (%s): %s", res.Reason, res.Message)
	}
	if res.Paused {
		return fmt.Errorf("paused (%s): %s", res.PauseKind, res.Message)
	}
	if res.Merged {
		_, err := fmt.Fprintln(w, "merged")
		return err
	}
	if res.MergeTicket != "" {
		_, err := fmt.Fprintln(w, res.MergeTicket)
		return err
	}
	for i, it := range res.Queue {
		if _, err := fmt.Fprintf(w, "%d\t%s\t%s\n", i+1, it.Address, it.Agent); err != nil {
			return err
		}
	}
	return nil
}
