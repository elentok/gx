package agentfake

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func readLog(t *testing.T, path string) string {
	t.Helper()
	data, _ := os.ReadFile(path)
	return string(data)
}

func TestWorkTicket_FinishCommitsAndReports(t *testing.T) {
	cfg, gxLog := WorkerEnv(t)
	before := CommitCount(t)
	if err := WorkTicket(cfg, "p:e/01"); err != nil {
		t.Fatal(err)
	}
	if got := CommitCount(t); got != before+1 {
		t.Errorf("commits = %d, want %d", got, before+1)
	}
	if got := readLog(t, gxLog); !strings.Contains(got, "tickets set p:e/01 --iteration-status finished") {
		t.Errorf("gx log = %q, want a finished report", got)
	}
}

// The finish guard (cmd/tickets_finish_guard.go) wants a passing `gx check` on
// the committed tree before a finished report.
func TestWorkTicket_FinishChecksTheCommittedTreeBeforeReporting(t *testing.T) {
	cfg, gxLog := WorkerEnv(t)
	if err := WorkTicket(cfg, "p:e/01"); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(readLog(t, gxLog)), "\n")
	if len(lines) != 2 || lines[0] != "check" || !strings.HasPrefix(lines[1], "tickets set p:e/01 --iteration-status finished") {
		t.Errorf("gx calls = %q, want check then the finished report", lines)
	}
}

func TestWorkTicket_FinishAcceptsACheckWithNothingToRun(t *testing.T) {
	cfg, gxLog := WorkerEnv(t)
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
	cfg, gxLog := WorkerEnv(t)
	cfg.Behaviour = map[string]string{"p:e/01": BehaviourFail}
	before := CommitCount(t)
	if err := WorkTicket(cfg, "p:e/01"); err != nil {
		t.Fatal(err)
	}
	if got := CommitCount(t); got != before {
		t.Errorf("commits = %d, want %d", got, before)
	}
	if got := readLog(t, gxLog); got != "" {
		t.Errorf("gx log = %q, want nothing", got)
	}
}

func TestWorkTicket_AskSendsAQuestionAndCommitsNothing(t *testing.T) {
	cfg, gxLog := WorkerEnv(t)
	cfg.Behaviour = map[string]string{"p:e/01": BehaviourAsk}
	before := CommitCount(t)
	if err := WorkTicket(cfg, "p:e/01"); err != nil {
		t.Fatal(err)
	}
	if got := CommitCount(t); got != before {
		t.Errorf("commits = %d, want %d", got, before)
	}
	got := readLog(t, gxLog)
	if !strings.Contains(got, "tickets ask p:e/01 --body -") || !strings.Contains(got, "## Question") || !strings.Contains(got, "## Handoff") {
		t.Errorf("gx log = %q, want a question with a handoff", got)
	}
}

func TestWorkTicket_RefuseAskCommitsGetsRefusedThenAsks(t *testing.T) {
	cfg, gxLog := WorkerEnv(t)
	cfg.Behaviour = map[string]string{"p:e/01": BehaviourRefuseAsk}
	script := "#!/bin/sh\necho \"$@\" >> " + gxLog + "\n[ \"$2\" = set ] && { echo refused >&2; exit 1; }\ncat >> " + gxLog + "\n"
	if err := os.WriteFile(cfg.Gx, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	before := CommitCount(t)
	if err := WorkTicket(cfg, "p:e/01"); err != nil {
		t.Fatal(err)
	}
	if got := CommitCount(t); got != before+1 {
		t.Errorf("commits = %d, want %d", got, before+1)
	}
	lines := strings.Split(strings.TrimSpace(readLog(t, gxLog)), "\n")
	if len(lines) < 2 || !strings.HasPrefix(lines[0], "tickets set p:e/01 --iteration-status finished") || !strings.HasPrefix(lines[1], "tickets ask p:e/01 --body -") {
		t.Errorf("gx calls = %q, want the refused finish and then the question", lines)
	}
}

func TestWorkTicket_RefuseAskFailsWhenTheFinishIsAccepted(t *testing.T) {
	cfg, _ := WorkerEnv(t)
	cfg.Behaviour = map[string]string{"p:e/01": BehaviourRefuseAsk}
	if err := WorkTicket(cfg, "p:e/01"); err == nil {
		t.Fatal("want an error when the finish guard does not refuse")
	}
}

func TestWorkTicket_HoldDelaysOnlyTheFinishAndTimesOut(t *testing.T) {
	cfg, gxLog := WorkerEnv(t)
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
