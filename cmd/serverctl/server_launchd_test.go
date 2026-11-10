package serverctl

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server/health"
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

func uninstallAgent(t *testing.T, loaded bool) (*launchdAgent, *[][]string) {
	t.Helper()
	a, calls := newTestAgent(t)
	a.output = func(args ...string) (string, error) {
		if loaded {
			return "pid = 4242\n\tstate = running", nil
		}
		return "Could not find service", errors.New("exit status 113")
	}
	return a, calls
}

func installPlist(t *testing.T, a *launchdAgent) {
	t.Helper()
	if err := os.MkdirAll(a.agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(a.plistPath(), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLaunchdUninstall_BootsOutRemovesPlistAndReports(t *testing.T) {
	a, calls := uninstallAgent(t, true)
	installPlist(t, a)
	var out strings.Builder

	if err := a.Uninstall(&out); err != nil {
		t.Fatal(err)
	}

	if want := [][]string{{"bootout", "gui/501/com.elentok.gx.server"}}; !reflect.DeepEqual(*calls, want) {
		t.Errorf("launchctl calls = %v, want %v", *calls, want)
	}
	if a.Installed() {
		t.Error("plist still present")
	}
	for _, want := range []string{a.plistPath(), "stopped the running server", "agent holders keep running"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestLaunchdUninstall_NothingInstalledPrintsNotInstalled(t *testing.T) {
	a, calls := uninstallAgent(t, false)
	var out strings.Builder

	if err := a.Uninstall(&out); err != nil {
		t.Fatal(err)
	}

	if strings.TrimSpace(out.String()) != "not installed" {
		t.Errorf("output = %q, want \"not installed\"", out.String())
	}
	if len(*calls) != 0 {
		t.Errorf("launchctl calls = %v, want none", *calls)
	}
}

func TestLaunchdUninstall_LoadedJobWithoutPlistIsStillBootedOut(t *testing.T) {
	a, calls := uninstallAgent(t, true)
	var out strings.Builder

	if err := a.Uninstall(&out); err != nil {
		t.Fatal(err)
	}

	if len(*calls) != 1 || (*calls)[0][0] != "bootout" {
		t.Errorf("launchctl calls = %v, want one bootout", *calls)
	}
	if strings.Contains(out.String(), "not installed") {
		t.Errorf("output = %q", out.String())
	}
}

func TestLaunchdUninstall_LoadedJobWithoutPidClaimsNoStop(t *testing.T) {
	a, _ := uninstallAgent(t, true)
	a.output = func(args ...string) (string, error) { return "state = waiting\n\tlast exit code = 0", nil }
	installPlist(t, a)
	var out strings.Builder

	if err := a.Uninstall(&out); err != nil {
		t.Fatal(err)
	}

	if strings.Contains(out.String(), "stopped the running server") || !strings.Contains(out.String(), "no server was running") {
		t.Errorf("output = %q", out.String())
	}
}

func TestLaunchdUninstall_PlistAlreadyGoneClaimsNoRemoval(t *testing.T) {
	a, _ := uninstallAgent(t, true)
	var out strings.Builder

	if err := a.Uninstall(&out); err != nil {
		t.Fatal(err)
	}

	if strings.Contains(out.String(), "removed") || !strings.Contains(out.String(), "stopped the running server") {
		t.Errorf("output = %q", out.String())
	}
}

func TestLaunchdUninstall_PlistWithoutLoadedJobSaysNoServerRan(t *testing.T) {
	a, calls := uninstallAgent(t, false)
	installPlist(t, a)
	var out strings.Builder

	if err := a.Uninstall(&out); err != nil {
		t.Fatal(err)
	}

	if len(*calls) != 0 || a.Installed() {
		t.Errorf("calls = %v, installed = %v", *calls, a.Installed())
	}
	if !strings.Contains(out.String(), "no server was running") {
		t.Errorf("output = %q", out.String())
	}
}

func TestLaunchdUninstall_InstallAfterwardsWorksWithoutDisableOrEnable(t *testing.T) {
	a, calls := uninstallAgent(t, true)
	installPlist(t, a)
	if err := a.Uninstall(&strings.Builder{}); err != nil {
		t.Fatal(err)
	}

	if err := a.Install(); err != nil {
		t.Fatal(err)
	}

	for _, c := range *calls {
		if c[0] == "disable" || c[0] == "enable" {
			t.Errorf("unexpected launchctl %v", c)
		}
	}
	if !a.Installed() {
		t.Error("not installed after reinstall")
	}
}

func TestNewLaunchdAgent_RefusesOffMacWithSharedMessage(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("refusal only happens off macOS")
	}
	if _, err := newLaunchdAgent(config.Dirs{}, ""); !errors.Is(err, errLaunchdOnly) {
		t.Errorf("err = %v, want errLaunchdOnly", err)
	}
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

// kickstart succeeds even when launchd refuses to spawn the binary, and the job
// stays refused until it is loaded again.
func TestLaunchdStart_ReloadsTheJobWhenLaunchdRefusesToSpawn(t *testing.T) {
	a, calls := newTestAgent(t)
	a.output = func(args ...string) (string, error) {
		return "\tstate = spawn scheduled\n\tjob state = spawn failed\n", nil
	}

	if err := a.Start(); err != nil {
		t.Fatal(err)
	}

	var verbs []string
	for _, c := range *calls {
		verbs = append(verbs, c[0])
	}
	if want := []string{"kickstart", "bootout", "bootstrap"}; !reflect.DeepEqual(verbs, want) {
		t.Errorf("launchctl verbs = %v, want %v", verbs, want)
	}
}

func TestLaunchdStart_LeavesARunningJobAlone(t *testing.T) {
	a, calls := newTestAgent(t)
	a.output = func(args ...string) (string, error) { return "\tstate = running\n", nil }

	if err := a.Start(); err != nil {
		t.Fatal(err)
	}

	if len(*calls) != 1 {
		t.Errorf("launchctl calls = %v, want only the kickstart", *calls)
	}
}

func TestLaunchdStart_LeavesAnIncidentNoteWhenItReloadsTheJob(t *testing.T) {
	a, _ := newTestAgent(t)
	a.output = func(args ...string) (string, error) { return "job state = spawn failed\n", nil }

	if err := a.Start(); err != nil {
		t.Fatal(err)
	}

	note, err := os.ReadFile(health.StartIncidentPath(a.stateDir))
	if err != nil || !strings.Contains(string(note), "spawn failed") {
		t.Fatalf("incident note = %q, %v; want the launchd state", note, err)
	}
}
