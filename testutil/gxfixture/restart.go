package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/elentok/gx/server"
)

// The restart scenarios point the server's restart binary (GX_RESTART_BINARY) at
// a fake gx script, so a flow steers the restart check and the restart itself
// through control files in <root>/restart, which the output names as RestartDir:
//
//	check.json  what `restart-check` prints (server.RestartCheckOutput); rewrite it
//	            to change the version, the config result or a project's migration.
//	old         present: the binary has no restart-check command (exits "unknown command").
//	slow        seconds `restart-check` sleeps before answering.
//	start       "up": `server restart --continue` runs the real gx, which brings the
//	            server back; anything else: it exits non-zero and the server stays down.
//
// Scenarios: restart-agents (a running agent), restart-projects (two projects, one
// failing its migration), restart-launchd (the binary sits at a launchd-style path
// other than the server's own) and restart-missing (the binary path does not exist).

// restartPath is where the fake binary sits.
type restartPath int

const (
	// restartAtDefault is <root>/restart/bin/gx.
	restartAtDefault restartPath = iota
	// restartAtLaunchd is <root>/launchd/bin/gx, as an installed launchd agent would name it.
	restartAtLaunchd
	// restartMissing names a path with no file behind it.
	restartMissing
)

// restartSpec is a scenario's fake restart binary.
type restartSpec struct {
	path restartPath
	// version is what the fake reports as the new version.
	version string
	// configError makes the config check fail with this message.
	configError string
	// failing maps a project to its dry-run error; the other projects pass.
	failing map[string]string
	// old makes the binary reject restart-check, as a binary older than the command does.
	old bool
	// slow is how long restart-check takes, in seconds ("0.5").
	slow string
	// startsUp lets the restart bring the server back.
	startsUp bool
}

const defaultNewVersion = "fixture-next"

// fakeGxBody answers like gx for the two commands a restart runs. %[1]s is the
// control dir and %[2]s the real gx.
const fakeGxBody = `#!/bin/sh
ctl=%[1]q
case "$1" in
restart-check)
	if [ -e "$ctl/old" ]; then
		echo 'Error: unknown command "restart-check" for "gx"' >&2
		exit 1
	fi
	slow=$(cat "$ctl/slow" 2>/dev/null)
	[ -n "$slow" ] && sleep "$slow"
	cat "$ctl/check.json"
	;;
server)
	if [ "$(cat "$ctl/start" 2>/dev/null)" = up ]; then
		exec %[2]q "$@"
	fi
	echo "fake gx: start refused" >&2
	exit 1
	;;
*)
	echo "fake gx: unexpected $*" >&2
	exit 2
	;;
esac
`

func (f *fixture) restartDir() string { return filepath.Join(f.root, "restart") }

// checkOutput is what the fake prints for the scenario's projects.
func (r restartSpec) checkOutput(projects []string) server.RestartCheckOutput {
	version := r.version
	if version == "" {
		version = defaultNewVersion
	}
	out := server.RestartCheckOutput{Version: version, Config: server.RestartCheckConfig{OK: r.configError == "", Error: r.configError}}
	for _, name := range projects {
		p := server.RestartCheckProject{Name: name, OK: true, Files: []string{}}
		if msg, bad := r.failing[name]; bad {
			p.OK, p.Error = false, msg
		}
		out.Projects = append(out.Projects, p)
	}
	return out
}

// setupRestart writes the control files and the fake binary, and points the
// server at it. projects are the scenario's project names.
func (f *fixture) setupRestart(r restartSpec, projects []string) error {
	ctl := f.restartDir()
	if err := os.MkdirAll(ctl, 0o755); err != nil {
		return err
	}
	check, err := json.Marshal(r.checkOutput(projects))
	if err != nil {
		return err
	}
	files := map[string]string{"check.json": string(check) + "\n", "slow": r.slow + "\n"}
	if r.startsUp {
		files["start"] = "up\n"
	}
	if r.old {
		files["old"] = "\n"
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(ctl, name), []byte(body), 0o644); err != nil {
			return err
		}
	}
	bin := filepath.Join(ctl, "bin", "gx")
	switch r.path {
	case restartAtLaunchd:
		bin = filepath.Join(f.root, "launchd", "bin", "gx")
	case restartMissing:
		f.env = append(f.env, "GX_RESTART_BINARY="+filepath.Join(ctl, "missing", "gx"))
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(bin, fmt.Appendf(nil, fakeGxBody, ctl, f.gx), 0o755); err != nil {
		return err
	}
	f.env = append(f.env, "GX_RESTART_BINARY="+bin)
	return nil
}
