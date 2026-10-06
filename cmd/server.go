package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
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
		Use:   "install",
		Short: "register a launchd agent that starts the server at login and after a crash (macOS)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return runServerInstall(c.OutOrStdout())
		},
	}, &cobra.Command{
		Use:   "start",
		Short: "start the server detached from this terminal (no-op if already running)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return runServerStartCmd(c.Context(), c.OutOrStdout())
		},
	}, &cobra.Command{
		Use:   "restart",
		Short: "stop the server and start it again (use after upgrading gx)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return runServerRestart(c.Context(), c.OutOrStdout())
		},
	}, &cobra.Command{
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
	n, err := c.Negotiate(ctx, getVersion())
	if err != nil {
		_, werr := fmt.Fprintln(w, "not running")
		return werr
	}
	if _, err = fmt.Fprintf(w, "running\npid: %d\nbuild: %s\n", n.Pid, n.Build); err != nil {
		return err
	}
	if n.Hint != "" {
		_, err = fmt.Fprintln(w, n.Hint)
	}
	return err
}

// spawnFunc launches a detached server and returns once it exits or the
// caller no longer needs it; its error is the launch failure.
type spawnFunc func() error

// runServerStart is idempotent: a server that already answers is left alone.
func runServerStart(ctx context.Context, c *apiclient.Client, w io.Writer, spawn spawnFunc) error {
	if h, err := c.Handshake(ctx); err == nil {
		_, werr := fmt.Fprintf(w, "already running (pid %d)\n", h.Pid)
		return werr
	}
	if err := spawn(); err != nil {
		return err
	}
	deadline := time.Now().Add(stopTimeout)
	for time.Now().Before(deadline) {
		if h, err := c.Handshake(ctx); err == nil {
			_, werr := fmt.Fprintf(w, "started (pid %d)\n", h.Pid)
			return werr
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("server did not answer within %s", stopTimeout)
}

// spawnDetached re-executes this binary as `gx server` in its own session with
// no terminal, so closing the launching terminal cannot stop it. exec.Cmd
// already closes every fd except stdio, and stdio is /dev/null plus the log.
func spawnDetached(stateDir string) spawnFunc {
	return func() error {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(stateDir, 0o700); err != nil {
			return err
		}
		logf, err := os.OpenFile(filepath.Join(stateDir, "server.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return err
		}
		defer logf.Close()
		child := exec.Command(exe, "server")
		child.Stdout, child.Stderr = logf, logf // Stdin nil = /dev/null
		child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := child.Start(); err != nil {
			return fmt.Errorf("start server: %w", err)
		}
		return child.Process.Release()
	}
}

// installedService returns the OS supervisor when `gx server install` has been
// run, nil otherwise. Once installed it is the only way to start the server.
func installedService(stateDir string) serviceManager {
	a, err := newLaunchdAgent(stateDir)
	if err != nil || !a.Installed() {
		return nil
	}
	return a
}

func runServerInstall(w io.Writer) error {
	stateDir, err := config.StateDir()
	if err != nil {
		return err
	}
	a, err := newLaunchdAgent(stateDir)
	if err != nil {
		return err
	}
	if err := a.Install(); err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "installed %s\n", a.plistPath())
	return err
}

func runServerStartCmd(ctx context.Context, w io.Writer) error {
	if ctx == nil {
		ctx = context.Background()
	}
	stateDir, err := config.StateDir()
	if err != nil {
		return err
	}
	c, err := serverClient()
	if err != nil {
		return err
	}
	spawn := spawnDetached(stateDir)
	if svc := installedService(stateDir); svc != nil {
		spawn = svc.Start
	}
	return runServerStart(ctx, c, w, spawn)
}

func runServerRestart(ctx context.Context, w io.Writer) error {
	if err := runServerStop(ctx, w); err != nil {
		return err
	}
	return runServerStartCmd(ctx, w)
}

func runServerStop(ctx context.Context, w io.Writer) error {
	stateDir, err := config.StateDir()
	if err != nil {
		return err
	}
	c, err := serverClient()
	if err != nil {
		return err
	}
	sig := func(pid int) error { return syscall.Kill(pid, syscall.SIGTERM) }
	if svc := installedService(stateDir); svc != nil {
		sig = func(int) error { return svc.Stop() }
	}
	return stopServer(ctx, c, w, sig)
}

// stopServer signals the server and returns once it stops answering, which
// happens after Serve has released the lock.
func stopServer(ctx context.Context, c *apiclient.Client, w io.Writer, signal func(pid int) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	h, err := c.Handshake(ctx)
	if err != nil {
		_, werr := fmt.Fprintln(w, "not running")
		return werr
	}
	if err := signal(h.Pid); err != nil {
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
