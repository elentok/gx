package main

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
)

var gxBin, fixtureBin, fakeBin, fakeCodexBin string

func TestMain(m *testing.M) {
	os.Exit(func() int {
		dir, err := os.MkdirTemp("", "gxfb")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		defer os.RemoveAll(dir)
		gxBin, fixtureBin, fakeBin = filepath.Join(dir, "gx"), filepath.Join(dir, "gxfixture"), filepath.Join(dir, "claude")
		fakeCodexBin = filepath.Join(dir, "codex")
		for out, pkg := range map[string]string{
			gxBin:        "github.com/elentok/gx",
			fixtureBin:   "github.com/elentok/gx/testutil/gxfixture",
			fakeBin:      "github.com/elentok/gx/testutil/agentfake/cmd/fakeclaude",
			fakeCodexBin: "github.com/elentok/gx/testutil/agentfake/cmd/fakecodex",
		} {
			if b, err := exec.Command("go", "build", "-tags", "gxfixture", "-o", out, pkg).CombinedOutput(); err != nil {
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
	return startWith(t, scenario, nil)
}

// startWith starts the scenario with extra gxfixture flags. env (KEY=VALUE)
// goes after the test's own env in the fixture's.
func startWith(t *testing.T, scenario string, env []string, flags ...string) *running {
	t.Helper()
	return launch(t, t.Cleanup, scenario, env, flags...)
}

// launch is startWith with the kill cleanup handed to register, for a fixture
// that outlives one test.
func launch(t *testing.T, register func(func()), scenario string, env []string, flags ...string) *running {
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
	register(r.kill)
	line, err := bufio.NewReader(stdout).ReadBytes('\n')
	if err != nil {
		t.Fatalf("no output line: %v", err)
	}
	if err := json.Unmarshal(line, &r.Output); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, line)
	}
	return r
}

// kill is the cleanup for a fixture its test did not stop.
func (r *running) kill() {
	if r.stopped {
		return
	}
	// SIGTERM lets the fixture stop its server and holders; a kill orphans them.
	_ = r.cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan struct{})
	go func() { _ = r.cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		_ = r.cmd.Process.Kill()
		<-done
	}
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
	var snap wire.Snapshot
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

func (r *running) waitStatus(t *testing.T, addr, want string) {
	t.Helper()
	if !testutil.Poll(90*time.Second, time.Second, func() bool { return r.ticketStatus(t, addr) == want }) {
		t.Logf("history:\n%s", r.gx(t, "server", "tickets", "history", addr))
		t.Fatalf("ticket %s is %q, want %s", addr, r.ticketStatus(t, addr), want)
	}
}

// webClient trusts the root the fixture's server wrote to its state dir. It
// carries the fixture's login cookie when signedIn.
func (r *running) webClient(t *testing.T, signedIn bool) *http.Client {
	t.Helper()
	pemBytes, err := os.ReadFile(filepath.Join(r.StateDir, "web-tls", "root.pem"))
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pemBytes) {
		t.Fatal("web-tls/root.pem has no certificate")
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	if signedIn {
		u, err := url.Parse(r.WebURL)
		if err != nil {
			t.Fatal(err)
		}
		jar.SetCookies(u, []*http.Cookie{{Name: r.CookieName, Value: r.Cookie, Path: "/", Secure: true}})
	}
	return &http.Client{Jar: jar, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots}}}
}

func (r *running) get(t *testing.T, url string, signedIn bool) int {
	t.Helper()
	resp, err := r.webClient(t, signedIn).Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

// web is a client of the fixture's web server, signed in with the fixture's cookie.
type web struct {
	client *http.Client
	origin string
}

func (r *running) web(t *testing.T) *web {
	t.Helper()
	c := r.webClient(t, true)
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

func (r *running) detail(t *testing.T, address string) wire.TicketDetail {
	t.Helper()
	var d wire.TicketDetail
	if err := json.Unmarshal([]byte(r.gx(t, "server", "tickets", "detail", address, "--json")), &d); err != nil {
		t.Fatal(err)
	}
	return d
}

// waitPermission waits for the approve ticket's agent to block on a prompt the web can approve.
func (r *running) waitPermission(t *testing.T, address string) wire.TicketDetail {
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
