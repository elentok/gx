package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// stubGX is what the stubbed `go build -o` installs as the gx binary. The "go"
// step (a go-test run that is neither -race nor ./server) waits for the e2e
// step's marker, so it only finishes if the two overlap.
const stubGX = `#!/usr/bin/env bash
case " $* " in
*" -race "*|*" ./server "*) exit 0 ;;
esac
if [[ -n ${WAIT_FOR_E2E:-} ]]; then
	for _ in $(seq 100); do
		[[ -e "$STUB_DIR/e2e.started" ]] && exit 0
		sleep 0.1
	done
	echo "e2e never started while go was running" >&2
	exit 1
fi
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
web-test) echo "$*" >"$STUB_DIR/web-test.args" ;;
web-e2e)
	touch "$STUB_DIR/e2e.started"
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
