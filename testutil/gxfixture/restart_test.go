package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
)

func (r *running) setRestart(t *testing.T, name, value string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(r.RestartDir, name), []byte(value+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// restartCheck is the server's restart-check route answer, through the CLI.
func (r *running) restartCheck(t *testing.T) server.RestartCheckReport {
	t.Helper()
	var rep server.RestartCheckReport
	if err := json.Unmarshal([]byte(r.gx(t, "server", "restart-check", "--json")), &rep); err != nil {
		t.Fatal(err)
	}
	return rep
}

func TestRestartScenarios_TheFakeBinaryAnswersTheCheck(t *testing.T) {
	cases := []struct {
		scenario string
		wantBin  string // suffix of the reported binary
		failing  []string
	}{
		{"restart-agents", "/restart/bin/gx", nil},
		{"restart-projects", "/restart/bin/gx", []string{"broken"}},
		{"restart-launchd", "/launchd/bin/gx", nil},
	}
	for _, tc := range cases {
		t.Run(tc.scenario, func(t *testing.T) {
			r := start(t, tc.scenario)
			defer r.stop(t)
			rep := r.restartCheck(t)
			if rep.Status != server.RestartCheckOK || rep.NewVersion != defaultNewVersion || !rep.Config.OK {
				t.Fatalf("report = %+v, want ok with version %s", rep, defaultNewVersion)
			}
			if !strings.HasSuffix(rep.Binary, tc.wantBin) {
				t.Errorf("binary = %s, want suffix %s", rep.Binary, tc.wantBin)
			}
			var failing []string
			for _, p := range rep.Projects {
				if !p.OK {
					failing = append(failing, p.Name)
				}
			}
			if strings.Join(failing, ",") != strings.Join(tc.failing, ",") {
				t.Errorf("failing projects = %v, want %v", failing, tc.failing)
			}
		})
	}
}

func TestRestartAgents_CountsTheRunningAgent(t *testing.T) {
	r := start(t, "restart-agents")
	defer r.stop(t)
	deadline := time.Now().Add(15 * time.Second)
	for r.restartCheck(t).Agents < 1 {
		if time.Now().After(deadline) {
			t.Fatal("no agent was counted running")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func TestRestartProjects_ReportsTheMigrationError(t *testing.T) {
	r := start(t, "restart-projects")
	defer r.stop(t)
	var broken *server.RestartCheckProject
	rep := r.restartCheck(t)
	for i, p := range rep.Projects {
		if p.Name == "broken" {
			broken = &rep.Projects[i]
		}
	}
	if broken == nil || broken.OK || broken.Error != "fixture migration failed" {
		t.Errorf("broken project = %+v, want the fixture error", broken)
	}
	if len(rep.Projects) != 2 {
		t.Errorf("projects = %+v, want demo and broken", rep.Projects)
	}
}

func TestRestartScenarios_ControlFilesSteerTheFake(t *testing.T) {
	r := start(t, "restart-agents")
	defer r.stop(t)

	r.setRestart(t, "check.json", `{"version":"v9","config":{"ok":false,"error":"bad config"},"projects":[]}`)
	rep := r.restartCheck(t)
	if rep.NewVersion != "v9" || rep.Config.OK || rep.Config.Error != "bad config" {
		t.Errorf("report = %+v, want v9 with a failed config", rep)
	}

	r.setRestart(t, "old", "")
	if rep := r.restartCheck(t); rep.Status != server.RestartCheckTooOld {
		t.Errorf("status = %q, want %q", rep.Status, server.RestartCheckTooOld)
	}
}

func TestRestartMissing_TheBinaryIsUnreadable(t *testing.T) {
	r := start(t, "restart-missing")
	defer r.stop(t)
	if rep := r.restartCheck(t); rep.Status != server.RestartCheckUnreadable {
		t.Errorf("status = %q, want %q", rep.Status, server.RestartCheckUnreadable)
	}
}

func TestRestartFake_StartFollowsTheStartFile(t *testing.T) {
	r := start(t, "restart-projects") // startsUp is false
	defer r.stop(t)
	bin := r.restartCheck(t).Binary
	out, err := exec.Command(bin, "server", "restart", "--continue").CombinedOutput()
	if err == nil || !strings.Contains(string(out), "start refused") {
		t.Errorf("start with no start file: err=%v out=%q, want a refusal", err, out)
	}
}
