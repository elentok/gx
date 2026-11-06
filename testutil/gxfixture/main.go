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
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

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
	if sc.LimitBurn && shape == "live" {
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

	rs, err := f.startServer(ctx, sc)
	if rs != nil {
		defer rs.logFile.Close()
	}
	if err != nil {
		return err
	}
	defer f.runShutdown(&err, rs)

	if err := f.loginWeb(ctx, sc, rs); err != nil {
		return err
	}
	s, err := f.seedAfterStart(ctx, o, sc)
	if err != nil {
		return err
	}
	res := f.buildOutput(sc, rs, s)
	if o.mockDir != "" {
		stopMock, mockURL, err := serveMock(o.mockDir)
		if err != nil {
			return err
		}
		defer stopMock()
		res.MockURL = mockURL
	}
	if err := printOutput(o, res); err != nil {
		return err
	}
	return f.waitForEnd(ctx, sc, rs.exited)
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
