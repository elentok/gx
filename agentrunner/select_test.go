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
		{name: "auto picks herdr when up", setting: "auto", onPath: both, herdrUp: true, want: agentrunner.ChoiceHerdr},
		{name: "empty means auto", setting: "", onPath: both, herdrUp: true, want: agentrunner.ChoiceHerdr},
		{name: "auto falls back to headless when herdr down", setting: "auto", onPath: both, want: agentrunner.ChoiceHeadless},
		{name: "auto falls back to headless without herdr binary", setting: "auto", onPath: []string{"claude"}, herdrUp: true, want: agentrunner.ChoiceHeadless},
		{name: "auto without herdr or claude", setting: "auto", wantErr: "needs claude on PATH"},
		{name: "herdr up", setting: "herdr", onPath: []string{"herdr"}, herdrUp: true, want: agentrunner.ChoiceHerdr},
		{name: "herdr down does not fall back", setting: "herdr", onPath: both, wantErr: "herdr is unavailable"},
		{name: "headless", setting: "headless", onPath: both, herdrUp: true, want: agentrunner.ChoiceHeadless},
		{name: "headless without claude", setting: "headless", onPath: []string{"herdr"}, wantErr: "needs claude on PATH"},
		{name: "pty by hand", setting: "pty", onPath: both, want: agentrunner.ChoicePTY},
		{name: "pty without claude", setting: "pty", wantErr: "needs claude on PATH"},
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

func TestSelectAutoNeverPicksPTY(t *testing.T) {
	for _, onPath := range [][]string{nil, {"claude"}, {"herdr"}, {"herdr", "claude"}} {
		for _, up := range []bool{false, true} {
			if got, _ := agentrunner.Select("auto", probe(onPath, up)); got == agentrunner.ChoicePTY {
				t.Fatalf("auto picked pty with path %v, herdr up %v", onPath, up)
			}
		}
	}
}
