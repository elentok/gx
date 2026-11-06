package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/testutil/gxfixture/websignin"
)

// runningServer is the started gx server and what the web login gave the caller.
type runningServer struct {
	cmd      *exec.Cmd
	exited   chan error
	logPath  string
	logFile  *os.File
	webURL   string
	in       websignin.SignIn
	password string
}

// seeded holds the paths and maps that post-start seeding hands to the output.
type seeded struct {
	agentTranscript  string
	checkWaitingFile string
	transcripts      map[string]string
}

// startServer launches `gx server` with its output in the log file. The caller closes
// logFile and shuts the server down.
func (f *fixture) startServer(ctx context.Context, sc scenario) (*runningServer, error) {
	rs := &runningServer{cmd: exec.Command(f.gx, "server"), logPath: filepath.Join(f.root, "server.out")}
	rs.cmd.Env = f.env
	logFile, err := os.Create(rs.logPath)
	if err != nil {
		return nil, err
	}
	rs.logFile = logFile
	rs.cmd.Stdout, rs.cmd.Stderr = logFile, logFile
	if err := rs.cmd.Start(); err != nil {
		return rs, err
	}
	if sc.Migrate {
		if err := f.writeServerControl(rs.cmd.Process.Pid, rs.logPath); err != nil {
			_ = rs.cmd.Process.Kill()
			return rs, err
		}
	}
	rs.exited = make(chan error, 1)
	go func() {
		defer close(rs.exited)
		err := rs.cmd.Wait()
		select {
		case rs.exited <- err:
		case <-ctx.Done():
		}
	}()
	return rs, nil
}

// loginWeb waits for the server and seeds the web login the scenario asks for.
func (f *fixture) loginWeb(ctx context.Context, sc scenario, rs *runningServer) error {
	webURL, setupCode, err := f.waitReady(ctx, rs.exited)
	if err != nil {
		return fmt.Errorf("%w\n%s", err, f.serverOutput())
	}
	rs.webURL = webURL
	if sc.Web != webSetupPending {
		if rs.in, err = websignin.Setup(f.stateD, webURL, setupCode); err != nil {
			return fmt.Errorf("web setup: %w\n%s", err, f.serverOutput())
		}
		rs.password = websignin.Password
	}
	if sc.Web == webSignedOut {
		rs.in.CookieName, rs.in.Cookie = "", ""
	}
	return nil
}

// queueAdd queues one ticket, with its scenario agent if any, through the CLI.
func (f *fixture) queueAdd(sc scenario, addr string) error {
	args := []string{"server", "queue", "add", addr}
	if agent := sc.QueueAgents[addr]; agent != "" {
		args = append(args, "--agent", agent)
	}
	if out, err := f.cli(args...); err != nil {
		return fmt.Errorf("queue %s: %w\n%s", addr, err, out)
	}
	return nil
}

func (f *fixture) queueAll(ctx context.Context, sc scenario) error {
	for _, addr := range sc.Queue {
		// A held migration holds the queue verb too; it finishes once the hold is released.
		if sc.MigrateHold {
			args := []string{"server", "queue", "add", addr}
			if agent := sc.QueueAgents[addr]; agent != "" {
				args = append(args, "--agent", agent)
			}
			cmd := exec.CommandContext(ctx, f.gx, args...)
			cmd.Env = f.env
			if err := cmd.Start(); err != nil {
				return fmt.Errorf("queue %s: %w", addr, err)
			}
			go func() { _ = cmd.Wait() }()
			continue
		}
		if err := f.queueAdd(sc, addr); err != nil {
			return err
		}
	}
	return nil
}

// queueAfterUsage queues the tickets that wait for the server's first usage sample.
func (f *fixture) queueAfterUsage(sc scenario) error {
	sampleFile := "usage-sample.json"
	if sc.CodexUsage {
		sampleFile = "usage-sample-codex.json"
	}
	sample := filepath.Join(f.root, "state", "gx", sampleFile)
	deadline := time.Now().Add(30 * time.Second)
	for _, err := os.Stat(sample); err != nil; _, err = os.Stat(sample) {
		if time.Now().After(deadline) {
			return fmt.Errorf("no usage sample at %s", sample)
		}
		time.Sleep(100 * time.Millisecond)
	}
	for _, addr := range sc.QueueAfterUsage {
		if err := f.queueAdd(sc, addr); err != nil {
			return err
		}
	}
	return nil
}

// seedAfterStart seeds what needs the running server: queue, agents, sessions and designs.
func (f *fixture) seedAfterStart(ctx context.Context, o opts, sc scenario) (s seeded, err error) {
	// The earlier run goes first so no held worker holds its slot.
	for _, addr := range sc.RanOnClaude {
		if err := sessionsOf(f).seedRanOnClaude(addr); err != nil {
			return s, err
		}
	}
	// --hold freezes every worker as running until the caller removes the hold file.
	if o.hold || sc.Hold {
		if err := os.WriteFile(f.hold, nil, 0o644); err != nil {
			return s, err
		}
	}
	if !sc.QueueAfterSessions {
		if err := f.queueAll(ctx, sc); err != nil {
			return s, err
		}
	}
	for _, args := range sc.Steps {
		if out, err := f.cli(args...); err != nil {
			return s, fmt.Errorf("gx %s: %w\n%s", strings.Join(args, " "), err, out)
		}
	}
	if len(sc.QueueAfterUsage) > 0 {
		if err := f.queueAfterUsage(sc); err != nil {
			return s, err
		}
	}
	if sc.AgentProcs {
		if err := f.tieAgentProcesses(ctx); err != nil {
			return s, err
		}
	}
	if sc.Spend {
		if s.agentTranscript, err = f.agentTranscript(ctx); err != nil {
			return s, err
		}
	}
	if sc.CheckWaiting {
		if s.checkWaitingFile, err = writeCheckWaiting(ctx, f); err != nil {
			return s, err
		}
	}
	if err := sessionsOf(f).seedSessions(sc); err != nil {
		return s, err
	}
	if sc.QueueAfterSessions {
		if err := f.queueAll(ctx, sc); err != nil {
			return s, err
		}
	}
	if err := f.seedDesigns(ctx, sc); err != nil {
		return s, err
	}
	if sc.TokenPoll {
		if s.transcripts, err = f.liveTranscripts(ctx); err != nil {
			return s, err
		}
	}
	return s, nil
}

// seedDesigns makes the plain, opened and linked design sessions.
func (f *fixture) seedDesigns(ctx context.Context, sc scenario) error {
	// Plain sessions are made before openDesigns, which then starts the newer
	// run, but linked after it: a map with a live design session makes `designs
	// open` return that one instead of starting another.
	plain := make(map[string]string)
	for _, addr := range sc.LinkedDesigns {
		id, err := sessionsOf(f).createSession(project, filepath.Join(f.root, "repo"), "hello", "")
		if err != nil {
			return err
		}
		plain[addr] = id
	}
	for _, addr := range sc.OpenDesigns {
		if err := f.openDesign(ctx, sc, addr); err != nil {
			return err
		}
	}
	for addr, id := range plain {
		if err := f.linkSession(id, addr); err != nil {
			return err
		}
	}
	return nil
}

func (f *fixture) openDesign(ctx context.Context, sc scenario, addr string) error {
	out, err := f.cli("designs", "open", addr, "--json")
	if err != nil {
		return fmt.Errorf("open design %s: %w\n%s", addr, err, out)
	}
	var res wire.SessionResult
	if err := json.Unmarshal(out, &res); err != nil || res.Session == nil {
		return fmt.Errorf("open design %s: no session in %s", addr, out)
	}
	// The step moves while the opening turn still runs, so that turn's end finds the map past its
	// step and the handoff fires. After the wait the run end would already be behind it.
	if step, ok := sc.AdvanceSteps[addr]; ok {
		if out, err := f.cli("tickets", "set", addr, "--design-step", step); err != nil {
			return fmt.Errorf("advance %s to %s: %w\n%s", addr, step, err, out)
		}
	}
	// The opening prompt is still running when `designs open` returns. A control the test sends
	// meanwhile (Block) is overwritten when that turn ends.
	if err := sessionsOf(f).waitFor(res.Session.ID, "past its opening turn", func(si wire.SessionInfo, _ wire.SessionChatResponse) bool {
		return si.Status.State != wire.SessionStarting && si.Status.State != wire.SessionWorking
	}); err != nil {
		return fmt.Errorf("open design %s: %w", addr, err)
	}
	if slices.Contains(sc.AutoAdvanceOff, addr) {
		// Not the CLI verb: its on|off arg validation rejects the session id.
		if res, err := apiclient.New(server.SocketPath(f.stateD)).Sessions.SetAutoAdvance(ctx, res.Session.ID, false); err != nil || res.Refused {
			return fmt.Errorf("auto-advance off for %s: %v %+v", addr, err, res)
		}
	}
	return nil
}

// buildOutput assembles what the fixture prints.
func (f *fixture) buildOutput(sc scenario, rs *runningServer, s seeded) Output {
	res := Output{WebURL: rs.webURL, CookieName: rs.in.CookieName, Cookie: rs.in.Cookie, Password: rs.password, TOTPSecret: rs.in.TOTPSecret, TOTPStep: rs.in.TOTPStep, StateDir: f.stateD, Socket: server.SocketPath(f.stateD), HoldFile: f.hold, Env: f.cliEnv, Gx: f.gx, DiskDir: f.diskDir(), HealthDir: f.healthDir(), LogFile: rs.logPath, AgentTranscript: s.agentTranscript, CheckWaitingFile: s.checkWaitingFile}
	f.addScenarioOutput(&res, sc, s.transcripts)
	return res
}

func printOutput(o opts, res Output) error {
	if o.shellEnv {
		fmt.Print(res.ShellEnv())
		return nil
	}
	out, err := json.Marshal(res)
	if err != nil {
		return err
	}
	fmt.Println(string(out))
	return nil
}

// waitForEnd blocks until the fixture is told to stop, its parent dies or the TTL passes.
// A server that exits on its own is an error unless the scenario stops it itself.
func (f *fixture) waitForEnd(ctx context.Context, sc scenario, exited <-chan error) error {
	lifeCtx, cancelLife := context.WithCancel(ctx)
	defer cancelLife()
	ended := make(chan string, 1)
	go func() { ended <- watchLifetime(lifeCtx, os.Getppid(), os.Getppid, lifetimeTTL, parentPoll) }()

	select {
	case <-ctx.Done():
		return nil
	case <-ended:
		return nil
	case err := <-exited:
		// A migrate scenario's flow kills and starts the server itself (migrate.go);
		// a cache-gate flow stops it to run gx brief go-clean-cache without one.
		if sc.Restart == nil && !sc.Migrate && !sc.FakeGo {
			return fmt.Errorf("server exited: %v\n%s", err, f.serverOutput())
		}
		f.restarted = true
		select {
		case <-ctx.Done():
		case <-ended:
		}
		return nil
	}
}

// runShutdown joins the shutdown error onto err.
func (f *fixture) runShutdown(err *error, rs *runningServer) {
	*err = errors.Join(*err, f.shutdown(rs.cmd, rs.exited))
}
