package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// stubGX is what the stubbed `go build -o` installs as the gx binary. The "go"
// step (a go-test run that is neither -race nor ./server) waits for the e2e
// step's marker, so it only finishes if the two overlap. Every non-burst step
// leaves "<name>.ended" after a short delay; the burst step (the run with -run
// naming the burst test) records which of those exist when it starts.
const stubGX = `#!/usr/bin/env bash
case " $* " in
*" -run ^TestProcessBurst"*)
	ls "$STUB_DIR" | grep '\.ended$' >"$STUB_DIR/burst.saw" || true
	exit "${BURST_EXIT:-0}"
	;;
*" -race "*) step=race ;;
*" ./server/... "*) step=server ;;
*)
	step=go
	echo "$*" >"$STUB_DIR/go.args"
	;;
esac
if [[ $step == go && -n ${WAIT_FOR_E2E:-} ]]; then
	for _ in $(seq 100); do
		[[ -e "$STUB_DIR/e2e.started" ]] && break
		sleep 0.1
	done
	if [[ ! -e "$STUB_DIR/e2e.started" ]]; then
		echo "e2e never started while go was running" >&2
		exit 1
	fi
fi
sleep 0.3
touch "$STUB_DIR/$step.ended"
`

const stubGo = `#!/usr/bin/env bash
case $1 in
build)
	cp "$STUB_DIR/gx" "$3"
	;;
list)
	echo github.com/elentok/gx/other
	;;
esac
`

const stubMake = `#!/usr/bin/env bash
case $1 in
web-test)
	echo "$*" >"$STUB_DIR/web-test.args"
	sleep 0.3
	touch "$STUB_DIR/web.ended"
	;;
web-e2e)
	touch "$STUB_DIR/e2e.started"
	sleep 0.3
	touch "$STUB_DIR/e2e.ended"
	exit "${E2E_EXIT:-0}"
	;;
esac
`

// runTestSh runs scripts/test.sh with stubbed gx, go and make on PATH. It
// returns the combined output, the exit code and the stub dir (where the stubs
// leave their markers).
func runTestSh(t *testing.T, env ...string) (out string, code int, stubs string) {
	t.Helper()
	script, err := filepath.Abs("test.sh")
	if err != nil {
		t.Fatal(err)
	}
	stubs = t.TempDir()
	for name, body := range map[string]string{"gx": stubGX, "go": stubGo, "make": stubMake} {
		if err := os.WriteFile(filepath.Join(stubs, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("bash", script)
	cmd.Env = append(os.Environ(), append([]string{
		"PATH=" + stubs + string(os.PathListSeparator) + os.Getenv("PATH"),
		"STUB_DIR=" + stubs,
		// `gx check --full` sets GX_TEST_E2E=1 in this process's env; a test turns it on itself.
		"GX_TEST_E2E=",
	}, env...)...)
	b, err := cmd.CombinedOutput()
	if exitErr, ok := err.(*exec.ExitError); ok {
		return string(b), exitErr.ExitCode(), stubs
	}
	if err != nil {
		t.Fatalf("test.sh: %v\n%s", err, b)
	}
	return string(b), 0, stubs
}

func TestTestSh_E2EOverlapsGoStep(t *testing.T) {
	out, code, _ := runTestSh(t, "GX_TEST_E2E=1", "WAIT_FOR_E2E=1")
	if code != 0 {
		t.Fatalf("exit %d, want 0 (the go step only passes if e2e started during it):\n%s", code, out)
	}
	if !strings.Contains(out, "PASS e2e") {
		t.Errorf("no e2e status line:\n%s", out)
	}
}

func TestTestSh_VitestWorkers(t *testing.T) {
	out, code, stubs := runTestSh(t)
	if code != 0 {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	args, err := os.ReadFile(filepath.Join(stubs, "web-test.args"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "WEB_TEST_WORKERS=3") {
		t.Errorf("make web-test args = %q, want WEB_TEST_WORKERS=3", args)
	}
}

func TestTestSh_FailingE2EFailsRun(t *testing.T) {
	out, code, _ := runTestSh(t, "GX_TEST_E2E=1", "E2E_EXIT=1")
	if code == 0 {
		t.Fatalf("want non-zero exit:\n%s", out)
	}
	if !strings.Contains(out, "FAIL e2e") {
		t.Errorf("no failing e2e line:\n%s", out)
	}
}

func TestTestSh_E2EOffByDefault(t *testing.T) {
	out, code, _ := runTestSh(t, "E2E_EXIT=1")
	if code != 0 || strings.Contains(out, "e2e") {
		t.Errorf("e2e ran without GX_TEST_E2E (exit %d):\n%s", code, out)
	}
}

func TestTestSh_BurstStartsAfterEveryOtherStepEnded(t *testing.T) {
	out, code, stubs := runTestSh(t, "GX_TEST_E2E=1")
	if code != 0 {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	if !strings.Contains(out, "PASS burst") {
		t.Errorf("no burst status line:\n%s", out)
	}
	saw, err := os.ReadFile(filepath.Join(stubs, "burst.saw"))
	if err != nil {
		t.Fatalf("the burst step never ran: %v\n%s", err, out)
	}
	for _, step := range []string{"race", "go", "server", "web", "e2e"} {
		if !strings.Contains(string(saw), step+".ended") {
			t.Errorf("burst started before %s ended; ended then: %q", step, saw)
		}
	}
}

func TestTestSh_FailingBurstFailsRun(t *testing.T) {
	out, code, _ := runTestSh(t, "BURST_EXIT=1")
	if code == 0 {
		t.Fatalf("want non-zero exit:\n%s", out)
	}
	if !strings.Contains(out, "FAIL burst") {
		t.Errorf("no failing burst line:\n%s", out)
	}
}

func TestTestSh_GoStepSkipsBurstTest(t *testing.T) {
	out, code, stubs := runTestSh(t)
	if code != 0 {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	args, err := os.ReadFile(filepath.Join(stubs, "go.args"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "-skip ^"+burstTestName(t)+"$") {
		t.Errorf("go step args = %q, want -skip for the burst test", args)
	}
}

// burstTestName reads the burst test's name from test.sh and checks it is a
// real test function in storm_test.go, so a rename fails here instead of
// putting the burst test back in the go step or dropping it.
func burstTestName(t *testing.T) string {
	t.Helper()
	script, err := os.ReadFile("test.sh")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^burst_test=(\w+)$`).FindSubmatch(script)
	if m == nil {
		t.Fatal("test.sh has no burst_test=<name> line")
	}
	name := string(m[1])
	storm, err := os.ReadFile(filepath.Join("..", "testutil", "gxfixture", "storm_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(storm), "func "+name+"(t *testing.T)") {
		t.Fatalf("test.sh burst_test=%s is not a test in testutil/gxfixture/storm_test.go", name)
	}
	return name
}

func TestTestSh_BurstTestNameIsARealTest(t *testing.T) {
	burstTestName(t)
}
