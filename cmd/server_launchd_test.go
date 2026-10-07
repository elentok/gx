package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func newTestAgent(t *testing.T) (*launchdAgent, *[][]string) {
	t.Helper()
	var calls [][]string
	return &launchdAgent{
		agentsDir: filepath.Join(t.TempDir(), "LaunchAgents"),
		stateDir:  filepath.Join(t.TempDir(), "state"),
		exe:       "/usr/local/bin/gx",
		uid:       501,
		run: func(args ...string) error {
			calls = append(calls, args)
			return nil
		},
	}, &calls
}

func TestLaunchdInstall_WritesCrashOnlyKeepAliveAndRunAtLoad(t *testing.T) {
	a, calls := newTestAgent(t)
	if a.Installed() {
		t.Fatal("installed before Install")
	}

	if err := a.Install(); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(a.plistPath())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"<key>RunAtLoad</key>\n\t<true/>",
		"<key>KeepAlive</key>\n\t<dict>\n\t\t<key>SuccessfulExit</key>\n\t\t<false/>",
		"<string>/usr/local/bin/gx</string>\n\t\t<string>server</string>",
		"<key>StandardErrorPath</key>\n\t<string>" + filepath.Join(a.stateDir, "server.stderr") + "</string>",
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("plist missing %q:\n%s", want, raw)
		}
	}
	if !a.Installed() {
		t.Error("not installed after Install")
	}
	wantLast := []string{"bootstrap", "gui/501", a.plistPath()}
	if got := (*calls)[len(*calls)-1]; !reflect.DeepEqual(got, wantLast) {
		t.Errorf("last launchctl call = %v, want %v", got, wantLast)
	}
}

// launchd starts agents with PATH=/usr/bin:/bin:/usr/sbin:/sbin, where herdr
// and the agent CLIs aren't, so the installing shell's PATH and herdr socket
// must be written into the plist.
func TestLaunchdInstall_WritesInstallingShellsEnvironment(t *testing.T) {
	a, _ := newTestAgent(t)
	a.environ = map[string]string{"PATH": "/opt/homebrew/bin:/usr/bin", "HERDR_SOCKET_PATH": "/h/herdr.sock"}

	if err := a.Install(); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(a.plistPath())
	if err != nil {
		t.Fatal(err)
	}
	want := "<key>EnvironmentVariables</key>\n\t<dict>\n" +
		"\t\t<key>HERDR_SOCKET_PATH</key>\n\t\t<string>/h/herdr.sock</string>\n" +
		"\t\t<key>PATH</key>\n\t\t<string>/opt/homebrew/bin:/usr/bin</string>\n" +
		"\t</dict>"
	if !strings.Contains(string(raw), want) {
		t.Errorf("plist missing %q:\n%s", want, raw)
	}
}

func TestLaunchdEnviron_KeepsOnlyPathAndHerdrSocket(t *testing.T) {
	got := launchdEnviron(func(key string) string {
		return map[string]string{"PATH": "/p", "HERDR_SOCKET_PATH": "/s", "HERDR_PANE_ID": "w:p1"}[key]
	})
	want := map[string]string{"PATH": "/p", "HERDR_SOCKET_PATH": "/s"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("launchdEnviron = %v, want %v", got, want)
	}
}

func TestLaunchdStartStop_RouteThroughLaunchctl(t *testing.T) {
	a, calls := newTestAgent(t)

	if err := a.Start(); err != nil {
		t.Fatal(err)
	}
	if err := a.Stop(); err != nil {
		t.Fatal(err)
	}

	want := [][]string{
		{"kickstart", "gui/501/" + launchdLabel},
		{"kill", "SIGTERM", "gui/501/" + launchdLabel},
	}
	if !reflect.DeepEqual(*calls, want) {
		t.Errorf("launchctl calls = %v, want %v", *calls, want)
	}
}

// bootout stops the old server asynchronously, so the bootstrap right after it
// can fail with "Input/output error" until the teardown finishes.
func TestLaunchdInstall_RetriesBootstrapWhileBootoutFinishes(t *testing.T) {
	a, _ := newTestAgent(t)
	oldAttempts, oldDelay := bootstrapAttempts, bootstrapRetryDelay
	bootstrapAttempts, bootstrapRetryDelay = 5, 0
	t.Cleanup(func() { bootstrapAttempts, bootstrapRetryDelay = oldAttempts, oldDelay })
	failures := 2
	a.run = func(args ...string) error {
		if args[0] == "bootstrap" && failures > 0 {
			failures--
			return errors.New("Bootstrap failed: 5: Input/output error")
		}
		return nil
	}

	if err := a.Install(); err != nil {
		t.Fatalf("Install = %v, want success after the bootstrap retries", err)
	}
	if failures != 0 {
		t.Errorf("bootstrap was not retried: %d failures left", failures)
	}
}
