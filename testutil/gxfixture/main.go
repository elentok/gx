// Command gxfixture starts one fresh, isolated gx server for a named scenario
// and prints, as one line of JSON, how to reach it. It stops on SIGTERM and
// removes everything it made. It also stops when its parent process dies or
// after two hours, and it runs the server with OS notifications off. It never reads or writes the real HOME, config
// or state: the server and every CLI verb run in a temp HOME and temp XDG dirs.
//
//	gxfixture [--gx <gx binary>] [--fakeclaude <binary>] [--fakecodex <binary>] [--auto-advance-delay <duration>] [--stall-threshold <duration>] <scenario>
//
// --auto-advance-delay shortens the server's run-end countdown (default 10s).
// --stall-threshold shortens how long a Working session's transcript may stay untouched before it counts as stalled (default 15m).
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
	"runtime"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/osnotify"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// Output is the JSON gxfixture prints once the server answers.
type Output struct {
	// WebURL is the plain URL: it signs nobody in. Cookie is the web login's
	// cookie value under CookieName, from the setup the fixture finished; it is
	// empty in the web-setup and web-login scenarios, where the page starts signed out.
	// Password is the web password, empty while the setup is pending.
	WebURL     string `json:"web_url"`
	CookieName string `json:"cookie_name"`
	Cookie     string `json:"cookie"`
	Password   string `json:"password,omitempty"`
	// TOTPSecret and TOTPStep (the last step used) are left out when the server
	// runs with server.web.totp=false.
	TOTPSecret string `json:"totp_secret,omitempty"`
	TOTPStep   int64  `json:"totp_step,omitempty"`
	StateDir   string `json:"state_dir"`
	Socket     string `json:"socket"`
	HoldFile   string `json:"hold_file"`
	// Env is what a CLI verb needs to reach this server and no other.
	Env []string `json:"env"`
	// Gx is the gx binary the server runs.
	Gx string `json:"gx"`
	// DiskDir holds the fake disk's control files: free and clean.
	DiskDir string `json:"disk_dir"`
	// HealthDir holds the fake CPU and memory control file: system.
	HealthDir string `json:"health_dir"`
	// LogFile is the server's combined stdout and stderr, which carries the cleanup
	// command's output tail ("disk cleanup command failed") and every server warning.
	LogFile string `json:"log_file"`
	// MockURL is the base URL serving --mock-dir; empty without the flag.
	MockURL string `json:"mock_url,omitempty"`
	// BrowserSession names this worktree's playwright-cli session (`-s=<name>`): the
	// worktree directory is unique per sibling session and stable across calls, so
	// concurrent sessions never share a browser.
	BrowserSession string `json:"browser_session,omitempty"`
}

// ShellEnv is the output as `export` lines a shell can source: the CLI env, then
// GXF_* variables for the rest. Values are single-quoted.
func (o Output) ShellEnv() string {
	var b strings.Builder
	export := func(k, v string) { fmt.Fprintf(&b, "export %s='%s'\n", k, strings.ReplaceAll(v, "'", `'\''`)) }
	for _, kv := range o.Env {
		k, v, _ := strings.Cut(kv, "=")
		export(k, v)
	}
	export("GXF_WEB_URL", o.WebURL)
	export("GXF_WEB_COOKIE", o.CookieName+"="+o.Cookie)
	export("GXF_GX", o.Gx)
	export("GXF_DISK_DIR", o.DiskDir)
	export("GXF_HEALTH_DIR", o.HealthDir)
	export("GXF_HOLD_FILE", o.HoldFile)
	export("GXF_STATE_DIR", o.StateDir)
	export("GXF_SOCKET", o.Socket)
	if o.LogFile != "" {
		export("GXF_LOG_FILE", o.LogFile)
	}
	if o.MockURL != "" {
		export("GXF_MOCK_URL", o.MockURL)
	}
	if o.BrowserSession != "" {
		export("GXF_BROWSER_SESSION", o.BrowserSession)
	}
	return b.String()
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "gxfixture:", err)
		os.Exit(1)
	}
}

func run(args []string) (err error) {
	var gxBin, fakeBin, fakeCodexBin, name, autoAdvance, stallThreshold, mockDir string
	var configs []string
	var hold, shellEnv bool
	for i := 0; i < len(args); i++ {
		switch args[i] {
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
		case "--stall-threshold":
			if i+1 == len(args) {
				return fmt.Errorf("%s needs a value", args[i])
			}
			stallThreshold = args[i+1]
			i++
		case "--config", "--mock-dir":
			if i+1 == len(args) {
				return fmt.Errorf("%s needs a value", args[i])
			}
			if args[i] == "--config" {
				configs = append(configs, args[i+1])
			} else {
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

	// Unix socket paths are capped near 100 bytes, and a holder's socket sits
	// deep in the state dir, so the root stays short: macOS's TMPDIR is too long.
	root, err := os.MkdirTemp("/tmp", "gxf")
	if err != nil {
		return err
	}
	defer removeAll(root)
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
	// Without the flag codex stays off PATH, which is how a host without Codex looks.
	if fakeCodexBin != "" || sc.codex {
		if fakeCodexBin, err = binary(fakeCodexBin, filepath.Join(root, "codex"), "github.com/elentok/gx/testutil/agentfake/cmd/fakecodex"); err != nil {
			return err
		}
		if err := os.Symlink(fakeCodexBin, filepath.Join(binDir, "codex")); err != nil {
			return err
		}
	}
	installedCodex := fakeCodexBin != ""

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
	if stallThreshold == "" {
		stallThreshold = sc.stallThreshold
	}
	if stallThreshold != "" {
		if _, err := time.ParseDuration(stallThreshold); err != nil {
			return fmt.Errorf("--stall-threshold: %w", err)
		}
		f.env = append(f.env, "GX_STALL_THRESHOLD="+stallThreshold)
	}
	if err := f.setupDisk(); err != nil {
		return err
	}
	if err := f.setupHealth(); err != nil {
		return err
	}
	if err := f.writeConfig(port, sc.recentlyFinished, configs); err != nil {
		return err
	}
	if err := f.seed(sc); err != nil {
		return err
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
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
	// --hold freezes every worker as running until the caller removes the hold file.
	if hold || sc.hold {
		if err := os.WriteFile(f.hold, nil, 0o644); err != nil {
			return err
		}
	}
	for _, addr := range sc.queue {
		if out, err := f.cli("server", "queue", "add", addr); err != nil {
			return fmt.Errorf("queue %s: %w\n%s", addr, err, out)
		}
	}
	if len(sc.queueAfterUsage) > 0 {
		sample := filepath.Join(f.root, "state", "gx", "usage-sample.json")
		deadline := time.Now().Add(30 * time.Second)
		for _, err := os.Stat(sample); err != nil; _, err = os.Stat(sample) {
			if time.Now().After(deadline) {
				return fmt.Errorf("no usage sample at %s", sample)
			}
			time.Sleep(100 * time.Millisecond)
		}
		for _, addr := range sc.queueAfterUsage {
			if out, err := f.cli("server", "queue", "add", addr); err != nil {
				return fmt.Errorf("queue %s: %w\n%s", addr, err, out)
			}
		}
	}
	if sc.agentProcs {
		if err := f.tieAgentProcesses(ctx); err != nil {
			return err
		}
	}
	if err := f.seedSessions(sc); err != nil {
		return err
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
		if slices.Contains(sc.autoAdvanceOff, addr) {
			var res server.SessionResult
			if err := json.Unmarshal(out, &res); err != nil || res.Session == nil {
				return fmt.Errorf("open design %s: no session in %s", addr, out)
			}
			// Not the CLI verb: its on|off arg validation rejects the session id.
			if res, err := apiclient.New(server.SocketPath(f.stateD)).SetAutoAdvance(ctx, res.Session.ID, false); err != nil || res.Refused {
				return fmt.Errorf("auto-advance off for %s: %v %+v", addr, err, res)
			}
		}
	}
	for addr, step := range sc.advanceSteps {
		if out, err := f.cli("tickets", "set", addr, "--design-step", step); err != nil {
			return fmt.Errorf("advance %s to %s: %w\n%s", addr, step, err, out)
		}
	}
	for addr, id := range plain {
		if err := f.linkSession(id, addr); err != nil {
			return err
		}
	}
	o := Output{WebURL: webURL, CookieName: in.cookieName, Cookie: in.cookie, Password: password, TOTPSecret: in.totpSecret, TOTPStep: in.totpStep, StateDir: f.stateD, Socket: server.SocketPath(f.stateD), HoldFile: f.hold, Env: f.cliEnv, Gx: gxBin, DiskDir: f.diskDir(), HealthDir: f.healthDir(), LogFile: logPath}
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
		return fmt.Errorf("server exited: %v\n%s", err, f.serverOutput())
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
}

// isolatedEnv is the process environment with every variable that could reach
// the real HOME, config, state or Claude config replaced, and the ones that
// make a nested agent act differently dropped.
func isolatedEnv(root, binDir string) (env, cliEnv []string) {
	drop := map[string]bool{"HOME": true, "PATH": true, "GIT_CONFIG_GLOBAL": true, "AI_AGENT": true}
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if !drop[k] && !strings.HasPrefix(k, "XDG_") && !strings.HasPrefix(k, "GX_") &&
			!strings.HasPrefix(k, "CLAUDE") && !strings.HasPrefix(k, "HERDR_") {
			env = append(env, kv)
		}
	}
	cliEnv = []string{
		"HOME=" + filepath.Join(root, "home"),
		"XDG_DATA_HOME=" + filepath.Join(root, "data"),
		"XDG_STATE_HOME=" + filepath.Join(root, "state"),
		"XDG_CONFIG_HOME=" + filepath.Join(root, "config"),
	}
	// HOME is replaced, so Playwright would look for its browsers in the empty
	// temp HOME: name the real cache.
	if p := playwrightBrowsersPath(); p != "" {
		cliEnv = append(cliEnv, "PLAYWRIGHT_BROWSERS_PATH="+p)
	}
	env = append(env, cliEnv...)
	env = append(env,
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		osnotify.OffEnv+"=off",
		"GIT_AUTHOR_NAME=gxfixture", "GIT_AUTHOR_EMAIL=gxfixture@example.com",
		"GIT_COMMITTER_NAME=gxfixture", "GIT_COMMITTER_EMAIL=gxfixture@example.com",
	)
	return git.WithMaintenanceOff(env), cliEnv
}

// playwrightBrowsersPath is the caller's PLAYWRIGHT_BROWSERS_PATH, else
// Playwright's default cache under the real home ("" when it cannot be told).
func playwrightBrowsersPath() string {
	if p := os.Getenv("PLAYWRIGHT_BROWSERS_PATH"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Caches", "ms-playwright")
	}
	return filepath.Join(home, ".cache", "ms-playwright")
}

// writeConfig writes the fixture's config. overrides are `dotted.key=<json>`
// entries (a value that is not JSON is a string) set over the defaults.
func (f *fixture) writeConfig(port, recentlyFinished int, overrides []string) error {
	dir := filepath.Join(f.root, "home", ".config", "gx")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	server := map[string]any{"watch-config": false, "web": map[string]any{"listen": fmt.Sprintf("127.0.0.1:%d", port)}}
	if recentlyFinished > 0 {
		server["recently-finished"] = recentlyFinished
	}
	// A wait shorter than a loaded fake's first turn reads as an undelivered
	// prompt: the runner kills the agent mid-finish and relaunches it.
	cfg := map[string]any{"agent-runner": "pty", "runner-wait": "2s", "server": server}
	for _, kv := range overrides {
		key, raw, ok := strings.Cut(kv, "=")
		if !ok || key == "" {
			return fmt.Errorf("--config %q: want key=value", kv)
		}
		var val any
		if json.Unmarshal([]byte(raw), &val) != nil {
			val = raw
		}
		node := cfg
		parts := strings.Split(key, ".")
		for _, p := range parts[:len(parts)-1] {
			next, ok := node[p].(map[string]any)
			if !ok {
				next = map[string]any{}
				node[p] = next
			}
			node = next
		}
		node[parts[len(parts)-1]] = val
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "config.json"), data, 0o644)
}

// seed writes the scenario's project repo (carrying the fake worker's config,
// which the worktrees inherit) and its tickets.
func (f *fixture) seed(sc scenario) error {
	repo := filepath.Join(f.root, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		return err
	}
	holdTimeout := "30s"
	if sc.hold {
		holdTimeout = "1h"
	}
	wcfg := agentfake.WorkerConfig{Gx: f.gx, Env: f.cliEnv, Hold: f.hold, HoldTimeout: holdTimeout, Behaviour: sc.behaviour, Design: sc.design, DesignByMap: sc.designByMap, Chat: sc.chat}
	if sc.usage {
		resets := time.Now().Add(72 * time.Hour).Unix()
		wcfg.Usage = &agentfake.UsageConfig{
			FiveHour: agentfake.UsageWindow{UsedPercentage: 10, ResetsAt: resets},
			SevenDay: agentfake.UsageWindow{UsedPercentage: 99, ResetsAt: resets},
		}
	}
	cfg, err := json.Marshal(wcfg)
	if err != nil {
		return err
	}
	// ownRepo makes a project's own repo; with the chat knob it carries the
	// worker config too, since a session reads the config from its dir.
	ownRepo := func(p string) (string, error) {
		dir := filepath.Join(f.root, p+"-repo")
		if err := f.initRepo(dir); err != nil {
			return "", err
		}
		if sc.chat {
			return dir, os.WriteFile(filepath.Join(dir, agentfake.WorkerConfigFile), cfg, 0o644)
		}
		return dir, nil
	}
	if err := os.WriteFile(filepath.Join(repo, agentfake.WorkerConfigFile), cfg, 0o644); err != nil {
		return err
	}
	for _, args := range [][]string{{"init", "-b", "main"}, {"add", "."}, {"commit", "-m", "fixture"}} {
		if err := f.git(repo, args...); err != nil {
			return err
		}
	}
	if sc.gitSetup != nil {
		if err := sc.gitSetup(f, repo); err != nil {
			return err
		}
	}
	if sc.conversations != nil {
		dir := filepath.Join(f.root, "data", "gx", "conversations")
		started, recs := sc.conversations(time.Now())
		if err := conversations.MarkStarted(dir, started); err != nil {
			return err
		}
		for _, r := range recs {
			if err := conversations.Save(dir, r); err != nil {
				return err
			}
		}
	}
	for _, p := range sc.projects {
		if err := servertest.WriteEpic(f.store, p, ""); err != nil {
			return err
		}
		if !sc.noCheckConfig {
			body := `{"name":"` + p + `","checks":{}}`
			if err := os.WriteFile(filepath.Join(f.store, p, "project.json"), []byte(body), 0o644); err != nil {
				return err
			}
		}
		if slices.Contains(sc.ownRepos, p) {
			dir, err := ownRepo(p)
			if err != nil {
				return err
			}
			extra := ""
			if !sc.noCheckConfig {
				extra = `,"checks":{}`
			}
			if err := servertest.WriteProjectRepo(f.store, p, dir, extra); err != nil {
				return err
			}
		}
	}
	for _, e := range sc.epics {
		if err := servertest.WriteEpic(f.store, e.project, e.name); err != nil {
			return err
		}
		addr := e.project + ":" + e.name
		_, isMap := sc.designSteps[addr]
		if isMap {
			ticketMD := filepath.Join(f.store, e.project, e.name, "ticket.md")
			if err := os.WriteFile(ticketMD, []byte("---\nstatus: open\nkind: map\n---\n"), 0o644); err != nil {
				return err
			}
		}
		if meta, ok := sc.epicMeta[addr]; ok {
			ticketMD := filepath.Join(f.store, e.project, e.name, "ticket.md")
			if err := os.WriteFile(ticketMD, []byte("---\n"+meta+"---\n"), 0o644); err != nil {
				return err
			}
		}
		for _, tk := range e.tickets {
			if err := servertest.WriteTicketFile(f.store, e.project, e.name, tk.id, tk.slug, tk.opts); err != nil {
				return err
			}
			if tk.opts.PauseReason != "" {
				ticketPath := filepath.Join(f.store, e.project, e.name, "issues", tk.id+"-"+tk.slug+".md")
				if err := seedPauseRound(ticketPath, tk.opts.PauseReason, sc.recommend[addr+"/"+tk.id]); err != nil {
					return err
				}
			}
		}
		extra := sc.projectExtra[e.project]
		if sc.diskLow {
			argv, err := json.Marshal([]string{f.cleanScript()})
			if err != nil {
				return err
			}
			extra += `,"disk-space":{"cleanup-command":` + string(argv) + `}`
		}
		if !slices.Contains(sc.manualMerge, e.project) {
			extra += `,"auto-ff-merge":true`
		}
		if !sc.noCheckConfig {
			extra += `,"checks":{}`
		}
		projectRepo := repo
		if slices.Contains(sc.ownRepos, e.project) {
			if projectRepo, err = ownRepo(e.project); err != nil {
				return err
			}
		}
		if err := servertest.WriteProjectRepo(f.store, e.project, projectRepo, extra); err != nil {
			return err
		}
		if isMap {
			if out, err := f.cli("tickets", "set", addr, "--design-step", sc.designSteps[addr]); err != nil {
				return fmt.Errorf("set design step of %s: %w\n%s", addr, err, out)
			}
		}
	}
	return nil
}

// initRepo makes a git repo in dir with one commit.
func (f *fixture) initRepo(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# "+filepath.Base(dir)+"\n"), 0o644); err != nil {
		return err
	}
	for _, args := range [][]string{{"init", "-b", "main"}, {"add", "."}, {"commit", "-m", "fixture"}} {
		if err := f.git(dir, args...); err != nil {
			return err
		}
	}
	return nil
}

// seedPauseRound opens the round a real pause of reason opens: a question only
// where the reply goes to the agent, else a repair round. A recommendation is
// stored on it as an investigate agent would.
func seedPauseRound(ticketPath, reason string, rec recommendation) error {
	summary := "Seeded pause: " + reason
	ev := tickets.LogEvent{Type: tickets.EventPauseRepair, By: "gx", PauseKind: reason, Reason: summary}
	if schema.PauseReason(reason).AsksQuestion() {
		ev = tickets.LogEvent{Type: tickets.EventPauseQuestion, By: "gx", Summary: summary}
	}
	if _, err := tickets.AppendLog(ticketPath, ev); err != nil {
		return err
	}
	if rec.verb == "" {
		return nil
	}
	_, err := tickets.RecommendOpenRound(ticketPath, rec.verb, rec.diagnosis)
	return err
}

// git runs a git command in dir with the fixture's identity.
func (f *fixture) git(dir string, args ...string) error {
	_, _, err := git.Run(dir, f.env, args...)
	return err
}

func (f *fixture) cli(args ...string) ([]byte, error) {
	cmd := exec.Command(f.gx, args...)
	cmd.Env = f.env
	return cmd.CombinedOutput()
}

// linkSession marks the session id a design session of the map at addr.
func (f *fixture) linkSession(id, addr string) error {
	cmd := exec.Command(f.gx, "agent", "link", addr)
	cmd.Env = append(slices.Clone(f.env), nativerunner.AgentSessionEnv+"="+id)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("link %s to %s: %w\n%s", id, addr, err, out)
	}
	return nil
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

// removeAll retries while a stopped process finishes writing into dir.
func removeAll(dir string) {
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if os.RemoveAll(dir) == nil {
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				return
			}
		}
	}
}

// binary returns given when set, and otherwise builds pkg into out.
func binary(given, out, pkg string) (string, error) {
	if given != "" {
		return filepath.Abs(given)
	}
	// The gxfixture tag adds the fake non-Claude harness to the gx binary.
	if b, err := exec.Command("go", "build", "-tags", "gxfixture", "-o", out, pkg).CombinedOutput(); err != nil {
		return "", fmt.Errorf("go build %s: %w\n%s", pkg, err, b)
	}
	return out, nil
}
