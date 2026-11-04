// Command gxfixture starts one fresh, isolated gx server for a named scenario
// and prints, as one line of JSON, how to reach it. It stops on SIGTERM and
// removes everything it made. It also stops when its parent process dies or
// after two hours, and it runs the server with OS notifications off. It never reads or writes the real HOME, config
// or state: the server and every CLI verb run in a temp HOME and temp XDG dirs.
//
//	gxfixture [--gx <gx binary>] [--fakeclaude <binary>] [--fakecodex <binary>] [--auto-advance-delay <duration>] [--command-list-age <duration>] [--stall-threshold <duration>] <scenario>
//
// --auto-advance-delay shortens the server's run-end countdown (default 10s).
// --command-list-age sets how long the server serves a command list from its cache (GX_COMMAND_LIST_AGE).
// --stall-threshold shortens how long a Working session's transcript may stay untouched before it counts as stalled (default 15m).
// --shape <fresh|stale|past-reset|one-window> picks the seeded limits state of the limits scenario (default fresh).
// --shape <normal|short|empty> picks the seeded budget ledger of the spend scenario (default normal).
// --shape <menu|few-days|stale|past-reset|stats> picks the seeded limit history of the limit-burn scenario (default menu).
// --env prints `export` lines (see Output.ShellEnv) instead of the JSON, so a shell can source them.
// --config <dotted.key>=<json> sets a config key, e.g. --config server.web.foo=true; repeatable.
// --mock-dir <path> serves that directory over loopback HTTP and reports its base URL as mock_url.
//
// --fakecodex installs that binary as `codex` next to `claude`; without it codex is not on PATH.
//
// A binary left out is built with `go build -tags gxfixture` into the fixture's own temp dir, which
// needs the module as the working directory. So `go run ./testutil/gxfixture <scenario>` is the
// one-step build-and-start; nothing is shared with a sibling worktree's run.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/testutil"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "gxfixture:", err)
		os.Exit(1)
	}
}

func run(args []string) (err error) {
	o, err := parseArgs(args)
	if err != nil {
		return err
	}
	sc, ok := scenarios[o.scenario]
	if !ok {
		return fmt.Errorf("unknown scenario %q; known: %s", o.scenario, strings.Join(scenarioNames(), ", "))
	}
	shape, err := checkShape(sc, o.shape)
	if err != nil {
		return err
	}
	if sc.limitBurn && shape == "live" {
		sc = limitBurnLive()
	}

	root, err := testutil.ShortTempDir("gxf")
	if err != nil {
		return err
	}
	defer removeAll(root)
	// Caught from here on, so a Ctrl-C or kill while building or seeding still
	// unwinds through the removeAll above instead of leaking the root.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// /tmp is a symlink on macOS: claude slugs transcripts by the real path, so
	// record dirs must use it too.
	if root, err = filepath.EvalSymlinks(root); err != nil {
		return err
	}
	gxBin, binDir, codex, err := installBinaries(root, o, sc)
	if err != nil {
		return err
	}

	port, err := freePort()
	if err != nil {
		return err
	}
	f := newFixture(root, gxBin, binDir, codex)
	if err := f.applyScenarioEnv(o, sc); err != nil {
		return err
	}
	if err := f.prepareStore(sc, port, o.configs); err != nil {
		return err
	}
	if o.hideCodex {
		if err := os.Remove(filepath.Join(binDir, "codex")); err != nil {
			return err
		}
	}
	if err := f.seedBeforeStart(sc, shape); err != nil {
		return err
	}

	srv := exec.Command(gxBin, "server")
	srv.Env = f.env
	logPath := filepath.Join(root, "server.out")
	logFile, err := os.Create(logPath)
	if err != nil {
		return err
	}
	defer logFile.Close()
	srv.Stdout, srv.Stderr = logFile, logFile
	if err := srv.Start(); err != nil {
		return err
	}
	if sc.migrate {
		if err := f.writeServerControl(srv.Process.Pid, logPath); err != nil {
			_ = srv.Process.Kill()
			return err
		}
	}
	exited := make(chan error, 1)
	go func() {
		defer close(exited)
		err := srv.Wait()
		select {
		case exited <- err:
		case <-ctx.Done():
		}
	}()
	defer func() { err = errors.Join(err, f.shutdown(srv, exited)) }()

	webURL, setupCode, err := f.waitReady(ctx, exited)
	if err != nil {
		return fmt.Errorf("%w\n%s", err, f.serverOutput())
	}
	var in signIn
	password := ""
	if sc.web != webSetupPending {
		if in, err = webSetup(f.stateD, webURL, setupCode); err != nil {
			return fmt.Errorf("web setup: %w\n%s", err, f.serverOutput())
		}
		password = fixturePassword
	}
	if sc.web == webSignedOut {
		in.cookieName, in.cookie = "", ""
	}
	// The earlier run goes first so no held worker holds its slot.
	for _, addr := range sc.ranOnClaude {
		if err := f.seedRanOnClaude(addr); err != nil {
			return err
		}
	}
	// --hold freezes every worker as running until the caller removes the hold file.
	if o.hold || sc.hold {
		if err := os.WriteFile(f.hold, nil, 0o644); err != nil {
			return err
		}
	}
	queueAll := func() error {
		for _, addr := range sc.queue {
			args := []string{"server", "queue", "add", addr}
			if agent := sc.queueAgents[addr]; agent != "" {
				args = append(args, "--agent", agent)
			}
			// A held migration holds the queue verb too; it finishes once the hold is released.
			if sc.migrateHold {
				cmd := exec.CommandContext(ctx, f.gx, args...)
				cmd.Env = f.env
				if err := cmd.Start(); err != nil {
					return fmt.Errorf("queue %s: %w", addr, err)
				}
				go func() { _ = cmd.Wait() }()
				continue
			}
			if out, err := f.cli(args...); err != nil {
				return fmt.Errorf("queue %s: %w\n%s", addr, err, out)
			}
		}
		return nil
	}
	if !sc.queueAfterSessions {
		if err := queueAll(); err != nil {
			return err
		}
	}
	if len(sc.queueAfterUsage) > 0 {
		sampleFile := "usage-sample.json"
		if sc.codexUsage {
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
		for _, addr := range sc.queueAfterUsage {
			args := []string{"server", "queue", "add", addr}
			if agent := sc.queueAgents[addr]; agent != "" {
				args = append(args, "--agent", agent)
			}
			if out, err := f.cli(args...); err != nil {
				return fmt.Errorf("queue %s: %w\n%s", addr, err, out)
			}
		}
	}
	if sc.agentProcs {
		if err := f.tieAgentProcesses(ctx); err != nil {
			return err
		}
	}
	agentTranscript := ""
	if sc.spend {
		if agentTranscript, err = f.agentTranscript(ctx); err != nil {
			return err
		}
	}
	checkWaitingFile := ""
	if sc.checkWaiting {
		if checkWaitingFile, err = f.writeCheckWaiting(ctx); err != nil {
			return err
		}
	}
	if err := f.seedSessions(sc); err != nil {
		return err
	}
	if sc.queueAfterSessions {
		if err := queueAll(); err != nil {
			return err
		}
	}
	// Plain sessions are made before openDesigns, which then starts the newer
	// run, but linked after it: a map with a live design session makes `designs
	// open` return that one instead of starting another.
	plain := make(map[string]string)
	for _, addr := range sc.linkedDesigns {
		id, err := f.createSession(project, filepath.Join(f.root, "repo"), "hello", "")
		if err != nil {
			return err
		}
		plain[addr] = id
	}
	for _, addr := range sc.openDesigns {
		out, err := f.cli("designs", "open", addr, "--json")
		if err != nil {
			return fmt.Errorf("open design %s: %w\n%s", addr, err, out)
		}
		var res server.SessionResult
		if err := json.Unmarshal(out, &res); err != nil || res.Session == nil {
			return fmt.Errorf("open design %s: no session in %s", addr, out)
		}
		// The step moves while the opening turn still runs, so that turn's end finds the map past its
		// step and the handoff fires. After the wait the run end would already be behind it.
		if step, ok := sc.advanceSteps[addr]; ok {
			if out, err := f.cli("tickets", "set", addr, "--design-step", step); err != nil {
				return fmt.Errorf("advance %s to %s: %w\n%s", addr, step, err, out)
			}
		}
		// The opening prompt is still running when `designs open` returns. A control the test sends
		// meanwhile (Block) is overwritten when that turn ends.
		if err := f.waitFor(res.Session.ID, "past its opening turn", func(si server.SessionInfo, _ server.SessionChatResponse) bool {
			return si.Status.State != server.SessionStarting && si.Status.State != server.SessionWorking
		}); err != nil {
			return fmt.Errorf("open design %s: %w", addr, err)
		}
		if slices.Contains(sc.autoAdvanceOff, addr) {
			// Not the CLI verb: its on|off arg validation rejects the session id.
			if res, err := apiclient.New(server.SocketPath(f.stateD)).SetAutoAdvance(ctx, res.Session.ID, false); err != nil || res.Refused {
				return fmt.Errorf("auto-advance off for %s: %v %+v", addr, err, res)
			}
		}
	}
	for addr, id := range plain {
		if err := f.linkSession(id, addr); err != nil {
			return err
		}
	}
	var transcripts map[string]string
	if sc.tokenPoll {
		if transcripts, err = f.liveTranscripts(ctx); err != nil {
			return err
		}
	}
	res := Output{WebURL: webURL, CookieName: in.cookieName, Cookie: in.cookie, Password: password, TOTPSecret: in.totpSecret, TOTPStep: in.totpStep, StateDir: f.stateD, Socket: server.SocketPath(f.stateD), HoldFile: f.hold, Env: f.cliEnv, Gx: gxBin, DiskDir: f.diskDir(), HealthDir: f.healthDir(), LogFile: logPath, AgentTranscript: agentTranscript, CheckWaitingFile: checkWaitingFile}
	f.addScenarioOutput(&res, sc, transcripts)
	if o.mockDir != "" {
		stopMock, mockURL, err := serveMock(o.mockDir)
		if err != nil {
			return err
		}
		defer stopMock()
		res.MockURL = mockURL
	}
	if o.shellEnv {
		fmt.Print(res.ShellEnv())
	} else {
		out, err := json.Marshal(res)
		if err != nil {
			return err
		}
		fmt.Println(string(out))
	}

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
		if sc.restart == nil && !sc.migrate && !sc.fakeGo {
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

// serveMock serves dir on a loopback port (`file://` is blocked in the
// browser) and returns its base URL and a stop func.
func serveMock(dir string) (stop func(), url string, err error) {
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return nil, "", fmt.Errorf("--mock-dir %q: not a directory", dir)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, "", err
	}
	srv := &http.Server{Handler: http.FileServer(http.Dir(dir))}
	go func() { _ = srv.Serve(ln) }()
	return func() { _ = srv.Close() }, "http://" + ln.Addr().String() + "/", nil
}

const (
	// lifetimeTTL caps a fixture a caller forgot to stop.
	lifetimeTTL = 2 * time.Hour
	parentPoll  = time.Second
)

// watchLifetime blocks until the fixture should stop: "parent" when the
// process that started it is gone (its pid changes, as the kernel reparents an
// orphan), "ttl" after ttl, or "" when ctx ends first. A fixture started with
// nohup or & outlives the shell that started it, so the parent check is what
// stops a leaked one.
func watchLifetime(ctx context.Context, parent int, getppid func() int, ttl, poll time.Duration) string {
	timer := time.NewTimer(ttl)
	defer timer.Stop()
	tick := time.NewTicker(poll)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return ""
		case <-timer.C:
			return "ttl"
		case <-tick.C:
			if getppid() != parent {
				return "parent"
			}
		}
	}
}

type fixture struct {
	root, gx, hold, store, stateD string
	// env is what the server and its agents run with; cliEnv is the part a
	// CLI verb needs to reach the same server, which is all the output names.
	env, cliEnv []string
	// codex is true when the fake codex is on the fixture PATH.
	codex bool
	// restarted is true once the server exited on a web restart: the fixture stays up
	// for the server that restart started, and stops that one at shutdown.
	restarted bool
}

// waitReady polls until the server answers with its web URL and Setup code.
func (f *fixture) waitReady(ctx context.Context, exited <-chan error) (webURL, setupCode string, err error) {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-exited:
			return "", "", fmt.Errorf("server exited before it answered: %v", err)
		case <-ctx.Done():
			return "", "", ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
		out, err := f.cli("server", "status", "--json")
		if err != nil {
			continue
		}
		var st struct {
			Running      bool   `json:"running"`
			WebURL       string `json:"web_url"`
			WebSetupCode string `json:"web_setup_code"`
		}
		if json.Unmarshal(out, &st) == nil && st.Running && st.WebURL != "" && st.WebSetupCode != "" {
			plain, _, _ := strings.Cut(st.WebURL, "?")
			return strings.TrimSuffix(plain, "/"), st.WebSetupCode, nil
		}
	}
	return "", "", errors.New("server did not answer within 30s")
}

func (f *fixture) serverOutput() string {
	out, _ := os.ReadFile(filepath.Join(f.root, "server.out"))
	return string(out)
}
