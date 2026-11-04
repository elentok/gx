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
	"github.com/elentok/gx/migrations"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "gxfixture:", err)
		os.Exit(1)
	}
}

func run(args []string) (err error) {
	var gxBin, fakeBin, fakeCodexBin, name, autoAdvance, commandListAge, stallThreshold, mockDir, shape string
	var configs []string
	var hold, shellEnv, hideCodex bool
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--hide-codex":
			hideCodex = true
		case "--hold":
			hold = true
		case "--env":
			shellEnv = true
		case "--auto-advance-delay":
			if i+1 == len(args) {
				return fmt.Errorf("%s needs a value", args[i])
			}
			autoAdvance = args[i+1]
			i++
		case "--command-list-age":
			if i+1 == len(args) {
				return fmt.Errorf("%s needs a value", args[i])
			}
			commandListAge = args[i+1]
			i++
		case "--stall-threshold":
			if i+1 == len(args) {
				return fmt.Errorf("%s needs a value", args[i])
			}
			stallThreshold = args[i+1]
			i++
		case "--config", "--mock-dir", "--shape":
			if i+1 == len(args) {
				return fmt.Errorf("%s needs a value", args[i])
			}
			switch args[i] {
			case "--config":
				configs = append(configs, args[i+1])
			case "--shape":
				shape = args[i+1]
			default:
				mockDir = args[i+1]
			}
			i++
		case "--gx", "--fakeclaude", "--fakecodex":
			if i+1 == len(args) {
				return fmt.Errorf("%s needs a value", args[i])
			}
			switch args[i] {
			case "--gx":
				gxBin = args[i+1]
			case "--fakeclaude":
				fakeBin = args[i+1]
			default:
				fakeCodexBin = args[i+1]
			}
			i++
		default:
			name = args[i]
		}
	}
	sc, ok := scenarios[name]
	if !ok {
		return fmt.Errorf("unknown scenario %q; known: %s", name, strings.Join(scenarioNames(), ", "))
	}
	if shape, err = checkShape(sc, shape); err != nil {
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
	if gxBin, err = binary(gxBin, filepath.Join(root, "gx"), "github.com/elentok/gx"); err != nil {
		return err
	}
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return err
	}
	if fakeBin, err = binary(fakeBin, filepath.Join(binDir, "claude"), "github.com/elentok/gx/testutil/agentfake/cmd/fakeclaude"); err != nil {
		return err
	}
	if filepath.Base(fakeBin) != "claude" || filepath.Dir(fakeBin) != binDir {
		if err := os.Symlink(fakeBin, filepath.Join(binDir, "claude")); err != nil {
			return err
		}
	}
	// Without the flag codex stays off PATH, which is how a host without Codex looks. A scenario that
	// needs codex has it anyway; --hide-codex takes it off PATH once the scenario is seeded.
	if fakeCodexBin != "" || sc.codex {
		if fakeCodexBin, err = binary(fakeCodexBin, filepath.Join(root, "codex"), "github.com/elentok/gx/testutil/agentfake/cmd/fakecodex"); err != nil {
			return err
		}
		if err := os.Symlink(fakeCodexBin, filepath.Join(binDir, "codex")); err != nil {
			return err
		}
	}
	installedCodex := fakeCodexBin != ""
	// Agent hooks run a bare `gx`; it must be the build under test, not the host's.
	if err := os.Symlink(gxBin, filepath.Join(binDir, "gx")); err != nil {
		return err
	}

	port, err := freePort()
	if err != nil {
		return err
	}
	f := &fixture{
		root:   root,
		gx:     gxBin,
		hold:   filepath.Join(root, "hold"),
		store:  filepath.Join(root, "data", "gx", "tickets"),
		stateD: filepath.Join(root, "state", "gx"),
		codex:  installedCodex,
	}
	f.env, f.cliEnv = isolatedEnv(root, binDir)
	if autoAdvance != "" {
		if _, err := time.ParseDuration(autoAdvance); err != nil {
			return fmt.Errorf("--auto-advance-delay: %w", err)
		}
		f.env = append(f.env, "GX_AUTO_ADVANCE_DELAY="+autoAdvance)
	}
	if commandListAge != "" {
		if _, err := time.ParseDuration(commandListAge); err != nil {
			return fmt.Errorf("--command-list-age: %w", err)
		}
		f.env = append(f.env, "GX_COMMAND_LIST_AGE="+commandListAge)
	}
	if sc.commands != nil {
		data, err := json.Marshal(sc.commands)
		if err != nil {
			return err
		}
		path := filepath.Join(root, "agentfake-commands.json")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return err
		}
		f.env = append(f.env, agentfake.CommandsFileEnv+"="+path)
	}
	if stallThreshold == "" {
		stallThreshold = sc.stallThreshold
	}
	if stallThreshold != "" {
		if _, err := time.ParseDuration(stallThreshold); err != nil {
			return fmt.Errorf("--stall-threshold: %w", err)
		}
		f.env = append(f.env, "GX_STALL_THRESHOLD="+stallThreshold)
	}
	if sc.dropAskedAgent {
		f.env = append(f.env, "GX_DROP_ASKED_AGENT=1")
	}
	if sc.unknownSkill {
		f.env = append(f.env, agentfake.TUIUnknownSkillEnv+"=1")
	}
	// Later entries win over the harness default of a ready signal at once.
	if sc.readyDelay != "" {
		f.env = append(f.env, agentfake.TUIReadyDelayEnv+"="+sc.readyDelay)
	}
	if sc.readyWait != "" {
		f.env = append(f.env, "GX_PTY_READY_WAIT="+sc.readyWait)
	}
	f.env = append(f.env, sc.env...)
	if err := f.setupDisk(); err != nil {
		return err
	}
	if err := f.setupHealth(); err != nil {
		return err
	}
	// Any scenario that cleans needs the fake ps: the real one sees the `go test` running this fixture.
	if sc.fakeGo || sc.diskLow {
		if err := f.setupFakeGo(); err != nil {
			return err
		}
	}
	if sc.restart != nil {
		if err := f.setupRestart(*sc.restart, sc.projectNames()); err != nil {
			return err
		}
	}
	if sc.migrate {
		if err := f.setupMigrate(sc.migrateHold); err != nil {
			return err
		}
	}
	if err := f.writeConfig(port, sc.recentlyFinished, append(slices.Clone(sc.config), configs...)); err != nil {
		return err
	}
	if err := f.seed(sc); err != nil {
		return err
	}
	// The server migrates on every start. The seed is already in the current
	// shape, so only the migrate scenarios leave steps pending.
	if !sc.migrate {
		for _, p := range sc.projectNames() {
			if err := migrations.WriteFullApplied(filepath.Join(f.store, p)); err != nil {
				return err
			}
		}
	}
	// The migration's steps read the store's git log.
	if err := f.commitStore(); err != nil {
		return fmt.Errorf("commit store: %w", err)
	}
	if hideCodex {
		if err := os.Remove(filepath.Join(binDir, "codex")); err != nil {
			return err
		}
	}
	// The running server has no test clock, so the old logins are written first.
	if len(sc.extraLogins) > 0 {
		if err := os.MkdirAll(f.stateD, 0o700); err != nil {
			return err
		}
	}
	for i, ago := range sc.extraLogins {
		if _, err := server.SeedWebLogin(f.stateD, fmt.Sprintf("gxfixture browser %d", i+1), time.Now().Add(-ago)); err != nil {
			return fmt.Errorf("seed web login: %w", err)
		}
	}

	if sc.limits {
		if err := f.seedLimits(shape); err != nil {
			return fmt.Errorf("seed limits: %w", err)
		}
	}
	if sc.limitBurn {
		if err := f.seedLimitBurn(shape); err != nil {
			return fmt.Errorf("seed limit burn: %w", err)
		}
	}
	if sc.rateLimitFromUsage {
		f.env = append(f.env, "GX_RATE_LIMIT_BUFFER=1s")
	}
	if sc.promptGrace != "" {
		f.env = append(f.env, "GX_PROMPT_GRACE="+sc.promptGrace)
	}
	if sc.tokenPoll {
		f.env = append(f.env, "GX_TOKEN_POLL_INTERVAL="+liveTokenPollInterval)
	}
	if sc.spend {
		if err := f.seedSpend(shape); err != nil {
			return fmt.Errorf("seed spend: %w", err)
		}
		f.env = append(f.env, "GX_BUDGET_POLL_INTERVAL="+spendPollInterval)
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
	if hold || sc.hold {
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
	o := Output{WebURL: webURL, CookieName: in.cookieName, Cookie: in.cookie, Password: password, TOTPSecret: in.totpSecret, TOTPStep: in.totpStep, StateDir: f.stateD, Socket: server.SocketPath(f.stateD), HoldFile: f.hold, Env: f.cliEnv, Gx: gxBin, DiskDir: f.diskDir(), HealthDir: f.healthDir(), LogFile: logPath, AgentTranscript: agentTranscript, CheckWaitingFile: checkWaitingFile}
	if sc.fakeGo {
		o.GateDir = f.gateDir()
	}
	o.Transcripts = transcripts
	if sc.limitBurn {
		o.CodexSessionsDir = filepath.Join(root, "home", ".codex", "sessions")
	}
	if sc.restart != nil {
		o.RestartDir = f.restartDir()
	}
	if sc.migrate {
		o.MigrateDir = f.migrateDir()
	}
	if sc.commands != nil {
		o.FeatureWorktree = filepath.Join(f.root, "repo", ".worktrees", "feature")
	}
	if cwd, err := os.Getwd(); err == nil {
		o.BrowserSession = filepath.Base(cwd)
	}
	if mockDir != "" {
		stopMock, mockURL, err := serveMock(mockDir)
		if err != nil {
			return err
		}
		defer stopMock()
		o.MockURL = mockURL
	}
	if shellEnv {
		fmt.Print(o.ShellEnv())
	} else {
		out, err := json.Marshal(o)
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
