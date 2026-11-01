package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// The migrate scenarios run the real server's start-up migration, steered
// through control files in <root>/migrate, which the output names as MigrateDir:
//
//	hold         present: the migration is held open; remove it to let it run.
//	             Only the migrate-held scenario creates it.
//	inject.json  extra migration steps, read when the server starts: a JSON list of
//	             {"name", "fail"} (a failing step) or {"name", "notes": [...]} (a step
//	             that returns skip notes). Write it, then restart the server.
//	pid          the pid of the bare `gx server` process.
//	server       a script that controls that process: `server pid`, `server kill`
//	             (SIGKILL, waits for exit) and `server start` (a new bare `gx server`
//	             with the fixture's environment, its pid written to pid).
//
// Scenarios: migrate-pending (a project with no applied list and an old-shape ticket),
// migrate-held (the same with the hold on, and a queued ready ticket), migrate-broken
// (project broken fails its migration, project demo is fine; both queued).

func (f *fixture) migrateDir() string { return filepath.Join(f.root, "migrate") }

// shellName matches the environment names a shell can export; others (bash
// functions, say) are left out of the script's environment.
var shellName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// commitStore gives the ticket store the history a real one has: the history
// backfill steps read the store's git log, and fail on a repo with no commit.
func (f *fixture) commitStore() error {
	for _, args := range [][]string{{"init", "-b", "main"}, {"add", "."}, {"commit", "-m", "fixture store"}} {
		if err := f.git(f.store, args...); err != nil {
			return err
		}
	}
	return nil
}

// setupMigrate points the server's migration hooks at the control files.
func (f *fixture) setupMigrate(hold bool) error {
	dir := f.migrateDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if hold {
		if err := os.WriteFile(filepath.Join(dir, "hold"), nil, 0o644); err != nil {
			return err
		}
		f.env = append(f.env, "GX_MIGRATE_HOLD_FILE="+filepath.Join(dir, "hold"))
	}
	f.env = append(f.env, "GX_MIGRATE_INJECT_FILE="+filepath.Join(dir, "inject.json"))
	return nil
}

const serverScript = `#!/bin/sh
dir=$(dirname "$0")
case "$1" in
pid)
	cat "$dir/pid"
	;;
kill)
	pid=$(cat "$dir/pid")
	kill -KILL "$pid" 2>/dev/null
	while kill -0 "$pid" 2>/dev/null; do sleep 0.05; done
	;;
start)
	. "$dir/vars"
	nohup %q server >>%q 2>&1 </dev/null &
	echo $! >"$dir/pid"
	;;
*)
	echo "usage: server pid|kill|start" >&2
	exit 2
	;;
esac
`

// writeServerControl writes the pid, the environment file and the control
// script, once the environment is final.
func (f *fixture) writeServerControl(pid int, logPath string) error {
	dir := f.migrateDir()
	var env strings.Builder
	for _, kv := range f.env {
		k, v, _ := strings.Cut(kv, "=")
		if !shellName.MatchString(k) {
			continue
		}
		fmt.Fprintf(&env, "export %s='%s'\n", k, strings.ReplaceAll(v, "'", `'\''`))
	}
	files := []struct {
		name, body string
		mode       os.FileMode
	}{
		{"pid", strconv.Itoa(pid) + "\n", 0o644},
		{"vars", env.String(), 0o600},
		{"server", fmt.Sprintf(serverScript, f.gx, logPath), 0o755},
	}
	for _, file := range files {
		if err := os.WriteFile(filepath.Join(dir, file.name), []byte(file.body), file.mode); err != nil {
			return err
		}
	}
	return nil
}
