package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/server"
	"github.com/spf13/cobra"
)

// KindInfo is one entry of the `gx server events kinds --json` payload.
type KindInfo struct {
	Kind       string `json:"kind"`
	CauseHerdr bool   `json:"cause_herdr"`
}

func newServerCmd(_ deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "orchestrator daemon commands",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return runServer(c.Context())
		},
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "show whether the server is running, with its pid and build",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return runServerStatus(c.Context(), c.OutOrStdout())
		},
	}, &cobra.Command{
		Use:   "stop",
		Short: "stop the running server (SIGTERM) and wait for it to exit",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return runServerStop(c.Context(), c.OutOrStdout())
		},
	})
	eventsCmd := &cobra.Command{
		Use:   "events",
		Short: "run-log event contract",
		Args:  cobra.NoArgs,
	}
	var jsonOut bool
	kinds := &cobra.Command{
		Use:   "kinds",
		Short: "print the closed event kind enum (no server needed)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return runServerEventsKinds(jsonOut, c.OutOrStdout())
		},
	}
	kinds.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	eventsCmd.AddCommand(kinds)
	cmd.AddCommand(eventsCmd)
	return cmd
}

// runServer runs the foreground server until interrupted.
func runServer(ctx context.Context) error {
	stateDir, err := config.StateDir()
	if err != nil {
		return err
	}
	srv, err := server.New(server.Config{StateDir: stateDir, Build: getVersion()})
	if err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	return srv.Serve(ctx)
}

func serverClient() (*apiclient.Client, error) {
	stateDir, err := config.StateDir()
	if err != nil {
		return nil, err
	}
	return apiclient.New(server.SocketPath(stateDir)), nil
}

func runServerStatus(ctx context.Context, w io.Writer) error {
	if ctx == nil {
		ctx = context.Background()
	}
	c, err := serverClient()
	if err != nil {
		return err
	}
	h, err := c.Handshake(ctx)
	if err != nil {
		_, werr := fmt.Fprintln(w, "not running")
		return werr
	}
	_, err = fmt.Fprintf(w, "running\npid: %d\nbuild: %s\n", h.Pid, h.Build)
	return err
}

// runServerStop SIGTERMs the server and returns once it stops answering,
// which happens after Serve has released the lock.
func runServerStop(ctx context.Context, w io.Writer) error {
	if ctx == nil {
		ctx = context.Background()
	}
	c, err := serverClient()
	if err != nil {
		return err
	}
	h, err := c.Handshake(ctx)
	if err != nil {
		_, werr := fmt.Fprintln(w, "not running")
		return werr
	}
	if err := syscall.Kill(h.Pid, syscall.SIGTERM); err != nil {
		return fmt.Errorf("signal server (pid %d): %w", h.Pid, err)
	}
	deadline := time.Now().Add(stopTimeout)
	for time.Now().Before(deadline) {
		if _, err := c.Handshake(ctx); err != nil {
			_, werr := fmt.Fprintf(w, "stopped (pid %d)\n", h.Pid)
			return werr
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("server (pid %d) still running after %s", h.Pid, stopTimeout)
}

const stopTimeout = 10 * time.Second

// runServerEventsKinds prints every event kind, reading Go data only.
func runServerEventsKinds(jsonOut bool, w io.Writer) error {
	all := events.Kinds()
	infos := make([]KindInfo, 0, len(all))
	for _, k := range all {
		infos = append(infos, KindInfo{Kind: string(k), CauseHerdr: k.CauseHerdr()})
	}
	if jsonOut {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(infos)
	}
	for _, i := range infos {
		line := i.Kind
		if i.CauseHerdr {
			line += "\tcause: herdr"
		}
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}
