package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/tickets"
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
	var statusJSON bool
	status := &cobra.Command{
		Use:   "status",
		Short: "show whether the server is running, with its pid and build",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return runServerStatus(c.Context(), statusJSON, c.OutOrStdout())
		},
	}
	status.Flags().BoolVar(&statusJSON, "json", false, "emit structured JSON instead of human-readable text")
	var restartMigrate bool
	restartCmd := &cobra.Command{
		Use:   "restart",
		Short: "stop the server and start it again (use after upgrading gx)",
		Long: `Stop the server and start it again.

With --migrate, run "gx tickets migrate" on every project in the ticket store
between the stop and the start. The stop does not wait for running tickets. If
a migration fails, the failed store is named and the server stays stopped.`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return runServerRestart(c.Context(), c.OutOrStdout(), restartMigrate)
		},
	}
	restartCmd.Flags().BoolVar(&restartMigrate, "migrate", false, "migrate every project's ticket store while the server is stopped")
	cmd.AddCommand(status, &cobra.Command{
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
	}, restartCmd, &cobra.Command{
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
	cmd.AddCommand(eventsCmd, newServerLocksCmd(), newServerTicketsCmd(), newServerEpicsCmd(), newServerStatsCmd(), newServerIterationsCmd(), newServerAgentsCmd(), newServerQueueCmd(), newServerOneOffCmd(), newServerReloadConfigCmd(), newServerConfigStatusCmd(), newServerCPUSpikeCmd(), newServerWebTokenCmd(), newServerWebURLCmd())
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
	choice, err := preflightAgentRunner(cfg.AgentRunner, liveProbe, server.LogPath(stateDir))
	if err != nil {
		return err
	}
	runner, runnerFor := serverRunner(choice, stateDir, cfg.RunnerWait)
	var tcpAddr string
	if cfg.Server.TCPListen {
		tcpAddr = server.DefaultTCPAddr
	}
	configPath, err := config.FilePath()
	if err != nil {
		return err
	}
	var logLevel slog.Level
	if v := os.Getenv("GX_SERVER_LOG_LEVEL"); v != "" {
		if err := logLevel.UnmarshalText([]byte(v)); err != nil {
			return fmt.Errorf("GX_SERVER_LOG_LEVEL: %w", err)
		}
	}
	srv, err := server.New(server.Config{Settings: server.SettingsFromConfig(cfg), StateDir: stateDir, Build: getVersion(), Runner: runner, RunnerFor: runnerFor,
		TicketStore: cfg.TicketStore.Path, ConfigPath: configPath, AgentRunner: cfg.AgentRunner, TCPAddr: tcpAddr, WebAddr: cfg.Server.Web.Listen, WebDevURL: cfg.Server.Web.DevFrontendURL(), LogLevel: logLevel})
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

// StatusInfo is the `gx server status --json` payload.
type StatusInfo struct {
	Running bool   `json:"running"`
	Pid     int    `json:"pid,omitempty"`
	Build   string `json:"build,omitempty"`
	Paused  bool   `json:"paused,omitempty"`
	// WebURL is the web UI link with its token; empty when the web listener is off.
	WebURL string `json:"web_url,omitempty"`
	// Starting: a server holds the lock but does not answer yet. It builds the
	// ticket index before it opens its socket.
	Starting bool     `json:"starting,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
	// Budget is today's spend and limits; nil when the server did not answer.
	Budget *server.BudgetStatus `json:"budget,omitempty"`
	// Config is the reload status; nil when there is nothing to show.
	Config *server.ConfigStatus `json:"config,omitempty"`
	// ConfigError is why config.json does not load; status then uses defaults.
	ConfigError string `json:"config_error,omitempty"`
}

func runServerStatus(ctx context.Context, jsonOut bool, w io.Writer) error {
	if ctx == nil {
		ctx = context.Background()
	}
	c, err := serverClient()
	if err != nil {
		return err
	}
	return runServerStatusWith(ctx, c, jsonOut, w)
}

func runServerStatusWith(ctx context.Context, c *apiclient.Client, jsonOut bool, w io.Writer) error {
	cfg, cfgErr := config.Load()
	if cfgErr != nil {
		cfg = config.Default()
	}
	info := StatusInfo{}
	n, err := c.Negotiate(ctx, getVersion())
	if err == nil {
		stateDir, serr := config.StateDir()
		if serr != nil {
			return serr
		}
		paused, perr := server.QueuePaused(stateDir)
		if perr != nil {
			return perr
		}
		info = StatusInfo{Running: true, Pid: n.Pid, Build: n.Build, Paused: paused, Warnings: statusWarnings(n, cfg.TicketStore.Path)}
		if n.WebURL != "" {
			token, terr := server.ReadWebToken(stateDir)
			if terr != nil {
				return terr
			}
			info.WebURL = n.WebURL + "/?gx_token=" + token
		}
		if b, berr := c.Budget(ctx); berr == nil {
			info.Budget = &b
		}
		if st, serr := c.ConfigStatus(ctx); serr == nil && !st.Empty() {
			info.Config = &st
		}
	}
	if cfgErr != nil {
		info.ConfigError = cfgErr.Error()
	}
	if !info.Running {
		if stateDir, serr := config.StateDir(); serr == nil {
			if pid, held := server.LockHeldBy(stateDir); held {
				info.Starting, info.Pid = true, pid
			}
		}
	}
	if jsonOut {
		via := viaDirect
		if info.Running {
			via = viaServer
		}
		return encodeProvenance(w, info, via, callerActor(os.Getwd))
	}
	if info.ConfigError != "" {
		if _, err = fmt.Fprintln(w, "config error: "+info.ConfigError); err != nil {
			return err
		}
	}
	if info.Starting {
		_, werr := fmt.Fprintf(w, "starting (pid %d): indexing the ticket store, not answering yet\n", info.Pid)
		return werr
	}
	if !info.Running {
		_, werr := fmt.Fprintln(w, "not running")
		return werr
	}
	if _, err = fmt.Fprintf(w, "running\npid: %d\nbuild: %s\n", info.Pid, info.Build); err != nil {
		return err
	}
	if info.WebURL != "" {
		if _, err = fmt.Fprintln(w, "web: "+info.WebURL); err != nil {
			return err
		}
	}
	if info.Paused {
		if _, err = fmt.Fprintln(w, "queue: paused (gx server queue resume)"); err != nil {
			return err
		}
	}
	if info.Budget != nil {
		if _, err = fmt.Fprintln(w, "budget: "+budgetLine(*info.Budget)); err != nil {
			return err
		}
	}
	if info.Config != nil {
		if err = writeConfigBlock(w, *info.Config, time.Now()); err != nil {
			return err
		}
	}
	for _, warning := range info.Warnings {
		if _, err = fmt.Fprintln(w, "warning: "+warning); err != nil {
			return err
		}
	}
	return nil
}

// statusWarnings lists what `gx server status` flags. n.Hint covers the
// binary-on-disk-vs-running-build mismatch (and API mismatch).
func statusWarnings(n apiclient.Negotiation, storePath string) []string {
	var out []string
	if n.TCPAddr != "" {
		out = append(out, fmt.Sprintf("TCP listener is on (%s): any local process can use the API, no auth", n.TCPAddr))
	}
	if n.WebError != "" {
		out = append(out, "web UI listener failed to bind: "+n.WebError)
	}
	if n.Hint != "" {
		out = append(out, n.Hint)
	}
	if !hasPushRemote(storePath) {
		out = append(out, "ticket store has no push remote configured; tickets are not backed up")
	}
	return out
}

func hasPushRemote(storePath string) bool {
	out, err := exec.Command("git", "-C", storePath, "remote").Output()
	return err == nil && strings.TrimSpace(string(out)) != ""
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
	return runServerStart(ctx, c, w, serverSpawn(stateDir))
}

// serverSpawn is the installed service when there is one, else a detached child.
func serverSpawn(stateDir string) spawnFunc {
	if svc := installedService(stateDir); svc != nil {
		return svc.Start
	}
	return spawnDetached(stateDir)
}

// serverStarter is runServerStart without output, for the TUI.
func serverStarter(c *apiclient.Client) func(context.Context) error {
	return func(ctx context.Context) error {
		stateDir, err := config.StateDir()
		if err != nil {
			return err
		}
		return runServerStart(ctx, c, io.Discard, serverSpawn(stateDir))
	}
}

func runServerRestart(ctx context.Context, w io.Writer, migrate bool) error {
	return restartSteps(migrate,
		func() error { return runServerStop(ctx, w) },
		func() error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			return migrateAllProjects(cfg.TicketStore.Path, w)
		},
		func() error { return runServerStartCmd(ctx, w) },
	)
}

// restartSteps stops, optionally migrates, then starts. A failed migration
// leaves the server stopped, so the person can retry or start without it.
func restartSteps(migrate bool, stop, migrateStores, start func() error) error {
	if err := stop(); err != nil {
		return err
	}
	if migrate {
		if err := migrateStores(); err != nil {
			return fmt.Errorf("%w (server left stopped: fix and retry, or run `gx server start` to skip migrating)", err)
		}
	}
	return start()
}

// migrateAllProjects runs `gx tickets migrate` on every project in the store
// and stops at the first failure.
func migrateAllProjects(storePath string, w io.Writer) error {
	dirs, err := tickets.ProjectDirs(storePath)
	if err != nil {
		return err
	}
	for _, dir := range dirs {
		name := filepath.Base(dir)
		fmt.Fprintf(w, "migrating %s\n", name)
		if err := runTicketsMigrate(dir, false, w); err != nil {
			return fmt.Errorf("migrate %s: %w", name, err)
		}
	}
	return nil
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
			// The socket closes before the process exits. A launchd kickstart
			// in that gap sees the job still running and does nothing, so
			// restart would leave the server stopped.
			if err := waitForExit(h.Pid, deadline, processAlive); err != nil {
				return err
			}
			_, werr := fmt.Fprintf(w, "stopped (pid %d)\n", h.Pid)
			return werr
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("server (pid %d) still running after %s", h.Pid, stopTimeout)
}

// waitForExit polls until pid is gone or deadline passes.
func waitForExit(pid int, deadline time.Time, alive func(pid int) bool) error {
	for alive(pid) {
		if !time.Now().Before(deadline) {
			return fmt.Errorf("server (pid %d) still running after %s", pid, stopTimeout)
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil
}

// processAlive reports whether pid still exists (signal 0 checks without
// sending anything).
func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// stopTimeout outlasts server.DefaultMergeStopTimeout so a stop that waits for a
// merge is not reported as a failure.
const stopTimeout = server.DefaultMergeStopTimeout + 15*time.Second

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
		return encodeProvenance(w, snap, viaServer, callerActor(os.Getwd))
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

// runServerTicketsFollow prints addr's state on every (re-)snapshot where it
// changed, and each event naming addr. Other tickets' events are dropped.
// With jsonOut each output line is one JSON object (events, and ticket state
// or {"address":...,"absent":true}).
func runServerTicketsFollow(ctx context.Context, c *apiclient.Client, addr string, jsonOut bool, w io.Writer) error {
	var last string
	var werr error
	print := func(text string, v any) {
		if werr != nil {
			return
		}
		if jsonOut {
			werr = json.NewEncoder(w).Encode(v)
			return
		}
		_, werr = fmt.Fprintln(w, text)
	}
	err := c.Follow(ctx, func(s *server.Snapshot, ev *server.Event) {
		if ev != nil {
			if ev.Address == addr {
				print(fmt.Sprintf("%d\t%s\t%s", ev.Seq, ev.Type, ev.Address), ev)
			}
			return
		}
		line, state := "absent", any(map[string]any{"address": addr, "absent": true})
		for _, t := range s.Tickets {
			if t.Address == addr {
				line, state = fmt.Sprintf("%s\t%s\t%s", t.Address, t.Status, t.Title), t
			}
		}
		if line != last {
			last = line
			print(line, state)
		}
	})
	if werr != nil {
		return werr
	}
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
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
