package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/syshealth"
)

const (
	// calmHealth is well below every threshold, so a scenario that does not steer
	// the meters never shows a warning.
	calmHealth = "cpu=20 mem=50 pressure=none"
	// calmProcesses is two idle rows and no agent, so no flow shows the
	// developer's real processes.
	calmProcesses = "proc 90001 1 1700000000 0 40M launchd\nproc 90002 1 1700000000 0 25M syslogd\n"
	// healthInterval is short so the 12-reading average settles in about a second.
	healthInterval = "100ms"
)

// healthDir sits outside the project repo, so the trunk stays clean.
func (f *fixture) healthDir() string { return filepath.Join(f.root, "health") }

// setupHealth gives every scenario fake CPU, memory and processes: the server
// reads them from <health dir>/system and <health dir>/processes, which start
// at calmHealth and calmProcesses.
func (f *fixture) setupHealth() error {
	if err := os.MkdirAll(f.healthDir(), 0o755); err != nil {
		return err
	}
	system := filepath.Join(f.healthDir(), "system")
	processes := filepath.Join(f.healthDir(), "processes")
	if err := os.WriteFile(system, []byte(calmHealth+"\n"), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(processes, []byte(calmProcesses), 0o644); err != nil {
		return err
	}
	f.env = append(f.env, "GX_HEALTH_FILE="+system, "GX_HEALTH_PROCESSES="+processes, "GX_HEALTH_INTERVAL="+healthInterval)
	return nil
}

// agentProcesses is the table that ties the fake agent to its pids: claude under
// its holder, a busy go test under claude, a plain node process and a hidden
// row, so "other users" makes the top 3.
func agentProcesses(m nativerunner.Meta) (string, error) {
	claudeStart, err := syshealth.ParseLstart(m.PIDStart)
	if err != nil {
		return "", err
	}
	holderStart, err := syshealth.ParseLstart(m.HolderPIDStart)
	if err != nil {
		return "", err
	}
	return calmProcesses + strings.Join([]string{
		"busy 4500",
		fmt.Sprintf("proc %d 1 %d 50 20M gx-holder", m.HolderPID, holderStart),
		fmt.Sprintf("proc %d %d %d 200 300M node claude", m.PID, m.HolderPID, claudeStart),
		fmt.Sprintf("proc 90101 %d %d 1500 150M go go test ./...", m.PID, claudeStart),
		fmt.Sprintf("proc 90102 1 %d 800 100M node node build.js", claudeStart),
		"hidden 90103",
	}, "\n") + "\n", nil
}

// writeHealthFile replaces a control file through a rename, so the server never
// reads half of it.
func writeHealthFile(path, body string) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(body), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// tieAgentProcesses waits for the fake agent's meta.json, then writes its real
// pids into the process table.
func (f *fixture) tieAgentProcesses(ctx context.Context) error {
	root := nativerunner.AgentsRoot(f.stateD, project)
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if m, ok := readAnyMeta(root); ok {
			body, err := agentProcesses(m)
			if err != nil {
				return err
			}
			return writeHealthFile(filepath.Join(f.healthDir(), "processes"), body)
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("no agent meta.json under %s", root)
}

// readAnyMeta is the first agent meta under root that names a claude pid, its
// holder and their start times.
func readAnyMeta(root string) (nativerunner.Meta, bool) {
	paths, _ := filepath.Glob(filepath.Join(root, "*", nativerunner.MetaFile))
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var m nativerunner.Meta
		if json.Unmarshal(b, &m) == nil && m.PID > 0 && m.PIDStart != "" && m.HolderPID > 0 && m.HolderPIDStart != "" {
			return m, true
		}
	}
	return nativerunner.Meta{}, false
}
