package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/testutil/agentfake"
)

var gxBin, fixtureBin, fakeBin string

func TestMain(m *testing.M) {
	os.Exit(func() int {
		dir, err := os.MkdirTemp("", "gxfb")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		defer os.RemoveAll(dir)
		gxBin, fixtureBin, fakeBin = filepath.Join(dir, "gx"), filepath.Join(dir, "gxfixture"), filepath.Join(dir, "claude")
		for out, pkg := range map[string]string{
			gxBin:      "github.com/elentok/gx",
			fixtureBin: "github.com/elentok/gx/testutil/gxfixture",
			fakeBin:    "github.com/elentok/gx/testutil/agentfake/cmd/fakeclaude",
		} {
			if b, err := exec.Command("go", "build", "-o", out, pkg).CombinedOutput(); err != nil {
				fmt.Fprintf(os.Stderr, "go build %s: %v\n%s", pkg, err, b)
				return 1
			}
		}
		return m.Run()
	}())
}

// running is a started fixture.
type running struct {
	cmd     *exec.Cmd
	stopped bool
	Output
}

func start(t *testing.T, scenario string) *running {
	t.Helper()
	cmd := exec.Command(fixtureBin, "--gx", gxBin, "--fakeclaude", fakeBin, scenario)
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	r := &running{cmd: cmd}
	t.Cleanup(func() {
		if !r.stopped {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	line, err := bufio.NewReader(stdout).ReadBytes('\n')
	if err != nil {
		t.Fatalf("no output line: %v", err)
	}
	if err := json.Unmarshal(line, &r.Output); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, line)
	}
	return r
}

// stop sends SIGTERM and waits for the fixture to exit.
func (r *running) stop(t *testing.T) {
	t.Helper()
	r.stopped = true
	if err := r.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- r.cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("fixture exit: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("fixture did not stop on SIGTERM")
	}
}

func (r *running) gx(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command(gxBin, args...)
	cmd.Env = r.Env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gx %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func (r *running) ticketStatus(t *testing.T, address string) string {
	t.Helper()
	var snap server.Snapshot
	if err := json.Unmarshal([]byte(r.gx(t, "server", "snapshot", "--json")), &snap); err != nil {
		t.Fatal(err)
	}
	for _, tk := range snap.Tickets {
		if tk.Address == address {
			return tk.Status
		}
	}
	return ""
}

func get(t *testing.T, url string) int {
	t.Helper()
	// The token URL redirects after setting a cookie, so the client keeps cookies.
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := (&http.Client{Jar: jar}).Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

func TestScenarios_StartAnswerAndStop(t *testing.T) {
	for _, name := range scenarioNames() {
		t.Run(name, func(t *testing.T) {
			r := start(t, name)
			if r.StateDir == "" || r.HoldFile == "" || r.Socket == "" {
				t.Fatalf("output = %+v, want state dir, socket and hold file", r.Output)
			}
			if !strings.Contains(r.WebURL, "127.0.0.1") || !strings.Contains(r.WebURL, "token=") {
				t.Fatalf("web_url = %q, want a loopback URL with the token", r.WebURL)
			}
			if code := get(t, r.WebURL); code != http.StatusOK {
				t.Errorf("GET web_url = %d, want 200", code)
			}
			bare, _, _ := strings.Cut(r.WebURL, "?")
			if code := get(t, bare); code == http.StatusOK {
				t.Errorf("GET without the token = %d, want it refused", code)
			}
			var st struct {
				Running bool `json:"running"`
			}
			if err := json.Unmarshal([]byte(r.gx(t, "server", "status", "--json")), &st); err != nil || !st.Running {
				t.Fatalf("server status = %+v, %v; want running", st, err)
			}
			if _, err := os.Stat(r.Socket); err != nil {
				t.Errorf("printed socket: %v", err)
			}

			r.stop(t)
			if _, err := os.Stat(filepath.Dir(filepath.Dir(r.StateDir))); !os.IsNotExist(err) {
				t.Errorf("temp dir after SIGTERM: %v, want it removed", err)
			}
			if err := exec.Command("pgrep", "-f", filepath.Dir(filepath.Dir(r.StateDir))).Run(); err == nil {
				t.Errorf("a process still names the temp dir")
			}
		})
	}
}

func TestFixture_NeverTouchesTheRealHomeOrState(t *testing.T) {
	decoy := t.TempDir()
	t.Setenv("HOME", decoy)
	for _, k := range []string{"XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CONFIG_HOME"} {
		t.Setenv(k, filepath.Join(decoy, k))
	}
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(decoy, "claude"))
	r := start(t, "ready")
	r.gx(t, "server", "snapshot", "--json")
	r.stop(t)
	entries, err := os.ReadDir(decoy)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		t.Errorf("the real HOME gained %s", e.Name())
	}
}

func TestFinishingScenario_LastTicketFinishesAndMerges(t *testing.T) {
	r := start(t, "finishing")
	const last = "demo:finishing-epic/02"
	deadline := time.Now().Add(90 * time.Second)
	for r.ticketStatus(t, last) != "done" {
		if status := r.ticketStatus(t, last); status == "needs-repair" || time.Now().After(deadline) {
			t.Logf("history:\n%s", r.gx(t, "server", "tickets", "history", last))
			t.Fatalf("ticket %s is %q, want done", last, status)
		}
		time.Sleep(time.Second)
	}
	r.stop(t)
}

// web is a client of the fixture's web server, with the cookie the token URL sets.
type web struct {
	client *http.Client
	origin string
}

func (r *running) web(t *testing.T) *web {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	c := &http.Client{Jar: jar}
	resp, err := c.Get(r.WebURL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	u, _ := url.Parse(r.WebURL)
	return &web{client: c, origin: u.Scheme + "://" + u.Host}
}

// post sends a write as the web page does and fails on a refusal.
func (w *web) post(t *testing.T, path string, body any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, w.origin+path, strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", w.origin)
	resp, err := w.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var out struct {
		Refused bool `json:"refused"`
	}
	if err := json.Unmarshal(data, &out); err != nil || resp.StatusCode != http.StatusOK || out.Refused {
		t.Fatalf("POST %s = %d: %s", path, resp.StatusCode, data)
	}
}

func (r *running) detail(t *testing.T, address string) server.TicketDetail {
	t.Helper()
	var d server.TicketDetail
	if err := json.Unmarshal([]byte(r.gx(t, "server", "tickets", "detail", address, "--json")), &d); err != nil {
		t.Fatal(err)
	}
	return d
}

// waitPermission waits for the approve ticket's agent to block on a prompt the web can approve.
func (r *running) waitPermission(t *testing.T, address string) server.TicketDetail {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for {
		d := r.detail(t, address)
		if d.Permission != nil && d.Permission.RequestID != "" {
			return d
		}
		if time.Now().After(deadline) {
			t.Fatalf("no approvable permission prompt on %s; detail = %+v", address, d)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

const (
	pausedEpic  = "demo:paused-epic/"
	approveAddr = "demo:approve-epic/01"
)

func TestPausedScenario_OneTicketPerRecoveryPath(t *testing.T) {
	r := start(t, "paused")
	cases := []struct {
		id, reason, action string
	}{
		{"03", "self-reported", server.ActionAnswer},
		{"04", "ambiguous-base", server.ActionResume},
		{"05", "merge-conflict", server.ActionResume},
		{"06", "zero-commit", server.ActionResume},
	}
	for _, c := range cases {
		d := r.detail(t, pausedEpic+c.id)
		if d.PauseReason != c.reason || d.PauseLabel == "" {
			t.Errorf("%s: reason %q label %q, want reason %q", c.id, d.PauseReason, d.PauseLabel, c.reason)
		}
		if !slices.Contains(d.Actions, c.action) {
			t.Errorf("%s: actions = %v, want %q", c.id, d.Actions, c.action)
		}
	}
	if bases := r.detail(t, pausedEpic+"04").Bases; !slices.Equal(bases, []string{"01", "02"}) {
		t.Errorf("choose-base bases = %v, want 01 and 02", bases)
	}

	repair := r.detail(t, pausedEpic+"07")
	if repair.PauseReason != "iteration-error" || repair.Status != "needs-repair" {
		t.Errorf("repair ticket: reason %q status %q, want iteration-error and needs-repair", repair.PauseReason, repair.Status)
	}

	d := r.waitPermission(t, approveAddr)
	if d.Permission.Tool != "Bash command" || d.Permission.Description != agentfake.PermissionCommand || !slices.Contains(d.Actions, server.ActionApprove) {
		t.Errorf("approve ticket: permission %+v actions %v, want the Bash prompt and approve", d.Permission, d.Actions)
	}
	r.stop(t)
}

func TestPausedScenario_WebRecoveryClearsEachPause(t *testing.T) {
	r := start(t, "paused")
	w := r.web(t)

	d := r.detail(t, pausedEpic+"03")
	w.post(t, "/v1/tickets/answer", map[string]any{"address": d.Address, "text": "go ahead", "state": d.State})

	// 05 (merge conflict) is left out: repair.Resume refuses a repair-class pause,
	// so the web's "Resolve conflict" cannot clear it. See the ticket's Spec deviations.
	for _, c := range []struct{ id, base string }{{"04", "01"}, {"06", ""}} {
		d := r.detail(t, pausedEpic+c.id)
		body := map[string]any{"address": d.Address, "state": d.State}
		if c.base != "" {
			body["base"] = c.base
		}
		w.post(t, "/v1/tickets/resume", body)
	}
	for _, id := range []string{"03", "04", "06"} {
		if d := r.detail(t, pausedEpic+id); d.Status != "open" {
			t.Errorf("%s is %q (reason %q) after its recovery, want open", id, d.Status, d.PauseReason)
		}
	}

	p := r.waitPermission(t, approveAddr).Permission
	w.post(t, "/v1/agents/answer", map[string]any{"address": approveAddr, "decision": "allow", "request_id": p.RequestID})
	if d := r.detail(t, approveAddr); d.Permission != nil {
		t.Errorf("permission after approve = %+v, want it cleared", d.Permission)
	}
	r.stop(t)
}

const designAddr = "demo:design-epic"

func (r *running) currentStep(t *testing.T) server.ProgressStep {
	t.Helper()
	var d server.EpicDetail
	if err := json.Unmarshal([]byte(r.gx(t, "server", "epics", "detail", designAddr, "--json")), &d); err != nil {
		t.Fatal(err)
	}
	for _, s := range d.Steps {
		if s.State == server.MapStepCurrent {
			return s.Step
		}
	}
	t.Fatalf("no current step in %+v", d.Steps)
	return ""
}

// waitChat polls the session's chat until ok accepts it.
func (r *running) waitChat(t *testing.T, id string, ok func(server.SessionChatResponse) bool) server.SessionChatResponse {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		var chat server.SessionChatResponse
		if err := json.Unmarshal([]byte(r.gx(t, "agents", "chat", id, "--json")), &chat); err != nil {
			t.Fatal(err)
		}
		if ok(chat) {
			return chat
		}
		if time.Now().After(deadline) {
			t.Fatalf("chat never reached the wanted state; round = %+v next = %+v", chat.Round, chat.Next)
		}
		time.Sleep(300 * time.Millisecond)
	}
}

// waitStep polls until the epic detail shows step as current.
func (r *running) waitStep(t *testing.T, step server.ProgressStep) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for r.currentStep(t) != step {
		if time.Now().After(deadline) {
			t.Fatalf("current step = %q, want %q", r.currentStep(t), step)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// waitIdle waits for the session to end its turn: the scripted message is in
// the chat while the turn is still ending, and a prompt typed then is refused.
func (r *running) waitIdle(t *testing.T, id string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		var list []server.SessionInfo
		if err := json.Unmarshal([]byte(r.gx(t, "agents", "list", "--json")), &list); err != nil {
			t.Fatal(err)
		}
		for _, s := range list {
			if s.ID == id && s.Status.State == server.SessionIdle {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("session %s never went idle", id)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func roundLabels(chat server.SessionChatResponse) []string {
	var labels []string
	for _, c := range chat.Round {
		labels = append(labels, c.Label)
	}
	return labels
}

func TestDesignScenario_SeededMapAndScriptedChat(t *testing.T) {
	r := start(t, "design")
	if step := r.currentStep(t); step != server.ProgressResolveTickets {
		t.Errorf("current step = %q, want resolve-tickets", step)
	}
	// The verb moves the step the page shows.
	r.gx(t, "tickets", "set", designAddr, "--design-step", "spec")
	r.waitStep(t, server.ProgressSpec)
	repo := filepath.Join(filepath.Dir(filepath.Dir(r.StateDir)), "repo")
	tracked, err := exec.Command("git", "-C", repo, "ls-files").Output()
	if err != nil || strings.TrimSpace(string(tracked)) != agentfake.WorkerConfigFile {
		t.Errorf("trunk files = %q, %v; want only the worker config, so no spec", tracked, err)
	}

	var res server.SessionResult
	if err := json.Unmarshal([]byte(r.gx(t, "designs", "open", designAddr, "--json")), &res); err != nil || res.Session == nil {
		t.Fatalf("designs open = %+v, %v", res, err)
	}
	id := res.Session.ID

	chat := r.waitChat(t, id, func(c server.SessionChatResponse) bool { return len(c.Round) > 0 })
	if got := roundLabels(chat); !slices.Equal(got, []string{"8", "8b"}) {
		t.Fatalf("round labels = %v, want 8 and 8b", got)
	}

	r.waitIdle(t, id)
	r.gx(t, "agents", "prompt", id, "SQLite, and yes to the migration")
	chat = r.waitChat(t, id, func(c server.SessionChatResponse) bool { return c.Next != nil })
	if len(chat.Next.Action.Action) == 0 {
		t.Errorf("next = %+v, want the Continue action", chat.Next)
	}

	// Continue as the page sends it; the CLI's run-action needs the session's
	// Actionable, which an answer typed by the server does not clear.
	r.waitIdle(t, id)
	r.web(t).post(t, "/v1/sessions/prompt", map[string]any{"id": id, "continue": true, "last_message": chat.Messages[len(chat.Messages)-1].ID})
	r.waitChat(t, id, func(c server.SessionChatResponse) bool { return slices.Equal(roundLabels(c), []string{"9"}) })
	r.stop(t)
}
