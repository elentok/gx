package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner"
)

func TestPreflightAgentRunner(t *testing.T) {
	found := func(string) (string, error) { return "/bin/x", nil }
	missing := func(string) (string, error) { return "", errors.New("not found") }
	up := func(time.Duration) error { return nil }
	down := func(time.Duration) error { return errors.New("connection refused") }

	tests := []struct {
		name      string
		setting   string
		probe     agentrunner.Probe
		wantErr   string
		wantLevel string
	}{
		{"bad value", "tmux", agentrunner.Probe{LookPath: found, PingHerdr: up}, `invalid agent-runner "tmux"`, "ERROR"},
		{"explicit herdr down", "herdr", agentrunner.Probe{LookPath: found, PingHerdr: down}, "herdr is unavailable", "ERROR"},
		{"headless without claude", "headless", agentrunner.Probe{LookPath: missing, PingHerdr: up}, "needs claude on PATH", "ERROR"},
		{"auto picks herdr", "auto", agentrunner.Probe{LookPath: found, PingHerdr: up}, "", "INFO"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logPath := filepath.Join(t.TempDir(), "state", "server.log")
			_, err := preflightAgentRunner(tt.setting, tt.probe, logPath)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("err = %v, want nil", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
			}

			data, rerr := os.ReadFile(logPath)
			if rerr != nil {
				t.Fatal(rerr)
			}
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			if len(lines) != 1 {
				t.Fatalf("log lines = %d, want 1: %s", len(lines), data)
			}
			var rec map[string]any
			if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
				t.Fatalf("log line not JSON: %v", err)
			}
			if rec["level"] != tt.wantLevel || rec["setting"] != tt.setting {
				t.Errorf("log = %s, want level %s setting %s", lines[0], tt.wantLevel, tt.setting)
			}
			if tt.wantErr != "" && !strings.Contains(rec["err"].(string), tt.wantErr) {
				t.Errorf("log err = %v, want containing %q", rec["err"], tt.wantErr)
			}
		})
	}
}

func TestServerRunner_HeadlessAgentDirsLiveUnderTheProjectsAgentsRoot(t *testing.T) {
	state := t.TempDir()
	_, runnerFor := serverRunner(agentrunner.ChoiceHeadless, state)
	h, ok := runnerFor("headless", "proj").(*nativerunner.Headless)
	if !ok {
		t.Fatalf("runnerFor(headless, proj) = %T, want *Headless", runnerFor("headless", "proj"))
	}
	if want := nativerunner.AgentsRoot(state, "proj"); h.Root != want {
		t.Errorf("Root = %q, want %q", h.Root, want)
	}
	if runnerFor("headless", "proj") != agentrunner.Runner(h) {
		t.Error("a project must keep one Headless: its live sessions are in memory")
	}
}

func TestServerRunner_PtyAgentDirsLiveUnderTheProjectsAgentsRoot(t *testing.T) {
	state := t.TempDir()
	r, runnerFor := serverRunner(agentrunner.ChoicePTY, state)
	if r.Name() != "pty" {
		t.Errorf("default runner Name = %q, want pty", r.Name())
	}
	p, ok := runnerFor("pty", "proj").(*nativerunner.Pty)
	if !ok {
		t.Fatalf("runnerFor(pty, proj) = %T, want *Pty", runnerFor("pty", "proj"))
	}
	if want := nativerunner.AgentsRoot(state, "proj"); p.Root != want {
		t.Errorf("Root = %q, want %q", p.Root, want)
	}
	if runnerFor("pty", "proj") != agentrunner.Runner(p) {
		t.Error("a project must keep one Pty: its live sessions are in memory")
	}
}

func TestServerRunner_BuildsAnyRunnerPerPairLazily(t *testing.T) {
	_, runnerFor := serverRunner(agentrunner.ChoiceHeadless, t.TempDir())
	herdrA := runnerFor("herdr", "a")
	if herdrA == nil || herdrA.Name() != "herdr" {
		t.Fatalf("runnerFor(herdr, a) = %v, want a herdr runner", herdrA)
	}
	if runnerFor("herdr", "a") != herdrA {
		t.Error("a (runner, project) pair must be built once and kept")
	}
	if runnerFor("herdr", "b") == herdrA {
		t.Error("each project gets its own runner")
	}
	if runnerFor("headless", "a") == herdrA {
		t.Error("each runner name gets its own runner")
	}
	if r := runnerFor("tmux", "a"); r != nil {
		t.Errorf("unknown runner = %v, want nil", r)
	}
}

func TestServerRunner_HeadlessDefaultRunnerHasARoot(t *testing.T) {
	r, _ := serverRunner(agentrunner.ChoiceHeadless, t.TempDir())
	if h, ok := r.(*nativerunner.Headless); !ok || h.Root == "" {
		t.Errorf("default runner = %#v, want a *Headless with a Root", r)
	}
}

func TestServerRunner_NamesComeFromTheRunners(t *testing.T) {
	if r, _ := serverRunner(agentrunner.ChoiceHerdr, t.TempDir()); r.Name() != "herdr" {
		t.Errorf("herdr Name = %q", r.Name())
	}
	if r, _ := serverRunner(agentrunner.ChoiceHeadless, t.TempDir()); r.Name() != "headless" {
		t.Errorf("headless Name = %q", r.Name())
	}
}

func TestServerRunner_HerdrServerCanBuildHeadless(t *testing.T) {
	_, runnerFor := serverRunner(agentrunner.ChoiceHerdr, t.TempDir())
	if _, ok := runnerFor("headless", "proj").(*nativerunner.Headless); !ok {
		t.Error("a herdr server must still reach a headless run's runner")
	}
}
