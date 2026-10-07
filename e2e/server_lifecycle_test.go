package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
		dir, err := os.MkdirTemp("", "gx-e2e-agentfakeworker")
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

	root, err := os.MkdirTemp("", "gxl")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	e := &lifecycleEnv{t: t, bin: gxBinary(t), env: []string{
		"XDG_DATA_HOME=" + filepath.Join(root, "data"),
		"XDG_STATE_HOME=" + filepath.Join(root, "state"),
		"HOME=" + filepath.Join(root, "home"), // gx reads config from ~/.config, ignoring XDG_CONFIG_HOME
		herdrSocketEnv(),
	}}
	home := filepath.Join(root, "home")
	configDir := filepath.Join(home, ".config", "gx")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// tab-env: HOME is the empty test home so the user's shell rc can't reorder
	// PATH behind the real `claude`, which would block on its startup prompts.
	tabEnv, err := json.Marshal([]string{
		"HOME=" + home,
		"PATH=" + workerBinary(t) + string(os.PathListSeparator) + os.Getenv("PATH"),
	})
	if err != nil {
		t.Fatal(err)
	}
	cfgJSON := fmt.Sprintf(`{"orchestrator":"server","server":{"tab-env":%s}}`, tabEnv)
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(cfgJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	store := filepath.Join(root, "data", "gx", "tickets")
	const project, epic, id = "lifecycle", "tiny-epic", "01"
	address := fmt.Sprintf("%s:%s/%s", project, epic, id)
	label := epic + "-iter-" + id // the server's herdr agent name for the iteration
	t.Cleanup(func() {
		if t.Failed() {
			log, _ := os.ReadFile(filepath.Join(root, "state", "gx", "server.log"))
			t.Logf("server.log:\n%s", log)
			cmd := exec.Command(e.bin, "server", "tickets", "history", address)
			cmd.Env = append(os.Environ(), e.env...)
			hist, _ := cmd.CombinedOutput()
			t.Logf("history:\n%s", hist)
		}
	})

	// The worker agent reads its config from the worktree, so commit it to the repo.
	hold := filepath.Join(root, "hold")
	if err := os.WriteFile(hold, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := json.Marshal(agentfake.WorkerConfig{Gx: e.bin, Env: e.env, Hold: hold})
	if err != nil {
		t.Fatal(err)
	}
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, agentfake.WorkerConfigFile, string(cfg))
	testutil.CommitAll(t, repo, "agentfake config")

	servertest.WriteTicket(t, store, project, epic, id, "tiny", "")
	servertest.SetProjectRepo(t, store, project, repo)

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

func waitForFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	for deadline := time.Now().Add(timeout); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		if _, err := os.Stat(path); err == nil {
			return
		}
	}
	t.Fatalf("%s never appeared", path)
}
