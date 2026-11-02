package scripts_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// soakRepo makes a temp git repo whose .gx/checks.json runs the script body as test.full.
func soakRepo(t *testing.T, body string) string {
	t.Helper()
	repo := t.TempDir()
	gitIn(t, repo, "init", "-q", "-b", "main")
	writeFile(t, filepath.Join(repo, "fake.sh"), body)
	writeFile(t, filepath.Join(repo, ".gx", "checks.json"), `{"test":{"full":"bash fake.sh"}}`)
	return repo
}

var (
	soakGXOnce sync.Once
	soakGXDir  string
	soakGXErr  error
)

// soakGX builds gx once per test run; the script runs the gx on PATH.
func soakGX(t *testing.T) string {
	t.Helper()
	soakGXOnce.Do(func() {
		soakGXDir, soakGXErr = os.MkdirTemp("", "soak-gx-")
		if soakGXErr != nil {
			return
		}
		build := exec.Command("go", "build", "-o", filepath.Join(soakGXDir, "gx"), ".")
		build.Dir = ".."
		if out, err := build.CombinedOutput(); err != nil {
			soakGXErr = fmt.Errorf("go build: %v\n%s", err, out)
		}
	})
	if soakGXErr != nil {
		t.Fatal(soakGXErr)
	}
	return soakGXDir
}

func runSoak(t *testing.T, repo string, args ...string) (string, int) {
	t.Helper()
	script, err := filepath.Abs("soak.sh")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", append([]string{script}, args...)...)
	cmd.Dir = repo
	cmd.Env = append(os.Environ(),
		"PATH="+soakGX(t)+string(os.PathListSeparator)+os.Getenv("PATH"),
		"XDG_DATA_HOME="+t.TempDir(),
		"XDG_STATE_HOME="+t.TempDir())
	out, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return string(out), exitErr.ExitCode()
	}
	if err != nil {
		t.Fatalf("soak script: %v\n%s", err, out)
	}
	return string(out), 0
}

func TestSoak_CountsRoundsPerFailingTest(t *testing.T) {
	repo := soakRepo(t, `
n=$(( $(cat count 2>/dev/null || echo 0) + 1 ))
echo $n > count
printf 'FAIL go-test: 1 failed in 1s\npkg/a\n  TestAlways\n    boom\n/tmp/log\n'
if [[ $n -eq 1 ]]; then
  printf 'FAIL go-test: 1 failed in 1s\npkg/b\n  TestOnce\n    boom\n/tmp/log\n'
fi
exit 1
`)

	out, code := runSoak(t, repo, "2")

	if code == 0 {
		t.Error("want non-zero exit")
	}
	for _, want := range []string{"round 1: FAIL", "round 2: FAIL", "2× TestAlways", "1× TestOnce"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if got := strings.Count(out, "TestAlways"); got != 1 {
		t.Errorf("TestAlways listed %d times, want 1:\n%s", got, out)
	}
}

func TestSoak_KeepsEachRoundsOutput(t *testing.T) {
	repo := soakRepo(t, "echo hello\nexit 1\n")

	out, _ := runSoak(t, repo, "2")

	dir := ""
	for l := range strings.SplitSeq(out, "\n") {
		if d, ok := strings.CutPrefix(l, "output files: "); ok {
			dir = d
		}
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	for _, name := range []string{"round-1.log", "round-2.log"} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || !strings.Contains(string(b), "hello") {
			t.Errorf("%s: %v %q", name, err, b)
		}
	}
	if !strings.Contains(out, "round 1 failed, no test names") {
		t.Errorf("want the no-names line:\n%s", out)
	}
}

func TestSoak_PassingCommandExitsZeroAndListsNoTests(t *testing.T) {
	repo := soakRepo(t, "echo ok\n")

	out, code := runSoak(t, repo, "2")

	if code != 0 {
		t.Errorf("exit = %d:\n%s", code, out)
	}
	if !strings.Contains(out, "round 2: pass") || strings.Contains(out, "×") {
		t.Errorf("output:\n%s", out)
	}
}

func TestSoak_DefaultsToTwentyRounds(t *testing.T) {
	repo := soakRepo(t, "echo x >> calls\n")

	runSoak(t, repo)

	b, err := os.ReadFile(filepath.Join(repo, "calls"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(b), "x"); got != 20 {
		t.Errorf("command ran %d times, want 20", got)
	}
}

const failInRound2 = `
n=$(( $(cat count 2>/dev/null || echo 0) + 1 ))
echo $n > count
if [[ $n -eq 2 ]]; then
  printf 'FAIL go-test: 1 failed in 1s\npkg/a\n  TestRound2\n    boom\n/tmp/log\n'
  exit 1
fi
`

func TestSoak_FailFastStopsAtFirstFailedRound(t *testing.T) {
	repo := soakRepo(t, failInRound2)

	out, code := runSoak(t, repo, "5", "--fail-fast")

	if code != 1 {
		t.Errorf("exit = %d, want 1:\n%s", code, out)
	}
	if !strings.Contains(out, "round 2: FAIL") || !strings.Contains(out, "TestRound2") {
		t.Errorf("output:\n%s", out)
	}
	if strings.Contains(out, "round 3") {
		t.Errorf("ran past the failed round:\n%s", out)
	}
}

func TestSoak_WithoutFailFastRunsEveryRound(t *testing.T) {
	repo := soakRepo(t, failInRound2)

	out, code := runSoak(t, repo, "4")

	if code != 1 {
		t.Errorf("exit = %d, want 1:\n%s", code, out)
	}
	for _, want := range []string{"round 3: pass", "round 4: pass", "1× TestRound2"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestSoak_RoundsRunThroughGXCheck(t *testing.T) {
	repo := soakRepo(t, "echo \"run=$GX_CHECK_RUN\" >> runs\n")

	runSoak(t, repo, "1")

	b, err := os.ReadFile(filepath.Join(repo, "runs"))
	if err != nil || strings.TrimSpace(string(b)) == "run=" {
		t.Errorf("GX_CHECK_RUN not set in the round: %v %q", err, b)
	}
}
