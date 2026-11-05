package serverctl

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"html"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/config"
)

const launchdLabel = "com.elentok.gx.server"

// serviceManager is the seam for OS supervisors. Only launchd exists today;
// a systemd user unit would be a second implementation.
type serviceManager interface {
	Installed() bool
	Install() error
	Start() error
	Stop() error
	// Status describes the job as the supervisor sees it, for the report
	// saved after a start that never answered.
	Status() string
}

// launchdAgent manages the gx server's launchd agent. run executes launchctl
// and is replaced in tests.
type launchdAgent struct {
	agentsDir string
	stateDir  string
	exe       string
	uid       int
	// environ is written into the plist: launchd's own PATH is
	// /usr/bin:/bin:/usr/sbin:/sbin, which has neither herdr nor the agents.
	environ map[string]string
	run     func(args ...string) error
	// output runs launchctl and returns its combined output.
	output func(args ...string) (string, error)
}

// launchdEnvKeys are the installing shell's variables the server needs: PATH
// to find herdr, git and the agent CLIs, and herdr's socket. Pane- and
// tab-scoped HERDR_* variables are left out on purpose.
var launchdEnvKeys = []string{"PATH", "HERDR_SOCKET_PATH"}

// launchdEnviron picks launchdEnvKeys' non-empty values via getenv.
func launchdEnviron(getenv func(string) string) map[string]string {
	environ := map[string]string{}
	for _, key := range launchdEnvKeys {
		if v := getenv(key); v != "" {
			environ[key] = v
		}
	}
	return environ
}

func newLaunchdAgent(dirs config.Dirs, stateDir string) (*launchdAgent, error) {
	if runtime.GOOS != "darwin" {
		return nil, fmt.Errorf("gx server install is only supported on macOS")
	}
	home, err := dirs.Home()
	if err != nil {
		return nil, err
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return &launchdAgent{
		agentsDir: filepath.Join(home, "Library", "LaunchAgents"),
		stateDir:  stateDir,
		exe:       exe,
		uid:       os.Getuid(),
		environ:   launchdEnviron(dirs.Getenv),
		run:       runLaunchctl(dirs.Environ()),
		output:    outputLaunchctl(dirs.Environ()),
	}, nil
}

func runLaunchctl(env []string) func(args ...string) error {
	return func(args ...string) error { return launchctl(env, args...) }
}

func outputLaunchctl(env []string) func(args ...string) (string, error) {
	return func(args ...string) (string, error) {
		cmd := exec.Command("launchctl", args...)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
}

func launchctl(env []string, args ...string) error {
	cmd := exec.Command("launchctl", args...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl %s: %w: %s", strings.Join(args, " "), err, bytes.TrimSpace(out))
	}
	return nil
}

func (a *launchdAgent) plistPath() string {
	return filepath.Join(a.agentsDir, launchdLabel+".plist")
}

func (a *launchdAgent) domain() string { return fmt.Sprintf("gui/%d", a.uid) }

func (a *launchdAgent) target() string { return a.domain() + "/" + launchdLabel }

func (a *launchdAgent) Installed() bool {
	_, err := os.Stat(a.plistPath())
	return err == nil
}

var plistProgramRE = regexp.MustCompile(`(?s)<key>ProgramArguments</key>\s*<array>\s*<string>(.*?)</string>`)

// installedBinary is the binary the installed agent starts, read from its
// plist; false when the plist is missing or has none.
func (a *launchdAgent) installedBinary() (string, bool) {
	raw, err := os.ReadFile(a.plistPath())
	if err != nil {
		return "", false
	}
	m := plistProgramRE.FindSubmatch(raw)
	if m == nil {
		return "", false
	}
	return html.UnescapeString(string(m[1])), true
}

// restartBinary is the binary a restart would start: the launchd agent's when
// it is installed, else this executable. Empty when neither is known.
func restartBinary(dirs config.Dirs, stateDir string) string {
	if a, err := newLaunchdAgent(dirs, stateDir); err == nil && a.Installed() {
		if exe, ok := a.installedBinary(); ok {
			return exe
		}
	}
	exe, _ := os.Executable()
	return exe
}

// plist renders the agent definition. KeepAlive restarts the server only after
// a crash: a clean exit (`gx server stop`) stays stopped.
func (a *launchdAgent) plist() (string, error) {
	esc := func(s string) (string, error) {
		var b bytes.Buffer
		if err := xml.EscapeText(&b, []byte(s)); err != nil {
			return "", err
		}
		return b.String(), nil
	}
	exe, err := esc(a.exe)
	if err != nil {
		return "", err
	}
	stderr, err := esc(filepath.Join(a.stateDir, "server.stderr"))
	if err != nil {
		return "", err
	}
	var environ strings.Builder
	if len(a.environ) > 0 {
		environ.WriteString("\t<key>EnvironmentVariables</key>\n\t<dict>\n")
		for _, key := range slices.Sorted(maps.Keys(a.environ)) {
			k, err := esc(key)
			if err != nil {
				return "", err
			}
			v, err := esc(a.environ[key])
			if err != nil {
				return "", err
			}
			fmt.Fprintf(&environ, "\t\t<key>%s</key>\n\t\t<string>%s</string>\n", k, v)
		}
		environ.WriteString("\t</dict>\n")
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>server</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<dict>
		<key>SuccessfulExit</key>
		<false/>
	</dict>
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
%s</dict>
</plist>
`, launchdLabel, exe, stderr, stderr, environ.String()), nil
}

// Install writes the definition and (re)loads it; RunAtLoad starts the server.
func (a *launchdAgent) Install() error {
	content, err := a.plist()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(a.agentsDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(a.stateDir, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(a.plistPath(), []byte(content), 0o644); err != nil {
		return err
	}
	_ = a.run("bootout", a.target()) // not loaded yet is fine
	return a.bootstrap()
}

// bootstrapAttempts and bootstrapRetryDelay bound how long Install waits for a
// bootout to finish: it stops the old server asynchronously, and a bootstrap
// racing it fails with "Input/output error".
var (
	bootstrapAttempts   = 20
	bootstrapRetryDelay = 250 * time.Millisecond
)

func (a *launchdAgent) bootstrap() error {
	var err error
	for i := 0; i < bootstrapAttempts; i++ {
		if err = a.run("bootstrap", a.domain(), a.plistPath()); err == nil {
			return nil
		}
		time.Sleep(bootstrapRetryDelay)
	}
	return err
}

// startCheckAttempts and startCheckDelay bound how long Start watches the job
// after a kickstart for launchd refusing to spawn it.
var (
	startCheckAttempts = 20
	startCheckDelay    = 250 * time.Millisecond
)

// Start kickstarts the job. kickstart succeeds even when launchd then refuses
// to spawn the binary (job state "spawn failed", exit 78), and a refused job
// stays refused until it is loaded again. So Start watches the job briefly and
// falls back to Install, which re-reads the plist and reloads the job.
func (a *launchdAgent) Start() error {
	if err := a.run("kickstart", a.target()); err != nil {
		return err
	}
	if a.output == nil {
		return nil
	}
	for i := 0; i < startCheckAttempts; i++ {
		out, err := a.output("print", a.target())
		if err == nil {
			switch {
			case strings.Contains(out, "spawn failed"):
				saveStartIncident(a.stateDir, out)
				return a.Install()
			case strings.Contains(out, "state = running"):
				return nil
			}
		}
		time.Sleep(startCheckDelay)
	}
	return nil
}

// Status is `launchctl print` for the job: its state, last exit code and
// launch constraints, which is what shows a spawn that launchd refused.
func (a *launchdAgent) Status() string {
	if a.output == nil {
		return ""
	}
	out, err := a.output("print", a.target())
	if err != nil {
		return fmt.Sprintf("launchctl print %s: %v\n%s", a.target(), err, out)
	}
	return out
}

func (a *launchdAgent) Stop() error { return a.run("kill", "SIGTERM", a.target()) }
