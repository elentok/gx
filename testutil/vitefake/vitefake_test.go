package vitefake_test

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/vitefake"
)

var pnpmBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "vitefake")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	pnpmBin = filepath.Join(dir, "pnpm")
	out, err := exec.Command("go", "build", "-o", pnpmBin, "github.com/elentok/gx/testutil/vitefake/cmd/pnpm").CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "build pnpm: %v\n%s", err, out)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func control(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "control.json")
	setControl(t, path, content)
	return path
}

// setControl rewrites atomically so the polling child never reads half a file.
func setControl(t *testing.T, path, content string) {
	t.Helper()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
}

func start(t *testing.T, ctl string, args ...string) (*exec.Cmd, *strings.Builder, chan error) {
	t.Helper()
	cmd := exec.Command(pnpmBin, args...)
	cmd.Env = append(os.Environ(), vitefake.ControlEnv+"="+ctl)
	out := &strings.Builder{}
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		select {
		case <-done:
		default:
		}
	})
	return cmd, out, done
}

func exitCode(t *testing.T, done chan error) int {
	t.Helper()
	select {
	case err := <-done:
		if err == nil {
			return 0
		}
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		t.Fatal(err)
	case <-time.After(10 * time.Second):
		t.Fatal("process did not exit")
	}
	return -1
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func get(port int, path string) (int, string) {
	resp, err := (&http.Client{Timeout: time.Second}).Get(fmt.Sprintf("http://127.0.0.1:%d%s", port, path))
	if err != nil {
		return 0, ""
	}
	defer resp.Body.Close()
	var b strings.Builder
	buf := make([]byte, 512)
	for {
		n, err := resp.Body.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return resp.StatusCode, b.String()
}

func viteArgs(port int) []string {
	return []string{"exec", "vite", "--host", "127.0.0.1", "--port", fmt.Sprint(port), "--strictPort"}
}

func TestInstall_ExitsWithScriptedCodeAfterPrintingLines(t *testing.T) {
	for _, tc := range []struct {
		name, ctl string
		code      int
	}{
		{"default succeeds", ``, 0},
		{"ok", `{"install":"ok"}`, 0},
		{"fail", `{"install":"fail"}`, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, out, done := start(t, control(t, tc.ctl), "install", "--frozen-lockfile")
			if got := exitCode(t, done); got != tc.code {
				t.Fatalf("exit = %d, want %d; output:\n%s", got, tc.code, out)
			}
			if !strings.Contains(out.String(), "Progress:") {
				t.Fatalf("no install lines printed:\n%s", out)
			}
		})
	}
}

func TestInstall_HoldBlocksUntilControlFileChanges(t *testing.T) {
	ctl := control(t, `{"install":"hold"}`)
	_, _, done := start(t, ctl, "install", "--frozen-lockfile")
	select {
	case err := <-done:
		t.Fatalf("exited while held: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	setControl(t, ctl, `{"install":"ok"}`)
	if got := exitCode(t, done); got != 0 {
		t.Fatalf("exit = %d, want 0", got)
	}
}

func TestVite_ServesProbeStubOnceReady(t *testing.T) {
	port := freePort(t)
	start(t, control(t, `{"vite":"ready"}`), viteArgs(port)...)
	var code int
	var body string
	if !testutil.Poll(10*time.Second, 20*time.Millisecond, func() bool {
		code, body = get(port, "/")
		return code == http.StatusOK
	}) {
		t.Fatal("vite stub never answered")
	}
	if !strings.Contains(body, "/src/main.tsx") {
		t.Fatalf("html does not reference the stub module: %q", body)
	}
	if code, _ := get(port, "/src/main.tsx"); code != http.StatusOK {
		t.Fatalf("stub module status = %d", code)
	}
}

func TestVite_HeldNeverAnswersThenReadyWhenReleased(t *testing.T) {
	port := freePort(t)
	ctl := control(t, `{"vite":"hold"}`)
	start(t, ctl, viteArgs(port)...)
	deadline := time.After(300 * time.Millisecond)
	for held := true; held; {
		select {
		case <-deadline:
			held = false
		default:
			if code, _ := get(port, "/"); code != 0 {
				t.Fatalf("answered while held: %d", code)
			}
		}
	}
	setControl(t, ctl, `{"vite":"ready"}`)
	if !testutil.Poll(10*time.Second, 20*time.Millisecond, func() bool {
		code, _ := get(port, "/")
		return code == http.StatusOK
	}) {
		t.Fatal("never became ready after release")
	}
}

func TestVite_CrashExitsNonZero(t *testing.T) {
	port := freePort(t)
	ctl := control(t, `{"vite":"ready"}`)
	_, _, done := start(t, ctl, viteArgs(port)...)
	if !testutil.Poll(10*time.Second, 20*time.Millisecond, func() bool {
		code, _ := get(port, "/")
		return code == http.StatusOK
	}) {
		t.Fatal("never ready")
	}
	setControl(t, ctl, `{"vite":"crash"}`)
	if got := exitCode(t, done); got == 0 {
		t.Fatal("crash exited 0")
	}
}

func TestVite_CrashBeforeReadyExitsNonZero(t *testing.T) {
	_, _, done := start(t, control(t, `{"vite":"crash"}`), viteArgs(freePort(t))...)
	if got := exitCode(t, done); got == 0 {
		t.Fatal("crash exited 0")
	}
}

func TestVite_TerminatesOnSigterm(t *testing.T) {
	port := freePort(t)
	cmd, _, done := start(t, control(t, `{"vite":"ready"}`), viteArgs(port)...)
	if !testutil.Poll(10*time.Second, 20*time.Millisecond, func() bool {
		code, _ := get(port, "/")
		return code == http.StatusOK
	}) {
		t.Fatal("never ready")
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	exitCode(t, done)
}

func TestRun_UnsupportedArgsExit2(t *testing.T) {
	_, _, done := start(t, "", "frobnicate")
	if got := exitCode(t, done); got != 2 {
		t.Fatalf("exit = %d, want 2", got)
	}
}
