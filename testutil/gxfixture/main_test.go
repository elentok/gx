package main

import (
	"bufio"
	"context"
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
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/transcript"
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

// cardsOf is `gx server epics --json`.
func (r *running) cardsOf(t *testing.T) []server.EpicListRow {
	t.Helper()
	var list server.EpicList
	if err := json.Unmarshal([]byte(r.gx(t, "server", "epics", "--json")), &list); err != nil {
		t.Fatal(err)
	}
	return list.Epics
}

func TestUnitedScenarioHasACardPerSectionAndReason(t *testing.T) {
	t.Parallel()
	r := start(t, "united")
	// name|section|pill. Running, queued and permission cards appear once the
	// server has launched the workers, so the list is polled.
	want := []string{
		"deadlock-epic|needs-you|Deadlocked",
		"awaiting-epic|needs-you|Awaiting merge",
		"blocked-merge-epic|needs-you|Merge blocked",
		"dirty-trunk-epic|needs-you|Merge blocked",
		"design-stopped|in-progress|",
		"paused-epic|needs-you|Paused",
		"question-epic|needs-you|Question",
		"perm-epic|needs-you|Permission",
		"design-question|needs-you|Question",
		"paused|needs-you|Paused",
		"running-epic|in-progress|",
		"queued-epic|in-progress|",
		"queued|in-progress|",
		"awaiting-auto-epic|in-progress|",
		"design-live|in-progress|",
		"ready-epic|ready-to-queue|",
		"follow-ups|ready-to-design|",
		"design-new|ready-to-design|",
		"draft-epic|ready-to-design|",
		"idle-epic|ready-to-design|",
		"finished-a|finished|",
	}
	var got []string
	var missing []string
	projects := map[string]bool{}
	for deadline := time.Now().Add(40 * time.Second); time.Now().Before(deadline); time.Sleep(500 * time.Millisecond) {
		got, projects = got[:0], map[string]bool{}
		for _, c := range r.cardsOf(t) {
			got = append(got, c.Name+"|"+string(c.Section)+"|"+c.StatePill)
			projects[c.Project] = true
		}
		missing = slices.DeleteFunc(slices.Clone(want), func(w string) bool { return slices.Contains(got, w) })
		if len(missing) == 0 {
			break
		}
	}
	if len(missing) > 0 {
		t.Fatalf("missing cards %v\ngot:\n%s", missing, strings.Join(got, "\n"))
	}
	if !projects["demo"] || !projects["auto"] {
		t.Errorf("cards span projects %v, want demo and auto", projects)
	}
	// Three finished one-offs exist and server.recently-finished is 2.
	oneOffs := 0
	for _, g := range got {
		if strings.HasPrefix(g, "done-") {
			oneOffs++
		}
	}
	if oneOffs != 2 {
		t.Errorf("%d finished one-offs listed, want 2:\n%s", oneOffs, strings.Join(got, "\n"))
	}
}

// running is a started fixture.
type running struct {
	cmd     *exec.Cmd
	stopped bool
	Output
}

func start(t *testing.T, scenario string) *running {
	t.Helper()
	return startWith(t, scenario, nil)
}

// startWith starts the scenario with extra gxfixture flags. env (KEY=VALUE)
// goes after the test's own env in the fixture's.
func startWith(t *testing.T, scenario string, env []string, flags ...string) *running {
	t.Helper()
	args := append([]string{"--gx", gxBin, "--fakeclaude", fakeBin}, flags...)
	cmd := exec.Command(fixtureBin, append(args, scenario)...)
	cmd.Env = append(os.Environ(), env...)
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
		if r.stopped {
			return
		}
		// SIGTERM lets the fixture stop its server and holders; a kill orphans them.
		_ = cmd.Process.Signal(syscall.SIGTERM)
		done := make(chan struct{})
		go func() { _ = cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(30 * time.Second):
			_ = cmd.Process.Kill()
			<-done
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
	t.Parallel()
	for _, name := range scenarioNames() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
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

func TestShellEnv_ExportsTheCLIEnvAndTheFixtureVariablesQuoted(t *testing.T) {
	o := Output{
		WebURL: "http://127.0.0.1:1/?token=a b", StateDir: "/tmp/s", Socket: "/tmp/s.sock", HoldFile: "/tmp/h",
		Gx: "/bin/gx", DiskDir: "/tmp/disk", Env: []string{"HOME=/tmp/it's/home", "XDG_DATA_HOME=/tmp/d"},
	}
	want := "export HOME='/tmp/it'\\''s/home'\n" +
		"export XDG_DATA_HOME='/tmp/d'\n" +
		"export GXF_WEB_URL='http://127.0.0.1:1/?token=a b'\n" +
		"export GXF_GX='/bin/gx'\n" +
		"export GXF_DISK_DIR='/tmp/disk'\n" +
		"export GXF_HOLD_FILE='/tmp/h'\n" +
		"export GXF_STATE_DIR='/tmp/s'\n" +
		"export GXF_SOCKET='/tmp/s.sock'\n"
	if got := o.ShellEnv(); got != want {
		t.Errorf("ShellEnv() =\n%s\nwant\n%s", got, want)
	}
}

func TestIsolatedEnv_TurnsNotificationsOffEvenWhenTheCallerSetsThemOn(t *testing.T) {
	t.Setenv("GX_NOTIFY", "on")
	env, cliEnv := isolatedEnv("/tmp/r", "/tmp/r/bin")
	var got []string
	for _, kv := range env {
		if strings.HasPrefix(kv, "GX_NOTIFY=") {
			got = append(got, kv)
		}
	}
	if want := []string{"GX_NOTIFY=off"}; !slices.Equal(got, want) {
		t.Errorf("server env GX_NOTIFY = %v, want %v", got, want)
	}
	for _, kv := range cliEnv {
		if strings.HasPrefix(kv, "GX_NOTIFY=") {
			t.Errorf("cliEnv carries %s; it should name only what reaches the server", kv)
		}
	}
}

func TestWatchLifetime_EndsWhenTheParentDiesOrTheTTLPasses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		ppid   int
		ttl    time.Duration
		reason string
	}{
		{"parent died", 1, time.Hour, "parent"},
		{"ttl passed", 42, time.Millisecond, "ttl"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			got := watchLifetime(ctx, 42, func() int { return tc.ppid }, tc.ttl, time.Millisecond)
			if got != tc.reason {
				t.Errorf("watchLifetime() = %q, want %q", got, tc.reason)
			}
		})
	}
}

func TestFixture_NeverTouchesTheRealHomeOrState(t *testing.T) {
	t.Parallel()
	decoy := t.TempDir()
	env := []string{"HOME=" + decoy, "CLAUDE_CONFIG_DIR=" + filepath.Join(decoy, "claude")}
	for _, k := range []string{"XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CONFIG_HOME"} {
		env = append(env, k+"="+filepath.Join(decoy, k))
	}
	r := startWith(t, "ready", env)
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
	t.Parallel()
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
	if data, ok := w.try(t, path, body); !ok {
		t.Fatalf("POST %s refused: %s", path, data)
	}
}

// postWhenIdle is post, retried while the server still reports the session as
// working: its status lags the fake's turn, as promptWhenIdle describes.
func (r *running) postWhenIdle(t *testing.T, id, path string, body any) {
	t.Helper()
	w := r.web(t)
	deadline := time.Now().Add(20 * time.Second)
	for {
		data, ok := w.try(t, path, body)
		if ok {
			return
		}
		if !strings.Contains(data, "session-not-idle") || time.Now().After(deadline) {
			t.Fatalf("POST %s refused: %s", path, data)
		}
		r.waitIdle(t, id)
	}
}

// try posts once and reports whether the server took the write; data is the response.
func (w *web) try(t *testing.T, path string, body any) (data string, ok bool) {
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
	raw, _ = io.ReadAll(resp.Body)
	var out struct {
		Refused bool `json:"refused"`
	}
	err = json.Unmarshal(raw, &out)
	return fmt.Sprintf("%d: %s", resp.StatusCode, raw), err == nil && resp.StatusCode == http.StatusOK && !out.Refused
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

// issueFiles is the ticket files of an epic on disk, by file name.
func (r *running) issueFiles(t *testing.T, project, epic string) map[string]string {
	t.Helper()
	root := filepath.Dir(filepath.Dir(r.StateDir))
	dir := filepath.Join(root, "data", "gx", "tickets", project, epic, "issues")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		files[e.Name()] = string(data)
	}
	return files
}

func TestManyDoneScenario_FiveMergedWithDistinctCloseTimesAndOneOpen(t *testing.T) {
	t.Parallel()
	r := start(t, "many-done")
	closed := map[string]bool{}
	open := 0
	for name, data := range r.issueFiles(t, "demo", "many-done-epic") {
		switch {
		case strings.Contains(data, "status: done\n"):
			_, after, ok := strings.Cut(data, "done_at: ")
			if !ok || !strings.Contains(data, "claimed_at: ") {
				t.Fatalf("%s is done without stamps:\n%s", name, data)
			}
			closed[strings.SplitN(after, "\n", 2)[0]] = true
		case strings.Contains(data, "status: open\n"):
			open++
		}
	}
	if len(closed) != 5 || open != 1 {
		t.Errorf("close times %v, open %d; want 5 distinct close times and 1 open", closed, open)
	}
	r.stop(t)
}

func TestDesignScenario_HasATimedDoneTicket(t *testing.T) {
	t.Parallel()
	r := start(t, "design")
	data := r.issueFiles(t, "demo", "design-epic")["03-timed.md"]
	for _, want := range []string{"status: done\n", "claimed_at: 2026-01-01T10:00:00Z\n", "done_at: 2026-01-01T10:02:30Z\n"} {
		if !strings.Contains(data, want) {
			t.Errorf("timed ticket lacks %q:\n%s", want, data)
		}
	}
	r.stop(t)
}

func TestUnitedScenario_HasADraftImplEpic(t *testing.T) {
	t.Parallel()
	r := start(t, "united")
	root := filepath.Dir(filepath.Dir(r.StateDir))
	meta, err := os.ReadFile(filepath.Join(root, "data", "gx", "tickets", "demo", "publishing-epic-impl", "ticket.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(meta), "status: draft\n") {
		t.Errorf("publishing-epic-impl ticket.md:\n%s", meta)
	}
	r.stop(t)
}

func TestUnitedScenario_CarriesTheCardTimeStamps(t *testing.T) {
	t.Parallel()
	r := start(t, "united")
	root := filepath.Dir(filepath.Dir(r.StateDir))
	read := func(path ...string) string {
		data, err := os.ReadFile(filepath.Join(append([]string{root, "data", "gx", "tickets"}, path...)...))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	for _, c := range []struct{ project, epic, stamp string }{
		{"demo", "ready-epic", "created_at: "},
		{"demo", "idle-epic", "created_at: "},
		{"demo", "design-live", "design_started_at: "},
		{"demo", "design-stopped", "design_started_at: "},
		{"demo", "running-epic", "started_at: "},
		{"auto", "awaiting-auto-epic", "started_at: "},
		{"demo", "awaiting-epic", "completed_at: "},
		{"demo", "blocked-merge-epic", "completed_at: "},
		{"demo", "dirty-trunk-epic", "completed_at: "},
	} {
		if meta := read(c.project, c.epic, "ticket.md"); !strings.Contains(meta, c.stamp) {
			t.Errorf("%s lacks %q:\n%s", c.epic, c.stamp, meta)
		}
	}
	if got := read("demo", "deadlock-epic", "issues", "01-first.md"); !strings.Contains(got, "claimed_at: ") {
		t.Errorf("deadlock-epic/01 lacks claimed_at:\n%s", got)
	}
	created := strings.SplitN(strings.SplitN(read("demo", "ready-epic", "ticket.md"), "created_at: ", 2)[1], "\n", 2)[0]
	at, err := time.Parse(time.RFC3339, created)
	if err != nil || time.Since(at) > time.Hour {
		t.Errorf("ready-epic created_at %q is not minutes old (%v)", created, err)
	}
	r.stop(t)
}

func TestDesignScenario_StampsTheMapsDesignStart(t *testing.T) {
	t.Parallel()
	r := start(t, "design")
	root := filepath.Dir(filepath.Dir(r.StateDir))
	meta, err := os.ReadFile(filepath.Join(root, "data", "gx", "tickets", "demo", "design-epic", "ticket.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(meta), "design_started_at: ") || !strings.Contains(string(meta), "kind: map") {
		t.Errorf("design-epic ticket.md:\n%s", meta)
	}
	r.stop(t)
}

func TestConfigFlag_WritesTheKeyIntoTheFixtureConfig(t *testing.T) {
	t.Parallel()
	r := startWith(t, "empty", nil, "--config", "server.remote-control=true", "--config", "extra.name=hello")
	root := filepath.Dir(filepath.Dir(r.StateDir))
	raw, err := os.ReadFile(filepath.Join(root, "home", ".config", "gx", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Server struct {
			RemoteControl bool `json:"remote-control"`
			WatchConfig   bool `json:"watch-config"`
		} `json:"server"`
		Extra struct {
			Name string `json:"name"`
		} `json:"extra"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	if !cfg.Server.RemoteControl || cfg.Extra.Name != "hello" {
		t.Errorf("config = %s, want server.remote-control true and extra.name hello", raw)
	}
	r.stop(t)
}

func TestPausedScenario_OneTicketPerRecoveryPath(t *testing.T) {
	t.Parallel()
	r := start(t, "paused")
	cases := []struct {
		id, reason, action string
	}{
		{"03", "self-reported", server.ActionAnswer},
		{"04", "ambiguous-base", server.ActionChooseBase},
		{"05", "merge-conflict", server.ActionMerge},
		{"06", "zero-commit", server.ActionRetry},
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

// roundTypes is the event type of each pause round in the ticket's log.
func (r *running) roundTypes(t *testing.T, project, epic, id string) []string {
	t.Helper()
	root := filepath.Dir(filepath.Dir(r.StateDir))
	paths, _ := filepath.Glob(filepath.Join(root, "data", "gx", "tickets", project, epic, "issues", id+"-*.md"))
	if len(paths) != 1 {
		t.Fatalf("ticket files for %s = %v", id, paths)
	}
	evs, err := tickets.ReadLog(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	var types []string
	for _, round := range tickets.AllRounds(evs) {
		types = append(types, round.Question.Type)
	}
	return types
}

func TestPausedScenario_EachPauseHasTheRoundOfItsKind(t *testing.T) {
	t.Parallel()
	r := start(t, "paused")
	// A question only where the reply goes to the agent.
	for id, want := range map[string]string{
		"03": tickets.EventPauseQuestion,
		"04": tickets.EventPauseQuestion,
		"05": tickets.EventPauseRepair,
		"06": tickets.EventPauseQuestion,
		"07": tickets.EventPauseRepair,
		"08": tickets.EventPauseRepair,
		"09": tickets.EventPauseRepair,
	} {
		if got := r.roundTypes(t, "demo", "paused-epic", id); !slices.Equal(got, []string{want}) {
			t.Errorf("%s rounds = %v, want [%s]", id, got, want)
		}
	}
	if d := r.detail(t, pausedEpic+"09"); d.Recommendation == nil || d.Recommendation.Verb != server.ActionRetry {
		t.Errorf("09 recommendation = %+v, want retry", d.Recommendation)
	}
	repo := filepath.Join(filepath.Dir(filepath.Dir(r.StateDir)), "repo")
	if out, err := exec.Command("git", "-C", repo, "rev-parse", "--verify", "ralph-loop/paused-epic-item-05").CombinedOutput(); err != nil {
		t.Errorf("05 has no iteration branch: %v\n%s", err, out)
	}
	r.stop(t)
}

func TestDirtyTrunkScenario_RootPausedOnADirtyCheckout(t *testing.T) {
	t.Parallel()
	r := start(t, "dirty-trunk")
	repo := filepath.Join(filepath.Dir(filepath.Dir(r.StateDir)), "repo")
	status, err := exec.Command("git", "-C", repo, "status", "--porcelain").Output()
	if err != nil || !strings.Contains(string(status), "same.txt") {
		t.Errorf("trunk status = %q, %v; want a dirty same.txt", status, err)
	}
	var found bool
	for _, c := range r.cardsOf(t) {
		if c.Name == "dirty-epic" {
			found = true
			if c.MergeBlockKind != "dirty-trunk" || c.Section != server.SectionNeedsYou {
				t.Errorf("dirty-epic card = %+v, want a needs-you card blocked on dirty-trunk", c)
			}
		}
	}
	if !found {
		t.Error("no dirty-epic card")
	}
	r.stop(t)
}

func TestPausedScenario_WebRecoveryClearsEachPause(t *testing.T) {
	t.Parallel()
	r := start(t, "paused")
	w := r.web(t)

	// The route each verb's button posts to, as webui/src/ticket/recover.ts has it.
	routes := map[string]string{
		server.ActionAnswer:     "/v1/tickets/answer",
		server.ActionResume:     "/v1/tickets/resume",
		server.ActionChooseBase: "/v1/tickets/resume",
		server.ActionRetry:      "/v1/tickets/retry",
		server.ActionMerge:      "/v1/tickets/merge",
	}
	ids := []string{"03", "04", "05", "06", "07", "08", "09"}
	for _, id := range ids {
		d := r.detail(t, pausedEpic+id)
		if len(d.Actions) == 0 {
			t.Fatalf("%s has no actions: %+v", id, d)
		}
		verb := d.Actions[0]
		body := map[string]any{"address": d.Address, "state": d.State}
		switch verb {
		case server.ActionAnswer:
			body["text"] = "go ahead"
		case server.ActionChooseBase:
			body["base"] = "01"
		}
		w.post(t, routes[verb], body)
	}
	for _, id := range ids {
		// A merge lands the ticket and a retry relaunches it, so only the pause is checked.
		if d := r.detail(t, pausedEpic+id); d.Status == "needs-answer" || d.Status == "needs-repair" {
			t.Errorf("%s is still %q (reason %q) after its recovery", id, d.Status, d.PauseReason)
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
	t.Parallel()
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

	r.promptWhenIdle(t, id, "SQLite, and yes to the migration")
	chat = r.waitChat(t, id, func(c server.SessionChatResponse) bool { return c.Next != nil })
	if len(chat.Next.Action.Action) == 0 {
		t.Errorf("next = %+v, want the Continue action", chat.Next)
	}

	// Continue as the page sends it; the CLI's run-action needs the session's
	// Actionable, which an answer typed by the server does not clear.
	r.waitIdle(t, id)
	r.postWhenIdle(t, id, "/v1/sessions/prompt", map[string]any{"id": id, "continue": true, "last_message": chat.Messages[len(chat.Messages)-1].ID})
	r.waitChat(t, id, func(c server.SessionChatResponse) bool { return slices.Equal(roundLabels(c), []string{"9"}) })
	r.stop(t)
}

func TestDesignTwoLiveScenario_TwoSessionsNewestPlaysRoundOne(t *testing.T) {
	r := start(t, "design-two-live")
	var list []server.SessionInfo
	if err := json.Unmarshal([]byte(r.gx(t, "agents", "list", "--json")), &list); err != nil {
		t.Fatal(err)
	}
	var live []server.SessionInfo
	for _, s := range list {
		if s.Ticket == designAddr && s.Design {
			live = append(live, s)
		}
	}
	if len(live) != 2 {
		t.Fatalf("design sessions of %s = %+v, want 2", designAddr, list)
	}
	newest := live[0]
	if live[1].RunStartedAt.After(newest.RunStartedAt) {
		newest = live[1]
	}
	chat := r.waitChat(t, newest.ID, func(c server.SessionChatResponse) bool { return len(c.Round) > 0 })
	if got := roundLabels(chat); !slices.Equal(got, []string{"8", "8b"}) {
		t.Errorf("round labels = %v, want 8 and 8b", got)
	}
	r.stop(t)
}

// openDesign opens the design session of the scenario's map and returns its id.
func (r *running) openDesign(t *testing.T) string {
	t.Helper()
	var res server.SessionResult
	if err := json.Unmarshal([]byte(r.gx(t, "designs", "open", designAddr, "--json")), &res); err != nil || res.Session == nil {
		t.Fatalf("designs open = %+v, %v", res, err)
	}
	return res.Session.ID
}

// promptWhenIdle types text until it shows in the chat. The status the server
// reports lags the fake's turn, so a prompt sent right after a scripted message
// can be refused, or typed into a busy session and lost.
func (r *running) promptWhenIdle(t *testing.T, id, text string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		cmd := exec.Command(gxBin, "agents", "prompt", id, text)
		cmd.Env = r.Env
		if _, err := cmd.CombinedOutput(); err == nil {
			for range 10 {
				var chat server.SessionChatResponse
				if err := json.Unmarshal([]byte(r.gx(t, "agents", "chat", id, "--json")), &chat); err != nil {
					t.Fatal(err)
				}
				for _, m := range chat.Messages {
					if m.Role == transcript.ChatRolePerson && m.Text == text {
						return
					}
				}
				time.Sleep(200 * time.Millisecond)
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("session %s never took the prompt %q", id, text)
}

// agentTexts are the text of the session's assistant messages, oldest first.
func agentTexts(chat server.SessionChatResponse) []string {
	var texts []string
	for _, m := range chat.Messages {
		if m.Role == transcript.ChatRoleAgent && m.Kind == transcript.ChatKindText {
			texts = append(texts, m.Text)
		}
	}
	return texts
}

func TestDesignScripts_FakeAgentPlaysTheTurns(t *testing.T) {
	t.Parallel()
	const runEnd = "scripted run is over"
	tests := []struct {
		scenario string
		// want is the text each assistant message must contain, in order; the
		// test answers every message but the last.
		want []string
	}{
		{"design-plain", []string{"Q8", "thinking it over"}},
		{"design-branch", []string{runEnd}},
		{"design-error", []string{"Q8", "Error: the spec step failed"}},
		{"design-guard", slices.Repeat([]string{runEnd}, 6)},
	}
	for _, tt := range tests {
		t.Run(tt.scenario, func(t *testing.T) {
			t.Parallel()
			// A long countdown keeps the server from answering run-end lines itself.
			r := startWith(t, tt.scenario, nil, "--auto-advance-delay", "1h")
			id := r.openDesign(t)
			for i, want := range tt.want {
				r.waitChat(t, id, func(c server.SessionChatResponse) bool {
					texts := agentTexts(c)
					return len(texts) == i+1 && strings.Contains(texts[i], want)
				})
				if i < len(tt.want)-1 {
					r.promptWhenIdle(t, id, fmt.Sprintf("go on (%d)", i))
				}
			}
			r.stop(t)
		})
	}
}

func TestGrillingStatsScenario_StatsReturnTheSeededTotals(t *testing.T) {
	t.Parallel()
	r := start(t, "grilling-stats")
	tests := []struct {
		args             []string
		words, questions float64
	}{
		{[]string{"--range", "30"}, 550, 12},
		{[]string{"--range", "30", "--project", "demo"}, 310, 7},
		{[]string{"--range", "30", "--project", "other"}, 240, 5},
		{[]string{"--range", "7"}, 260, 6},
		{[]string{"--range", "7", "--project", "demo"}, 60, 2},
		{[]string{"--range", "7", "--project", "other"}, 200, 4},
	}
	for _, tt := range tests {
		var st server.Stats
		if err := json.Unmarshal([]byte(r.gx(t, append([]string{"server", "stats", "--json"}, tt.args...)...)), &st); err != nil {
			t.Fatal(err)
		}
		var words, questions float64
		for i := range st.Grilling.Words {
			words += st.Grilling.Words[i].Value
			questions += st.Grilling.Questions[i].Value
		}
		if words != tt.words || questions != tt.questions {
			t.Errorf("%v: words %v questions %v, want %v and %v", tt.args, words, questions, tt.words, tt.questions)
		}
	}

	// 30 days: index 0 is 29 days ago. Days 29..26 ago precede the earliest count.
	var st server.Stats
	if err := json.Unmarshal([]byte(r.gx(t, "server", "stats", "--json", "--range", "30")), &st); err != nil {
		t.Fatal(err)
	}
	for i, w := range st.Grilling.Words {
		daysAgo := 29 - i
		if want := daysAgo > 25; w.NoData != want {
			t.Errorf("%d days ago: no_data = %v, want %v", daysAgo, w.NoData, want)
		}
	}
	if w := st.Grilling.Words[29-24]; w.Value != 0 || w.NoData {
		t.Errorf("24 days ago = %+v, want a zero bar", w)
	}
	r.stop(t)
}

func TestStart_AutoAdvanceDelayFlagIsAccepted(t *testing.T) {
	t.Parallel()
	r := startWith(t, "design", nil, "--auto-advance-delay", "50ms")
	r.stop(t)
}

// checksOf is `gx server checks --json`, by project.
func (r *running) checksOf(t *testing.T) map[string]server.ProjectChecks {
	t.Helper()
	var st server.ChecksStatus
	if err := json.Unmarshal([]byte(r.gx(t, "server", "checks", "--json")), &st); err != nil {
		t.Fatal(err)
	}
	out := map[string]server.ProjectChecks{}
	for _, p := range st.Projects {
		out[p.Project] = p
	}
	return out
}

func kindOf(p server.ProjectChecks, kind string) server.KindCheck {
	for _, k := range p.Kinds {
		if k.Kind == kind {
			return k
		}
	}
	return server.KindCheck{}
}

func TestChecksMissingScenarioReportsBothProjectsMissing(t *testing.T) {
	t.Parallel()
	got := start(t, "checks-missing").checksOf(t)
	if !got["demo"].Missing || !got["auto"].Missing {
		t.Fatalf("want demo and auto missing, got %+v", got)
	}
	test := kindOf(got["demo"], "test")
	if test.Recommended == nil || test.Recommended.Source != "Makefile · test" {
		t.Errorf("demo test recommendation = %+v", test.Recommended)
	}
	if tc := kindOf(got["demo"], "typecheck"); tc.Recommended != nil {
		t.Errorf("demo typecheck should have no candidate, got %+v", tc.Recommended)
	}
}

func TestOtherScenariosReportNoMissingChecks(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"united", "empty"} {
		for p, c := range start(t, name).checksOf(t) {
			if c.Missing {
				t.Errorf("%s: project %s reports missing checks", name, p)
			}
		}
	}
}
