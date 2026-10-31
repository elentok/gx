package nativerunner_test

import (
	"slices"
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/testutil/agentfake"
)

// A start with a registered harness kind runs that harness's command under a
// holder that watches with it: the fake reports ready by OSC 7501 alone, so a
// claude-watching holder would never see the session idle.
func TestPty_StartRunsTheKindsHarness(t *testing.T) {
	t.Parallel()
	p := newPty(t)
	p.Env = ptyTestEnv(t.TempDir(), holderEnv+"=1", agentfake.ProgramIdleEnv+"=1")
	s, err := p.Start(agentrunner.StartOptions{Label: "e-01", Epic: "e", Cwd: t.TempDir(), Kind: agentfake.HarnessKind})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	p.Stop(s)
}

func TestPty_LaunchCommandPicksTheHarnessFromTheKind(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		kind         agentrunner.Kind
		codex        string
		wantName     string
		wantSettings bool
	}{
		{"empty kind is claude", "", "", "/bin/fake-claude", true},
		{"claude", "claude", "", "/bin/fake-claude", true},
		{"codex on PATH", "codex", "", "codex", false},
		{"codex override", "codex", "/bin/fake-codex", "/bin/fake-codex", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &nativerunner.Pty{Claude: "/bin/fake-claude", Codex: tc.codex}
			cmd, err := nativerunner.LaunchCommand(p, agentrunner.StartOptions{Kind: tc.kind, Args: []string{"--caller-arg"}}, t.TempDir(), "sid")
			if err != nil {
				t.Fatal(err)
			}
			if cmd.Name != tc.wantName {
				t.Errorf("Name = %q, want %q", cmd.Name, tc.wantName)
			}
			if !slices.Contains(cmd.Args, "--caller-arg") {
				t.Errorf("Args = %q, want the caller's args kept", cmd.Args)
			}
			if got := slices.Contains(cmd.Args, "--settings"); got != tc.wantSettings {
				t.Errorf("--settings in Args = %v, want %v: %q", got, tc.wantSettings, cmd.Args)
			}
		})
	}
}

func TestPty_LaunchCommandRefusesAnUnknownKind(t *testing.T) {
	t.Parallel()
	if _, err := nativerunner.LaunchCommand(&nativerunner.Pty{}, agentrunner.StartOptions{Kind: "nope"}, t.TempDir(), "sid"); err == nil {
		t.Error("LaunchCommand accepted an unknown kind")
	}
}
