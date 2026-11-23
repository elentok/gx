package deps

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// setupResult is what installAndSetupWithin returns, for assertions.
type setupResult struct {
	commands               []string
	installTime, setupTime time.Duration
	hookCalls              int
	err                    error
}

func runSetupWithin(t *testing.T, repo, binDir, logDir string, budget time.Duration, setup []string, onStart func()) setupResult {
	t.Helper()
	var r setupResult
	// The stub dir leads PATH; setup commands still need sleep, basename and touch.
	binDir += ":/bin:/usr/bin"
	r.commands, r.installTime, r.setupTime, r.err = installAndSetupWithin(context.Background(), repo, binDir, logDir, budget, setup, onStart)
	return r
}

func lockfileRepo(t *testing.T) string {
	t.Helper()
	return installRepo(t, map[string]string{"package.json": pkgJSON, "package-lock.json": "{}"}, nil)
}

func TestInstallDependencies_Setup_RunsAfterTheInstallInOrder(t *testing.T) {
	t.Parallel()
	repo := lockfileRepo(t)
	binDir, logPath := stubManagers(t, 0, "npm")
	setup := []string{"echo first-$(basename $(pwd)) >> " + logPath, "echo second >> " + logPath}

	r := runSetupWithin(t, repo, binDir, "", installTimeout, setup, nil)
	if r.err != nil {
		t.Fatalf("error = %v", r.err)
	}
	got := readLog(t, logPath)
	if len(got) != 3 || !strings.Contains(got[0], "npm ci") ||
		got[1] != "first-"+filepath.Base(repo) || got[2] != "second" {
		t.Errorf("log = %q, want the install, then the setup commands in order inside the worktree", got)
	}
	if n := len(r.commands); n != 3 || r.commands[1] != "setup: "+setup[0] || r.commands[2] != "setup: "+setup[1] {
		t.Errorf("commands = %q, want the install entry then one setup-marked entry per command", r.commands)
	}
}

func TestInstallDependencies_Setup_FailureNamesCommandTailAndLog(t *testing.T) {
	t.Parallel()
	repo := lockfileRepo(t)
	binDir, _ := stubManagers(t, 0, "npm")
	logDir := t.TempDir()
	command := "echo setup-output; exit 3"

	r := runSetupWithin(t, repo, binDir, logDir, installTimeout, []string{command}, nil)
	if r.err == nil {
		t.Fatal("error = nil, want the setup failure")
	}
	msg := r.err.Error()
	prefix := "running setup in " + repo + ": " + command + ": "
	if !strings.HasPrefix(msg, prefix) || !strings.Contains(msg, "setup-output") || strings.Contains(msg, "installing dependencies") {
		t.Fatalf("error = %q, want prefix %q, the output tail, no install wording", msg, prefix)
	}
	_, logPath, found := strings.Cut(strings.TrimSuffix(msg, ")"), "(full output: ")
	if !found || filepath.Dir(logPath) != logDir {
		t.Fatalf("error = %q, want a full-output path inside %s", msg, logDir)
	}
	if raw, err := os.ReadFile(logPath); err != nil || !strings.Contains(string(raw), "setup-output") {
		t.Errorf("log file = %q, %v; want the full output", raw, err)
	}
}

func TestInstallDependencies_Setup_SharesTheInstallBudget(t *testing.T) {
	t.Parallel()
	repo := lockfileRepo(t)
	binDir, _ := stubManagers(t, 0, "npm")

	start := time.Now()
	// The budget must outlast a stub install on a loaded machine, or the install times out instead.
	r := runSetupWithin(t, repo, binDir, "", 2*time.Second, []string{"sleep 10"}, nil)
	want := "running setup in " + repo + ": sleep 10: timed out after 5m"
	if r.err == nil || r.err.Error() != want {
		t.Fatalf("error = %v, want %q", r.err, want)
	}
	if elapsed := time.Since(start); elapsed > 6*time.Second {
		t.Errorf("took %v, want the command killed near the budget", elapsed)
	}
}

func TestInstallDependencies_Setup_HangingCommandIsCancelled(t *testing.T) {
	t.Parallel()
	repo := lockfileRepo(t)
	binDir, _ := stubManagers(t, 0, "npm")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cancelSoon := func() { time.AfterFunc(300*time.Millisecond, cancel) }

	start := time.Now()
	_, _, _, err := installAndSetupWithin(ctx, repo, binDir+":/bin:/usr/bin", "", time.Minute, []string{"sleep 5"}, cancelSoon)
	want := "running setup in " + repo + ": sleep 5: install cancelled"
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("took %v, want the cancel to kill the command promptly", elapsed)
	}
}

func TestInstallDependencies_NoSetup_RunsNothingAndReportsZeroSetupTime(t *testing.T) {
	t.Parallel()
	repo := lockfileRepo(t)
	binDir, logPath := stubManagers(t, 0, "npm")
	hook := 0

	r := runSetupWithin(t, repo, binDir, "", installTimeout, nil, func() { hook++ })
	if r.err != nil {
		t.Fatalf("error = %v", r.err)
	}
	if hook != 0 || r.setupTime != 0 {
		t.Errorf("hook calls = %d, setup time = %v, want 0 and 0", hook, r.setupTime)
	}
	if len(r.commands) != 1 || len(readLog(t, logPath)) != 1 {
		t.Errorf("commands = %q, log = %q, want the install only", r.commands, readLog(t, logPath))
	}
}

func TestInstallDependencies_Setup_ReportsInstallAndSetupDurations(t *testing.T) {
	t.Parallel()
	repo := lockfileRepo(t)
	binDir := t.TempDir()
	writeScriptStub(t, binDir, "npm", "sleep 0.3")

	r := runSetupWithin(t, repo, binDir, "", installTimeout, []string{"sleep 0.3"}, nil)
	if r.err != nil {
		t.Fatalf("error = %v", r.err)
	}
	if r.installTime < 300*time.Millisecond || r.setupTime < 300*time.Millisecond {
		t.Errorf("install = %v, setup = %v, want each at least the 300ms its command slept", r.installTime, r.setupTime)
	}
}

func TestInstallDependencies_Setup_OnSetupStartFiresOnceBetweenInstallAndSetup(t *testing.T) {
	t.Parallel()
	repo := lockfileRepo(t)
	binDir, logPath := stubManagers(t, 0, "npm")
	var seen []string
	hook := func() { seen = append(seen, strings.Join(readLog(t, logPath), "|")) }

	r := runSetupWithin(t, repo, binDir, "", installTimeout,
		[]string{"echo one >> " + logPath, "echo two >> " + logPath}, hook)
	if r.err != nil {
		t.Fatalf("error = %v", r.err)
	}
	if len(seen) != 1 || strings.Count(seen[0], "|") != 0 || !strings.Contains(seen[0], "npm ci") {
		t.Errorf("hook saw logs %q, want one call with only the install logged", seen)
	}
}

func TestInstallDependencies_FailedInstall_SkipsSetup(t *testing.T) {
	t.Parallel()
	repo := lockfileRepo(t)
	binDir, _ := stubManagers(t, 1, "npm")
	marker := filepath.Join(t.TempDir(), "marker")
	hook := 0

	r := runSetupWithin(t, repo, binDir, "", installTimeout, []string{"touch " + marker}, func() { hook++ })
	if r.err == nil || !strings.HasPrefix(r.err.Error(), "installing dependencies in ") {
		t.Fatalf("error = %v, want the install failure", r.err)
	}
	if _, err := os.Stat(marker); err == nil || hook != 0 {
		t.Errorf("setup ran after a failed install (marker err = %v, hook = %d)", err, hook)
	}
}

func TestInstallDependencies_NoLockfile_StillRunsSetup(t *testing.T) {
	t.Parallel()
	repo := installRepo(t, map[string]string{"README.md": "x"}, nil)
	marker := filepath.Join(t.TempDir(), "marker")

	r := runSetupWithin(t, repo, "", "", installTimeout, []string{"touch " + marker}, nil)
	if r.err != nil {
		t.Fatalf("error = %v", r.err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("marker missing: setup did not run without a lockfile (%v)", err)
	}
}
