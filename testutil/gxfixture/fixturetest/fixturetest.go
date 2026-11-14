// Package fixturetest is the test harness for gxfixture's scenario tests: it
// builds the fixture, gx and the fake agents once per test binary, starts a
// scenario as a subprocess and gives the test its output and web client.
package fixturetest

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"flag"
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
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/elentok/gx/procwatch"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/syshealth"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/testutil/gxfixture/spec"
)

// The binaries Run builds: gx, the fixture program, fakeclaude, fakecodex and the fake gh.
var GxBin, FixtureBin, FakeBin, FakeCodexBin, FakeGHBin string

// RunMatches reports whether a -run filter lets a test whose name contains
// part run: true when there is no filter. A package's shared fixture starts
// only when it does. Call it from Run's setup, where flags are parsed.
func RunMatches(part string) bool {
	f := flag.Lookup("test.run")
	return f == nil || f.Value.String() == "" || strings.Contains(f.Value.String(), part)
}

// Run is a test package's TestMain: it builds the binaries, runs setup (which
// starts the package's shared fixtures, outside every test's time limit) and
// then the tests. setup may be nil; its teardown runs after the tests.
func Run(m *testing.M, setup func() (teardown func(), err error)) int {
	dir, err := makeTempDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(dir)
	GxBin, FixtureBin, FakeBin = filepath.Join(dir, "gx"), filepath.Join(dir, "gxfixture"), filepath.Join(dir, "claude")
	FakeCodexBin, FakeGHBin = filepath.Join(dir, "codex"), filepath.Join(dir, "gh")
	for out, pkg := range map[string]string{
		FakeGHBin:    "github.com/elentok/gx/testutil/ghfake/cmd/gh",
		GxBin:        "github.com/elentok/gx",
		FixtureBin:   "github.com/elentok/gx/testutil/gxfixture",
		FakeBin:      "github.com/elentok/gx/testutil/agentfake/cmd/fakeclaude",
		FakeCodexBin: "github.com/elentok/gx/testutil/agentfake/cmd/fakecodex",
	} {
		if b, err := exec.Command("go", "build", "-tags", "gxfixture", "-o", out, pkg).CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "go build %s: %v\n%s", pkg, err, b)
			return 1
		}
	}
	flag.Parse() // test.run is unset until flags are parsed
	if setup != nil {
		teardown, err := setup()
		if teardown != nil {
			defer teardown()
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	return m.Run()
}

// Running is a started fixture.
type Running struct {
	cmd     *exec.Cmd
	stopped bool
	spec.Output
}

// Start starts the scenario.
func Start(t *testing.T, scenario string) *Running {
	t.Helper()
	return StartWith(t, scenario, nil)
}

// StartWith starts the scenario with extra gxfixture flags. env (KEY=VALUE)
// goes after the test's own env in the fixture's.
func StartWith(t *testing.T, scenario string, env []string, flags ...string) *Running {
	t.Helper()
	return Launch(t, t.Cleanup, scenario, env, flags...)
}

// Launch is startWith with the kill cleanup handed to register, for a fixture
// that outlives one test.
func Launch(t *testing.T, register func(func()), scenario string, env []string, flags ...string) *Running {
	t.Helper()
	r, err := LaunchProc(register, scenario, env, flags...)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// startTimeout is how long a fixture may take to print its output line; it must
// stay under the runner's per-test limit so the failure names the stuck step.
const startTimeout = 40 * time.Second

// LaunchProc is launch without a test, so TestMain can start a shared fixture
// outside every test's time limit.
func LaunchProc(register func(func()), scenario string, env []string, flags ...string) (*Running, error) {
	// Startup is the fork-heavy phase; a parallel test run starting every
	// fixture at once exhausted the user's process limit.
	release, err := acquireLaunchSlot(filepath.Join(os.TempDir(), "gx-fixture-launch"), maxParallelLaunches)
	if err != nil {
		return nil, err
	}
	defer release()
	r, err := launchProc(register, scenario, env, flags...)
	return r, explainStartErr(err)
}

// maxParallelLaunches caps the fixtures starting at once, shared across every
// test binary on the machine (flock slot files), not just one process.
const maxParallelLaunches = 4

// launchPoll is how long acquireLaunchSlot sleeps when every slot is taken.
const launchPoll = 20 * time.Millisecond

// acquireLaunchSlot takes one of slots flock files in dir, waiting while all
// are held. The kernel drops the lock when the process dies, so a crashed
// binary leaks nothing. The returned release is safe to call more than once.
func acquireLaunchSlot(dir string, slots int) (release func(), err error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	files := make([]*os.File, slots)
	for i := range files {
		if files[i], err = os.OpenFile(filepath.Join(dir, fmt.Sprintf("slot-%d", i)), os.O_CREATE|os.O_RDWR, 0o644); err != nil {
			for _, f := range files[:i] {
				f.Close()
			}
			return nil, err
		}
	}
	for {
		for i, f := range files {
			if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
				continue
			}
			for j, other := range files {
				if j != i {
					other.Close()
				}
			}
			var once sync.Once
			return func() { once.Do(func() { f.Close() }) }, nil
		}
		time.Sleep(launchPoll)
	}
}

// explainStartErr names the process limit when fork/exec hit it, so a mass
// failure reads as one cause instead of many unrelated test failures.
func explainStartErr(err error) error {
	if errors.Is(err, syscall.EAGAIN) {
		return fmt.Errorf("process limit hit (EAGAIN): the machine ran out of processes; lower test parallelism or close other work: %w", err)
	}
	return err
}

func launchProc(register func(func()), scenario string, env []string, flags ...string) (*Running, error) {
	args := append([]string{"--gx", GxBin, "--fakeclaude", FakeBin, "--fakegh", FakeGHBin}, flags...)
	cmd := exec.Command(FixtureBin, append(args, scenario)...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	r := &Running{cmd: cmd}
	register(r.Kill)
	// A fixture that never prints its line would hang the test until the
	// runner's per-test limit kills it with no diagnosis. SIGQUIT makes it dump
	// its goroutines to stderr, so the next occurrence names the stuck step.
	timer := time.AfterFunc(startTimeout, func() { _ = cmd.Process.Signal(syscall.SIGQUIT) })
	line, err := bufio.NewReader(stdout).ReadBytes('\n')
	if !timer.Stop() {
		return r, fmt.Errorf("fixture printed no output line within %s; its goroutine dump is on stderr", startTimeout)
	}
	if err != nil {
		return r, fmt.Errorf("no output line: %v", err)
	}
	if err := json.Unmarshal(line, &r.Output); err != nil {
		return r, fmt.Errorf("output is not JSON: %v\n%s", err, line)
	}
	return r, nil
}

// Kill is the cleanup for a fixture its test did not stop.
func (r *Running) Kill() {
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

// Stop sends SIGTERM and waits for the fixture to exit.
func (r *Running) Stop(t *testing.T) {
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

func (r *Running) Gx(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command(GxBin, args...)
	cmd.Env = r.Env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gx %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func (r *Running) TicketStatus(t *testing.T, address string) string {
	t.Helper()
	var snap wire.Snapshot
	if err := json.Unmarshal([]byte(r.Gx(t, "server", "snapshot", "--json")), &snap); err != nil {
		t.Fatal(err)
	}
	for _, tk := range snap.Tickets {
		if tk.Address == address {
			return tk.Status
		}
	}
	return ""
}

func (r *Running) WaitStatus(t *testing.T, addr, want string) {
	t.Helper()
	if !testutil.Poll(90*time.Second, time.Second, func() bool { return r.TicketStatus(t, addr) == want }) {
		t.Logf("history:\n%s", r.Gx(t, "server", "tickets", "history", addr))
		t.Fatalf("ticket %s is %q, want %s", addr, r.TicketStatus(t, addr), want)
	}
}

// WebClient trusts the root the fixture's server wrote to its state dir. It
// carries the fixture's login cookie when signedIn.
func (r *Running) WebClient(t *testing.T, signedIn bool) *http.Client {
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

func (r *Running) Get(t *testing.T, url string, signedIn bool) int {
	t.Helper()
	resp, err := r.WebClient(t, signedIn).Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

// Web is a client of the fixture's web server, signed in with the fixture's cookie.
type Web struct {
	client *http.Client
	origin string
}

func (r *Running) Web(t *testing.T) *Web {
	t.Helper()
	c := r.WebClient(t, true)
	u, _ := url.Parse(r.WebURL)
	return &Web{client: c, origin: u.Scheme + "://" + u.Host}
}

// Get reads a JSON route into into and fails on any status but 200.
func (w *Web) Get(t *testing.T, path string, into any) {
	t.Helper()
	resp, err := w.client.Get(w.origin + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: %d: %s", path, resp.StatusCode, raw)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
}

// Post sends a write as the web page does and fails on a refusal.
func (w *Web) Post(t *testing.T, path string, body any) {
	t.Helper()
	if data, ok := w.Try(t, path, body); !ok {
		t.Fatalf("POST %s refused: %s", path, data)
	}
}

// Try posts once and reports whether the server took the write; data is the response.
func (w *Web) Try(t *testing.T, path string, body any) (data string, ok bool) {
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

func (r *Running) Detail(t *testing.T, address string) wire.TicketDetail {
	t.Helper()
	var d wire.TicketDetail
	if err := json.Unmarshal([]byte(r.Gx(t, "server", "tickets", "detail", address, "--json")), &d); err != nil {
		t.Fatal(err)
	}
	return d
}

// WaitPermission waits for the approve ticket's agent to block on a prompt the web can approve.
func (r *Running) WaitPermission(t *testing.T, address string) wire.TicketDetail {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for {
		d := r.Detail(t, address)
		if d.Permission != nil && d.Permission.RequestID != "" {
			return d
		}
		if time.Now().After(deadline) {
			t.Fatalf("no approvable permission prompt on %s; detail = %+v", address, d)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// Root is the fixture's temp directory: home, repo, data and bin sit under it.
func (r *Running) Root() string { return filepath.Dir(filepath.Dir(r.StateDir)) }

// Repo is the fixture's demo repository.
func (r *Running) Repo() string { return filepath.Join(r.Root(), "repo") }

// Home is the fixture's HOME.
func (r *Running) Home() string { return filepath.Join(r.Root(), "home") }

// ProcessCounts counts the live agent-holder processes of the fixture's saved
// sessions and the live agents: the descendants of those holders (the agent
// command line does not name its session id, but it runs under its holder).
func (r *Running) ProcessCounts(t *testing.T) (holders, agents int) {
	t.Helper()
	var list []wire.SessionInfo
	if err := json.Unmarshal([]byte(r.Gx(t, "agents", "list", "--json")), &list); err != nil {
		t.Fatal(err)
	}
	rows := testutil.PsRows(t, r.Env)
	procs := make([]syshealth.Process, len(rows))
	for i, row := range rows {
		procs[i] = syshealth.Process{PID: row.PID, PPID: row.PPID}
	}
	forest := procwatch.NewForest(procs)
	for _, row := range rows {
		if !strings.Contains(row.Command, "agent-holder") {
			continue
		}
		if slices.ContainsFunc(list, func(s wire.SessionInfo) bool { return strings.Contains(row.Command, s.ID) }) {
			holders++
			agents += len(forest.Descendants(row.PID))
		}
	}
	return holders, agents
}

// KillSessionProcesses SIGKILLs every holder and agent process naming one of
// the session ids, and their descendants (the agent does not name its id but
// inherits the holder's socket, which then still accepts), leaving the sessions
// as a crashed holder would. It does not need the server, which is why the
// caller passes the ids. It returns once no such process is left.
func (r *Running) KillSessionProcesses(t *testing.T, ids []string) {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		children := map[int][]int{}
		var doomed []int
		for _, row := range testutil.PsRows(t, r.Env) {
			children[row.PPID] = append(children[row.PPID], row.PID)
			if slices.ContainsFunc(ids, func(id string) bool { return strings.Contains(row.Command, id) }) {
				doomed = append(doomed, row.PID)
			}
		}
		for i := 0; i < len(doomed); i++ {
			doomed = append(doomed, children[doomed[i]]...)
		}
		killed := 0
		for _, pid := range doomed {
			if syscall.Kill(pid, syscall.SIGKILL) == nil {
				killed++
			}
		}
		if len(doomed) == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d session processes survived SIGKILL", killed)
		}
	}
}

// Sleepers counts the live processes of the fake agents' forked trees on the
// host. Compare it with a count taken before the storm: another fixture's
// tree would add to it.
func (r *Running) Sleepers(t *testing.T) int {
	t.Helper()
	n := 0
	for _, line := range testutil.PsLines(t, os.Environ()) {
		if strings.TrimSpace(line) == agentfake.SleeperCommand {
			n++
		}
	}
	return n
}

// SetDisk replaces a fake-disk control file through a rename, as a flow does.
func (r *Running) SetDisk(t *testing.T, name, value string) {
	t.Helper()
	tmp := filepath.Join(r.DiskDir, name+".tmp")
	if err := os.WriteFile(tmp, []byte(value+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, filepath.Join(r.DiskDir, name)); err != nil {
		t.Fatal(err)
	}
}
