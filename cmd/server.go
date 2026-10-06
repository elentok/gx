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
	var logOpts serverLogsOpts
	logs := &cobra.Command{
		Use:   "logs",
		Short: "print the server log (server.log)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
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
			return runServerLogs(ctx, server.LogPath(stateDir), logOpts, c.OutOrStdout())
		},
	}
	logs.Flags().BoolVarP(&logOpts.Follow, "follow", "f", false, "keep printing new lines as they are written")
	logs.Flags().StringVar(&logOpts.Level, "level", "", "minimum level to show: debug, info, warn or error")
	logs.Flags().BoolVar(&logOpts.JSON, "json", false, "print raw JSON lines instead of pretty output")
	cmd.AddCommand(logs)
	var snapJSON bool
	snapshot := &cobra.Command{
		Use:   "snapshot",
		Short: "print the server's ticket index and sequence number",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return runServerSnapshot(c.Context(), snapJSON, c.OutOrStdout())
		},
	}
	snapshot.Flags().BoolVar(&snapJSON, "json", false, "emit structured JSON instead of human-readable text")
	cmd.AddCommand(snapshot)
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
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	srv, err := server.New(server.Config{StateDir: stateDir, Build: getVersion(), TicketStore: cfg.TicketStore.Path})
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

func runServerSnapshot(ctx context.Context, jsonOut bool, w io.Writer) error {
	if ctx == nil {
		ctx = context.Background()
	}
	c, err := serverClient()
	if err != nil {
		return err
	}
	snap, err := c.Snapshot(ctx)
	if err != nil {
		return fmt.Errorf("server not reachable (is `gx server` running?): %w", err)
	}
	if jsonOut {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(snap)
	}
	if _, err := fmt.Fprintf(w, "seq: %d\n", snap.Seq); err != nil {
		return err
	}
	for _, t := range snap.Tickets {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\n", t.Address, t.Status, t.Title); err != nil {
			return err
		}
	}
	return nil
}

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
