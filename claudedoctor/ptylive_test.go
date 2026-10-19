package claudedoctor_test

import (
	"os"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/claudedoctor"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/testutil/claudeenv"
)

// The test binary doubles as the PTY holder and, under it, as the TUI fake.
const holderEnv = "GX_TEST_HOLDER"

func TestMain(m *testing.M) {
	if os.Getenv(holderEnv) == "1" {
		args := os.Args[1:]
		_ = os.Unsetenv(holderEnv)
		if err := nativerunner.RunHolder(args[slices.Index(args, "--dir")+1], args[slices.Index(args, "--")+1:]); err != nil {
			os.Exit(1)
		}
		return
	}
	if os.Getenv(agentfake.TUIEnv) == "1" {
		agentfake.TUIClaude()
	}
	cleanup := claudeenv.Isolate()
	code := m.Run()
	cleanup()
	os.Exit(code)
}

func TestPtyLive_RecordsTrustFrameAndWaitingSnapshot(t *testing.T) {
	// PtyLive's recorder polls the screen, and Start answers the trust
	// dialog on its own poll, so the dialog can come and go between two
	// samples. The race detector's slowdown makes that miss common; the plain
	// run keeps covering this test.
	if raceEnabled {
		t.Skip("trust frame capture is timing-sensitive under -race")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp("", "gxd")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv(holderEnv, "1")
	t.Setenv(agentfake.TUIEnv, "1")
	t.Setenv(agentfake.TUITrustEnv, "1")
	p := &nativerunner.Pty{Root: root, Holder: []string{exe}, Claude: exe,
		PollInterval: 10 * time.Millisecond, StartTimeout: 10 * time.Second, PromptGrace: 500 * time.Millisecond}

	rec, err := claudedoctor.PtyLive(p, t.TempDir(), func(s agentrunner.Session) error {
		if err := agentfake.SendTUIControl(os.Getenv("HOME"), s.SessionID, agentfake.Control{Action: agentfake.ActionBlock, WaitingFor: "permission prompt"}); err != nil {
			return err
		}
		_, err := p.Wait(s, []agentrunner.State{agentrunner.StateBlocked}, 5*time.Second)
		return err
	})
	if err != nil {
		t.Fatalf("PtyLive: %v", err)
	}
	src := claudedoctor.Source{Session: func(string) (claudedoctor.Recording, error) { return rec, nil }}
	for _, r := range claudedoctor.Run(claudedoctor.Table, "pty", src) {
		if r.Status == claudedoctor.Fail {
			t.Errorf("%s failed on the live recording: %s", r.Name, r.Detail)
		}
	}
}
