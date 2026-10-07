package cmd

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

const launchdLabel = "com.elentok.gx.server"

// serviceManager is the seam for OS supervisors. Only launchd exists today;
// a systemd user unit would be a second implementation.
type serviceManager interface {
	Installed() bool
	Install() error
	Start() error
	Stop() error
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

func newLaunchdAgent(stateDir string) (*launchdAgent, error) {
	if runtime.GOOS != "darwin" {
		return nil, fmt.Errorf("gx server install is only supported on macOS")
	}
	home, err := os.UserHomeDir()
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
		environ:   launchdEnviron(os.Getenv),
		run:       runLaunchctl,
	}, nil
}

func runLaunchctl(args ...string) error {
	out, err := exec.Command("launchctl", args...).CombinedOutput()
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
	return a.run("bootstrap", a.domain(), a.plistPath())
}

func (a *launchdAgent) Start() error { return a.run("kickstart", a.target()) }

func (a *launchdAgent) Stop() error { return a.run("kill", "SIGTERM", a.target()) }
