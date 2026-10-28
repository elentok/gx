package agentfake

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/testutil"
)

// workerEnv is a repo to commit in and a gx stand-in that logs its arguments
// and stdin.
func workerEnv(t *testing.T) (cfg WorkerConfig, gxLog string) {
	t.Helper()
	repo := testutil.TempRepo(t)
	t.Chdir(repo)
	dir := t.TempDir()
	gxLog = filepath.Join(dir, "gx.log")
	gx := filepath.Join(dir, "gx")
	script := "#!/bin/sh\necho \"$@\" >> " + gxLog + "\ncat >> " + gxLog + "\n"
	if err := os.WriteFile(gx, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return WorkerConfig{Gx: gx}, gxLog
}

func commitCount(t *testing.T) int {
	t.Helper()
	out, err := exec.Command("git", "rev-list", "--count", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, c := range strings.TrimSpace(string(out)) {
		n = n*10 + int(c-'0')
	}
	return n
}

func readLog(t *testing.T, path string) string {
	t.Helper()
	data, _ := os.ReadFile(path)
	return string(data)
}

func TestWorkTicket_FinishCommitsAndReports(t *testing.T) {
	cfg, gxLog := workerEnv(t)
	before := commitCount(t)
	if err := WorkTicket(cfg, "p:e/01"); err != nil {
		t.Fatal(err)
	}
	if got := commitCount(t); got != before+1 {
		t.Errorf("commits = %d, want %d", got, before+1)
	}
	if got := readLog(t, gxLog); !strings.Contains(got, "tickets set p:e/01 --iteration-status finished") {
		t.Errorf("gx log = %q, want a finished report", got)
	}
}

// The finish guard (cmd/tickets_finish_guard.go) wants a passing `gx check` on
// the committed tree before a finished report.
func TestWorkTicket_FinishChecksTheCommittedTreeBeforeReporting(t *testing.T) {
	cfg, gxLog := workerEnv(t)
	if err := WorkTicket(cfg, "p:e/01"); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(readLog(t, gxLog)), "\n")
	if len(lines) != 2 || lines[0] != "check" || !strings.HasPrefix(lines[1], "tickets set p:e/01 --iteration-status finished") {
		t.Errorf("gx calls = %q, want check then the finished report", lines)
	}
}

func TestWorkTicket_FinishAcceptsACheckWithNothingToRun(t *testing.T) {
	cfg, gxLog := workerEnv(t)
	script := "#!/bin/sh\necho \"$@\" >> " + gxLog + "\n[ \"$1\" = check ] && exit 3\nexit 0\n"
	if err := os.WriteFile(cfg.Gx, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WorkTicket(cfg, "p:e/01"); err != nil {
		t.Fatal(err)
	}
	if got := readLog(t, gxLog); !strings.Contains(got, "--iteration-status finished") {
		t.Errorf("gx log = %q, want a finished report", got)
	}
}

func TestWorkTicket_FailCommitsAndReportsNothing(t *testing.T) {
	cfg, gxLog := workerEnv(t)
	cfg.Behaviour = map[string]string{"p:e/01": BehaviourFail}
	before := commitCount(t)
	if err := WorkTicket(cfg, "p:e/01"); err != nil {
		t.Fatal(err)
	}
	if got := commitCount(t); got != before {
		t.Errorf("commits = %d, want %d", got, before)
	}
	if got := readLog(t, gxLog); got != "" {
		t.Errorf("gx log = %q, want nothing", got)
	}
}

func TestWorkTicket_AskSendsAQuestionAndCommitsNothing(t *testing.T) {
	cfg, gxLog := workerEnv(t)
	cfg.Behaviour = map[string]string{"p:e/01": BehaviourAsk}
	before := commitCount(t)
	if err := WorkTicket(cfg, "p:e/01"); err != nil {
		t.Fatal(err)
	}
	if got := commitCount(t); got != before {
		t.Errorf("commits = %d, want %d", got, before)
	}
	got := readLog(t, gxLog)
	if !strings.Contains(got, "tickets ask p:e/01 --body -") || !strings.Contains(got, "## Question") || !strings.Contains(got, "## Handoff") {
		t.Errorf("gx log = %q, want a question with a handoff", got)
	}
}

func TestWorkTicket_HoldDelaysOnlyTheFinishAndTimesOut(t *testing.T) {
	cfg, gxLog := workerEnv(t)
	cfg.Hold = filepath.Join(t.TempDir(), "hold")
	cfg.HoldTimeout = "600ms"
	if err := os.WriteFile(cfg.Hold, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- WorkTicket(cfg, "p:e/01") }()

	// The launch is not delayed: the worker is up and marks the prompt as received.
	deadline := time.Now().Add(5 * time.Second)
	for !fileExists(cfg.Hold + StartedSuffix) {
		if time.Now().After(deadline) {
			t.Fatal("worker never marked the prompt as received")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := readLog(t, gxLog); got != "" {
		t.Fatalf("gx log = %q while held, want nothing", got)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < 600*time.Millisecond {
		t.Errorf("finished after %v, want it held until the timeout", time.Since(start))
	}
	if got := readLog(t, gxLog); !strings.Contains(got, "--iteration-status finished") {
		t.Errorf("gx log = %q, want a finished report after the timeout", got)
	}
}
