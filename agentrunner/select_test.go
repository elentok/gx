package agentrunner_test

import (
	"errors"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
)

func probe(onPath []string, herdrUp bool) agentrunner.Probe {
	return agentrunner.Probe{
		LookPath: func(file string) (string, error) {
			if slices.Contains(onPath, file) {
				return "/bin/" + file, nil
			}
			return "", exec.ErrNotFound
		},
		PingHerdr: func(timeout time.Duration) error {
			if timeout != agentrunner.HerdrPingTimeout {
				return errors.New("unexpected timeout")
			}
			if !herdrUp {
				return errors.New("herdr: connection refused")
			}
			return nil
		},
	}
}

func TestSelectAutoNeverPingsHerdr(t *testing.T) {
	p := probe([]string{"herdr", "claude"}, true)
	p.PingHerdr = func(time.Duration) error {
		t.Fatal("auto pinged herdr")
		return nil
	}
	for _, setting := range []string{"auto", ""} {
		if got, err := agentrunner.Select(setting, p); err != nil || got != agentrunner.ChoicePTY {
			t.Fatalf("Select(%q) = %q, %v; want pty", setting, got, err)
		}
	}
}

func TestSelect(t *testing.T) {
	both := []string{"herdr", "claude"}
	tests := []struct {
		name    string
		setting string
		onPath  []string
		herdrUp bool
		want    agentrunner.Choice
		wantErr string
	}{
		{name: "auto picks pty when herdr up", setting: "auto", onPath: both, herdrUp: true, want: agentrunner.ChoicePTY},
		{name: "auto picks pty when herdr down", setting: "auto", onPath: both, want: agentrunner.ChoicePTY},
		{name: "auto picks pty without herdr binary", setting: "auto", onPath: []string{"claude"}, want: agentrunner.ChoicePTY},
		{name: "empty means auto", setting: "", onPath: both, herdrUp: true, want: agentrunner.ChoicePTY},
		{name: "auto without claude", setting: "auto", onPath: []string{"herdr"}, herdrUp: true, wantErr: "agent-runner: herdr"},
		{name: "auto without claude names claude", setting: "", wantErr: "needs claude on PATH"},
		{name: "herdr up", setting: "herdr", onPath: []string{"herdr"}, herdrUp: true, want: agentrunner.ChoiceHerdr},
		{name: "herdr down does not fall back", setting: "herdr", onPath: both, wantErr: "herdr is unavailable"},
		{name: "headless", setting: "headless", onPath: both, herdrUp: true, want: agentrunner.ChoiceHeadless},
		{name: "headless without claude", setting: "headless", onPath: []string{"herdr"}, wantErr: "needs claude on PATH"},
		{name: "pty", setting: "pty", onPath: both, herdrUp: true, want: agentrunner.ChoicePTY},
		{name: "pty without claude", setting: "pty", onPath: []string{"herdr"}, herdrUp: true, wantErr: "needs claude on PATH"},
		{name: "bad value", setting: "tmux", onPath: both, herdrUp: true, wantErr: `invalid agent-runner "tmux"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := agentrunner.Select(tt.setting, probe(tt.onPath, tt.herdrUp))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Select() err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("Select() = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}
