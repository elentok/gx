package claudedoctor_test

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/claudedoctor"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/testutil/agentfake"
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
	os.Exit(m.Run())
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
	home := t.TempDir()
	p := &nativerunner.Pty{Root: root, Holder: []string{exe}, Claude: exe,
		Env:          []string{"HOME=" + home, "CLAUDE_CONFIG_DIR=", holderEnv + "=1", agentfake.TUIEnv + "=1", agentfake.TUITrustEnv + "=1"},
		PollInterval: 10 * time.Millisecond, StartTimeout: 10 * time.Second, PromptGrace: 500 * time.Millisecond}

	rec, err := claudedoctor.PtyLive(p, t.TempDir(), func(s agentrunner.Session) error {
		if err := agentfake.SendTUIControl(home, s.SessionID, agentfake.Control{Action: agentfake.ActionBlock, WaitingFor: "permission prompt"}); err != nil {
			return err
		}
		_, err := p.Wait(s, []agentrunner.State{agentrunner.StateBlocked}, 5*time.Second)
		return err
	})
	if err != nil {
		t.Fatalf("PtyLive: %v", err)
	}
	src := claudedoctor.Source{Live: true, Session: func(name string) (claudedoctor.Recording, error) {
		if name == "pty-rate-limit" { // a normal reply: a live run is never rate limited
			return claudedoctor.Recording{Events: []json.RawMessage{
				json.RawMessage(`{"type":"assistant","message":{"content":"ok"}}`),
			}}, nil
		}
		return rec, nil
	}}
	// holder-survives-kickstart needs launchd; it is not under test here.
	var table []claudedoctor.Check
	for _, c := range claudedoctor.Table {
		if c.Name != "holder-survives-kickstart" {
			table = append(table, c)
		}
	}
	ran := map[string]bool{}
	for _, r := range claudedoctor.Run(table, "pty", src) {
		ran[r.Name] = r.Status != claudedoctor.Skip
		// The recording ends at the trust dialog; no permission dialog is shown.
		if r.Status == claudedoctor.Fail && r.Name != "permission-dialog" {
			t.Errorf("%s failed on the live recording: %s", r.Name, r.Detail)
		}
	}
	if !ran["rate-limit-transcript"] {
		t.Error("rate-limit-transcript was skipped; it should run on a live transcript")
	}
}
