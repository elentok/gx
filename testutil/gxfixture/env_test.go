package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/testutil/claudeenv"
)

func TestShellEnv_ExportsTheCLIEnvAndTheFixtureVariablesQuoted(t *testing.T) {
	o := Output{
		WebURL: "https://127.0.0.1:1/a b", CookieName: "__Host-gx_login_1", Cookie: "c", StateDir: "/tmp/s", Socket: "/tmp/s.sock", HoldFile: "/tmp/h",
		Gx: "/bin/gx", DiskDir: "/tmp/disk", HealthDir: "/tmp/health", Env: []string{"HOME=/tmp/it's/home", "XDG_DATA_HOME=/tmp/d"},
	}
	want := "export HOME='/tmp/it'\\''s/home'\n" +
		"export XDG_DATA_HOME='/tmp/d'\n" +
		"export GXF_WEB_URL='https://127.0.0.1:1/a b'\n" +
		"export GXF_WEB_COOKIE='__Host-gx_login_1=c'\n" +
		"export GXF_GX='/bin/gx'\n" +
		"export GXF_DISK_DIR='/tmp/disk'\n" +
		"export GXF_HEALTH_DIR='/tmp/health'\n" +
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
	// The fixture would ask go for its caches, and go writes telemetry under HOME.
	env = append(env, claudeenv.GoCacheEnv()...)
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

func TestStart_AutoAdvanceDelayFlagIsAccepted(t *testing.T) {
	t.Parallel()
	r := startWith(t, "design", nil, "--auto-advance-delay", "50ms")
	r.stop(t)
}

func TestStart_MockDirServesTheDirectoryAndReportsMockURL(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "mock.html"), []byte("<p>mock</p>"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := startWith(t, "empty", nil, "--mock-dir", dir)
	if !strings.HasPrefix(r.MockURL, "http://127.0.0.1:") {
		t.Fatalf("mock_url = %q, want a loopback URL", r.MockURL)
	}
	resp, err := http.Get(r.MockURL + "mock.html")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "<p>mock</p>" {
		t.Errorf("GET mock.html = %q", body)
	}
	r.stop(t)
	if resp, err := http.Get(r.MockURL + "mock.html"); err == nil {
		resp.Body.Close()
		t.Errorf("mock server still answers after SIGTERM")
	}
}

func TestStart_NoMockDirLeavesMockURLOut(t *testing.T) {
	t.Parallel()
	r := startWith(t, "empty", nil)
	defer r.stop(t)
	if r.MockURL != "" {
		t.Errorf("mock_url = %q, want empty without --mock-dir", r.MockURL)
	}
}

func TestStart_BrowserSessionIsTheWorktreeDirectoryName(t *testing.T) {
	t.Parallel()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	r := start(t, "empty")
	if want := filepath.Base(cwd); r.BrowserSession != want {
		t.Errorf("browser_session = %q, want %q", r.BrowserSession, want)
	}
}

func TestShellEnv_ExportsBrowserSessionWhenSet(t *testing.T) {
	got := Output{BrowserSession: "wt-1"}.ShellEnv()
	if want := "export GXF_BROWSER_SESSION='wt-1'\n"; !strings.Contains(got, want) {
		t.Errorf("ShellEnv() = %s, want it to contain %s", got, want)
	}
}

func TestStart_LogFileIsTheServerLogAndExists(t *testing.T) {
	r := start(t, "empty")
	if r.LogFile == "" {
		t.Fatal("log_file is empty")
	}
	if _, err := os.Stat(r.LogFile); err != nil {
		t.Errorf("log_file %q: %v", r.LogFile, err)
	}
}

func TestShellEnv_ExportsLogFileWhenSet(t *testing.T) {
	got := Output{LogFile: "/tmp/r/server.out"}.ShellEnv()
	if want := "export GXF_LOG_FILE='/tmp/r/server.out'\n"; !strings.Contains(got, want) {
		t.Errorf("ShellEnv() = %s, want it to contain %s", got, want)
	}
}

func TestShellEnv_ExportsMockURLWhenSet(t *testing.T) {
	got := Output{MockURL: "http://127.0.0.1:2/"}.ShellEnv()
	if want := "export GXF_MOCK_URL='http://127.0.0.1:2/'\n"; !strings.Contains(got, want) {
		t.Errorf("ShellEnv() = %s, want it to contain %s", got, want)
	}
}

func TestIsolatedEnv_NamesTheRealPlaywrightCacheForTheServerAndTheCLI(t *testing.T) {
	t.Setenv("PLAYWRIGHT_BROWSERS_PATH", "/real/pw")
	env, cliEnv := isolatedEnv("/tmp/r", "/tmp/r/bin")
	const want = "PLAYWRIGHT_BROWSERS_PATH=/real/pw"
	if !slices.Contains(cliEnv, want) || !slices.Contains(env, want) {
		t.Errorf("env/cliEnv = %v / %v, want both to carry %s", env, cliEnv, want)
	}
}

func TestIsolatedEnv_DefaultsPlaywrightCacheToTheRealHomeNotTheTempOne(t *testing.T) {
	t.Setenv("PLAYWRIGHT_BROWSERS_PATH", "")
	_, cliEnv := isolatedEnv("/tmp/r", "/tmp/r/bin")
	var got string
	for _, kv := range cliEnv {
		if v, ok := strings.CutPrefix(kv, "PLAYWRIGHT_BROWSERS_PATH="); ok {
			got = v
		}
	}
	if got == "" || strings.HasPrefix(got, "/tmp/r/") || !strings.HasSuffix(got, "ms-playwright") {
		t.Errorf("PLAYWRIGHT_BROWSERS_PATH = %q, want the real home's ms-playwright cache", got)
	}
}

func TestStart_StallThresholdFlagIsAccepted(t *testing.T) {
	t.Parallel()
	r := startWith(t, "design", nil, "--stall-threshold", "2s")
	r.stop(t)
}

func TestStart_BadStallThresholdIsRefused(t *testing.T) {
	t.Parallel()
	out, err := exec.Command(fixtureBin, "--gx", gxBin, "--fakeclaude", fakeBin, "--stall-threshold", "soon", "design").CombinedOutput()
	if err == nil || !strings.Contains(string(out), "--stall-threshold") {
		t.Fatalf("err = %v, output = %q, want a refusal naming --stall-threshold", err, out)
	}
}

func TestIsolatedEnv_KeepsGoOutOfTheTempHomeForTheCLIToo(t *testing.T) {
	_, cliEnv := isolatedEnv("/tmp/r", "/tmp/r/bin")
	if !slices.Contains(cliEnv, "GOFLAGS=-modcacherw") {
		t.Errorf("cliEnv = %v, want GOFLAGS=-modcacherw", cliEnv)
	}
	for _, kv := range cliEnv {
		if strings.HasPrefix(kv, "GOMODCACHE=/tmp/r/") {
			t.Errorf("cliEnv has %s, want the real module cache", kv)
		}
	}
}
