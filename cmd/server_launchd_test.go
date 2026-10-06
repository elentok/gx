package cmd

import (
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
