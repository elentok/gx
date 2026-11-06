package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/elentok/gx/testutil/gxfixture/spec"
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
//	gx          optional path of another gx build that `server restart --continue` runs
//	            instead of the real one, so a restart can land on a new build.
//
// Scenarios: restart-agents (a running agent), restart-projects (two projects, one
// failing its migration), restart-launchd (the binary sits at a launchd-style path
// other than the server's own) and restart-missing (the binary path does not exist).

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
		next=$(cat "$ctl/gx" 2>/dev/null)
		exec "${next:-%[2]s}" "$@"
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

// setupRestart writes the control files and the fake binary, and points the
// server at it. projects are the scenario's project names.
func (f *fixture) setupRestart(r restartSpec, projects []string) error {
	ctl := f.restartDir()
	if err := os.MkdirAll(ctl, 0o755); err != nil {
		return err
	}
	check, err := json.Marshal(r.CheckOutput(projects))
	if err != nil {
		return err
	}
	files := map[string]string{"check.json": string(check) + "\n", "slow": r.Slow + "\n"}
	if r.StartsUp {
		files["start"] = "up\n"
	}
	if r.Old {
		files["old"] = "\n"
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(ctl, name), []byte(body), 0o644); err != nil {
			return err
		}
	}
	bin := filepath.Join(ctl, "bin", "gx")
	switch r.Path {
	case spec.RestartAtLaunchd:
		bin = filepath.Join(f.root, "launchd", "bin", "gx")
	case spec.RestartMissing:
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
