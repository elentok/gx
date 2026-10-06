package cmd

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	run       func(args ...string) error
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
</dict>
</plist>
`, launchdLabel, exe, stderr, stderr), nil
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
