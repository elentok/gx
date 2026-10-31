package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/testutil/herdrctl"
)

var (
	buildWorkerOnce sync.Once
	workerBinDir    string
	buildWorkerErr  error
)

// workerBinary builds testutil/agentfake's worker as the literal binary name
// "claude" and returns its directory.
func workerBinary(t *testing.T) string {
	t.Helper()
	buildWorkerOnce.Do(func() {
		dir, err := os.MkdirTemp(binRoot, "agentfakeworker")
		if err != nil {
			buildWorkerErr = err
			return
		}
		cmd := exec.Command("go", "build", "-o", filepath.Join(dir, "claude"), "github.com/elentok/gx/testutil/agentfake/cmd/agentfakeworker")
		if out, err := cmd.CombinedOutput(); err != nil {
			buildWorkerErr = err
			t.Logf("go build output:\n%s", out)
			return
		}
		workerBinDir = dir
	})
	if buildWorkerErr != nil {
		t.Fatalf("build agentfakeworker binary: %v", buildWorkerErr)
	}
	return workerBinDir
}

// herdrSocketEnv pins the real herdr's socket for a child whose HOME is
// overridden. Inside herdr the var is inherited; on a bare runner herdr's
// default socket lives under the real HOME, which the child would not find.
func herdrSocketEnv() string {
	if p := os.Getenv("HERDR_SOCKET_PATH"); p != "" {
		return "HERDR_SOCKET_PATH=" + p
	}
	home, _ := os.UserHomeDir()
	return "HERDR_SOCKET_PATH=" + filepath.Join(home, ".config", "herdr", "herdr.sock")
}

// lifecycleEnv is the real gx binary's isolated world: its own state dir,
// ticket store and config, so the e2e never touches the user's server.
type lifecycleEnv struct {
	t   *testing.T
	bin string
	env []string
}

func (e *lifecycleEnv) gx(args ...string) string {
	e.t.Helper()
	cmd := exec.Command(e.bin, args...)
	cmd.Env = append(os.Environ(), e.env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		e.t.Fatalf("gx %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func (e *lifecycleEnv) running() (server.Handshake, bool) {
	e.t.Helper()
	var st struct {
		Running bool `json:"running"`
		Pid     int  `json:"pid"`
	}
	if err := json.Unmarshal([]byte(e.gx("server", "status", "--json")), &st); err != nil {
		e.t.Fatal(err)
	}
	return server.Handshake{Pid: st.Pid}, st.Running
}

func (e *lifecycleEnv) ticketStatus(address string) string {
	e.t.Helper()
	var snap server.Snapshot
	if err := json.Unmarshal([]byte(e.gx("server", "snapshot", "--json")), &snap); err != nil {
		e.t.Fatal(err)
	}
	for _, tk := range snap.Tickets {
		if tk.Address == address {
			return tk.Status
		}
	}
	return ""
}

func (e *lifecycleEnv) waitStatus(address, want string, timeout time.Duration) {
	e.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if e.ticketStatus(address) == want {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	e.t.Fatalf("ticket %s never reached %q (last %q)", address, want, e.ticketStatus(address))
}

// TestServerLifecycle_StartEnqueueLandStopRestartReclaims drives the real gx
// binary and real herdr through the orchestrator daemon's whole life: a
// self-detaching `gx server start`, enqueueing a one-ticket epic, a stop while
// the agent is mid-iteration, and a restart that reclaims the live iteration
// and lands the ticket.
func TestServerLifecycle_StartEnqueueLandStopRestartReclaims(t *testing.T) {
	herdrctl.RequireHerdr(t)

	w := newAgentWorld(t)
	e, repo, hold, address, label, epic := w.e, w.repo, w.hold, w.address, w.label, w.epic

	// Self-detaching start: launched from a pane's shell, the server must end up
	// in its own session, reparented away from that shell.
	ws := herdrctl.NewWorkspace(t, repo)
	ws.PrependPath(workerBinary(t))

	// The server reuses a workspace labeled after the epic; its tabs get the
	// fake `claude` on PATH through the config's tab-env (see the config above).
	out, err := exec.Command("herdr", "workspace", "create", "--cwd", repo, "--label", epic, "--no-focus").Output()
	if err != nil {
		t.Fatalf("create epic workspace: %v\n%s", err, out)
	}
	var created struct {
		Result struct {
			Workspace struct {
				ID string `json:"workspace_id"`
			} `json:"workspace"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &created); err != nil {
		t.Fatalf("parse workspace create: %v\n%s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("herdr", "workspace", "close", created.Result.Workspace.ID).Run() })

	ws.Run(append(append([]string{"env"}, e.env...), e.bin, "server", "start")...)
	ws.WaitForText("started (pid", 30*time.Second)
	t.Cleanup(func() {
		if _, up := e.running(); up {
			e.gx("server", "stop")
		}
	})
	h, up := e.running()
	if !up {
		t.Fatal("server not running after `gx server start`")
	}
	ps, err := exec.Command("ps", "-o", "ppid=,pgid=", "-p", fmt.Sprint(h.Pid)).Output()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Fields(string(ps)), []string{"1", fmt.Sprint(h.Pid)}; strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("server ppid/pgid = %v, want %v (detached into its own session)", got, want)
	}

	e.gx("server", "queue", "add", address)
	e.waitStatus(address, "claimed", 60*time.Second)

	// Claiming comes before the launch: wait until the agent has its prompt.
	waitForFile(t, hold+agentfake.StartedSuffix, 60*time.Second)

	// Stop mid-iteration: the agent is still held working in herdr.
	e.gx("server", "stop")
	if _, up := e.running(); up {
		t.Fatal("server still running after stop")
	}
	if _, err := herdr.AgentGet(label); err != nil {
		t.Fatalf("agent %s did not survive the server stopping: %v", address, err)
	}

	// Restart reclaims the live iteration; once the agent finishes, it lands.
	e.gx("server", "start")
	if err := os.Remove(hold); err != nil {
		t.Fatal(err)
	}
	e.waitStatus(address, "done", 90*time.Second)
}

// newLifecycleEnv is a lifecycleEnv in its own root dir, with extra env entries
// after the isolating ones. It stops the server at the end and, on failure,
// logs what the server and the restart wrote.
func newLifecycleEnv(t *testing.T, extra ...string) (*lifecycleEnv, string) {
	t.Helper()
	root, err := os.MkdirTemp("", "gxl")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	e := &lifecycleEnv{t: t, bin: gxBinary(t), env: append([]string{
		"XDG_DATA_HOME=" + filepath.Join(root, "data"),
		"XDG_STATE_HOME=" + filepath.Join(root, "state"),
		"HOME=" + filepath.Join(root, "home"), // gx reads config from ~/.config, ignoring XDG_CONFIG_HOME
		herdrSocketEnv(),
	}, extra...)}
	if err := os.MkdirAll(filepath.Join(root, "home", ".config", "gx"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, up := e.running(); up {
			e.gx("server", "stop")
		}
	})
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		for _, name := range []string{"server.log", "restart.log"} {
			log, _ := os.ReadFile(filepath.Join(root, "state", "gx", name))
			t.Logf("%s:\n%s", name, log)
		}
	})
	return e, root
}

// agentWorld is a lifecycleEnv with a one-ticket epic whose agent is the fake
// worker, held mid-iteration until the hold file is removed.
type agentWorld struct {
	e                                         *lifecycleEnv
	repo, hold, address, label, epic, project string
}

func newAgentWorld(t *testing.T) *agentWorld {
	t.Helper()
	e, root := newLifecycleEnv(t)
	home := filepath.Join(root, "home")
	// tab-env: HOME is the empty test home so the user's shell rc can't reorder
	// PATH behind the real `claude`, which would block on its startup prompts.
	tabEnv, err := json.Marshal([]string{
		"HOME=" + home,
		"PATH=" + workerBinary(t) + string(os.PathListSeparator) + os.Getenv("PATH"),
	})
	if err != nil {
		t.Fatal(err)
	}
	cfgJSON := fmt.Sprintf(`{"server":{"tab-env":%s}}`, tabEnv)
	if err := os.WriteFile(filepath.Join(home, ".config", "gx", "config.json"), []byte(cfgJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	store := filepath.Join(root, "data", "gx", "tickets")
	const project, epic, id = "lifecycle", "tiny-epic", "01"
	w := &agentWorld{e: e, project: project, epic: epic,
		address: fmt.Sprintf("%s:%s/%s", project, epic, id),
		label:   epic + "-iter-" + id, // the server's herdr agent name for the iteration
		hold:    filepath.Join(root, "hold"),
	}
	t.Cleanup(func() {
		if t.Failed() {
			cmd := exec.Command(e.bin, "server", "tickets", "history", w.address)
			cmd.Env = append(os.Environ(), e.env...)
			hist, _ := cmd.CombinedOutput()
			t.Logf("history:\n%s", hist)
		}
	})

	// The worker agent reads its config from the worktree, so commit it to the repo.
	if err := os.WriteFile(w.hold, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := json.Marshal(agentfake.WorkerConfig{Gx: e.bin, Env: e.env, Hold: w.hold})
	if err != nil {
		t.Fatal(err)
	}
	w.repo = testutil.TempRepo(t)
	testutil.WriteFile(t, w.repo, agentfake.WorkerConfigFile, string(cfg))
	testutil.CommitAll(t, w.repo, "agentfake config")

	servertest.WriteTicket(t, store, project, epic, id, "tiny", "")
	servertest.SetProjectRepo(t, store, project, w.repo)
	return w
}

// waitRestarted waits for a server whose pid is not old, which is how a
// finished restart looks from outside.
func (e *lifecycleEnv) waitRestarted(old int, timeout time.Duration) server.Handshake {
	e.t.Helper()
	for deadline := time.Now().Add(timeout); time.Now().Before(deadline); time.Sleep(500 * time.Millisecond) {
		if h, up := e.running(); up && h.Pid != old {
			return h
		}
	}
	e.t.Fatalf("no new server after the restart (old pid %d)", old)
	return server.Handshake{}
}

// TestServerLifecycle_WebRestartReclaimsRunningIteration restarts through the
// restart route while an agent is mid-iteration: the new server reclaims the
// iteration and lands the ticket.
func TestServerLifecycle_WebRestartReclaimsRunningIteration(t *testing.T) {
	herdrctl.RequireHerdr(t)
	w := newAgentWorld(t)
	e := w.e

	e.gx("server", "start")
	h, _ := e.running()
	e.gx("server", "queue", "add", w.address)
	e.waitStatus(w.address, "claimed", 60*time.Second)
	waitForFile(t, w.hold+agentfake.StartedSuffix, 60*time.Second)

	e.gx("server", "restart-now")
	e.waitRestarted(h.Pid, 90*time.Second)
	if _, err := herdr.AgentGet(w.label); err != nil {
		t.Fatalf("agent %s did not survive the restart: %v", w.address, err)
	}

	if err := os.Remove(w.hold); err != nil {
		t.Fatal(err)
	}
	e.waitStatus(w.address, "done", 90*time.Second)
}

// TestServerLifecycle_WebRestartWithFailedMigrationStartsAnyway restarts with a
// project whose migration fails: the server still comes back and the failure is
// saved and logged.
func TestServerLifecycle_WebRestartWithFailedMigrationStartsAnyway(t *testing.T) {
	e, root := newLifecycleEnv(t)
	store := filepath.Join(root, "data", "gx", "tickets")
	// Two tickets naming each other as parent cannot migrate.
	for id, parent := range map[string]string{"01": "02", "02": "01"} {
		if err := servertest.WriteTicketFile(store, "broken", "epic", id, "t"+id, servertest.TicketOpts{Parent: parent}); err != nil {
			t.Fatal(err)
		}
	}

	e.gx("server", "start")
	h, _ := e.running()
	e.gx("server", "restart-now")
	e.waitRestarted(h.Pid, 60*time.Second)

	state := filepath.Join(root, "state", "gx")
	failures, err := os.ReadFile(filepath.Join(state, "migration-failures.json"))
	if err != nil || !strings.Contains(string(failures), "broken") {
		t.Fatalf("migration-failures.json does not name the broken project: %v\n%s", err, failures)
	}
	log, _ := os.ReadFile(filepath.Join(state, "restart.log"))
	if !strings.Contains(string(log), "migrate broken failed") {
		t.Fatalf("restart.log does not report the failure:\n%s", log)
	}
}

// TestServerLifecycle_WebRestartThroughLaunchdSurvivesStop restarts a server
// run by a fake launchd agent, with a migration that stays slow until released.
// The detached restart must outlive the server it stops, start no server
// before the migration ends, and bring the server back.
func TestServerLifecycle_WebRestartThroughLaunchdSurvivesStop(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the launchd agent is macOS only")
	}
	fake := t.TempDir()
	e, root := newLifecycleEnv(t, "PATH="+fake+string(os.PathListSeparator)+os.Getenv("PATH"))
	gate, migrating := filepath.Join(fake, "gate"), filepath.Join(fake, "migrating")
	script := func(name, body string) string {
		path := filepath.Join(fake, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
			t.Fatal(err)
		}
		return path
	}
	// launchd runs the server in the background and signals it by pid file.
	script("launchctl", fmt.Sprintf(`case "$1" in
kickstart)
  nohup %q server </dev/null >>%q 2>&1 &
  echo $! > %q ;;
kill) kill -TERM "$(cat %q)" ;;
esac
`, e.bin, filepath.Join(fake, "server.out"), filepath.Join(fake, "server.pid"), filepath.Join(fake, "server.pid")))
	// The restart binary: the slow migration stands in for the real one, between
	// the real stop and the real start.
	restartBin := script("gx", fmt.Sprintf(`if [ "$1 $2 $3" = "server restart --continue" ]; then
  %[1]q server stop || exit 1
  : > %[2]q
  while [ ! -e %[3]q ]; do sleep 0.1; done
  exec %[1]q server start
fi
exec %[1]q "$@"
`, e.bin, migrating, gate))
	agents := filepath.Join(root, "home", "Library", "LaunchAgents")
	if err := os.MkdirAll(agents, 0o755); err != nil {
		t.Fatal(err)
	}
	plist := "<plist><dict><key>ProgramArguments</key><array><string>" + restartBin + "</string><string>server</string></array></dict></plist>"
	if err := os.WriteFile(filepath.Join(agents, "com.elentok.gx.server.plist"), []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := servertest.WriteEpic(filepath.Join(root, "data", "gx", "tickets"), "lifecycle", "epic"); err != nil {
		t.Fatal(err)
	}

	e.gx("server", "start")
	h, up := e.running()
	if !up {
		t.Fatal("server not running after `gx server start` through the fake launchd")
	}
	e.gx("server", "restart-now")

	waitForFile(t, migrating, 60*time.Second)
	if _, up := e.running(); up {
		t.Fatal("a server is running while the migration is still going")
	}
	if err := os.WriteFile(gate, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	e.waitRestarted(h.Pid, 60*time.Second)
}

func waitForFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	for deadline := time.Now().Add(timeout); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		if _, err := os.Stat(path); err == nil {
			return
		}
	}
	t.Fatalf("%s never appeared", path)
}
