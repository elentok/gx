package nativerunner_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/agentrunner/screen"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/testutil/agentfake"
)

func launchPty(t *testing.T, caps claudeui.LaunchProofCaps, env ...string) (*nativerunner.Pty, agentrunner.Session) {
	t.Helper()
	p := newPty(t)
	p.Env = append(p.Env, env...)
	p.LaunchCaps = caps
	// A worker config makes "/gx-implement x/1" start a turn (it blocks on a
	// permission prompt), as a skill does.
	cwd := t.TempDir()
	cfg := `{"behaviour":{"x/1":"` + agentfake.BehaviourPermission + `"}}`
	if err := os.WriteFile(filepath.Join(cwd, agentfake.WorkerConfigFile), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := p.Start(agentrunner.StartOptions{Label: "e-01", Epic: "e", Cwd: cwd, Kind: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Stop(s) })
	return p, s
}

func TestPty_LaunchPromptThatStartsNoTurnIsNotDelivered(t *testing.T) {
	t.Parallel()
	p, s := launchPty(t, claudeui.LaunchProofCaps{Grace: 600 * time.Millisecond, ResendEvery: time.Hour}, agentfake.TUINoTurnEnv+"=1")

	if err := p.PromptLaunch(s, "/gx-implement x/1", nil); !errors.Is(err, agentrunner.ErrNotDelivered) {
		t.Fatalf("PromptLaunch = %v; want ErrNotDelivered", err)
	}
}

func TestPty_LaunchPromptUnknownCommandFailsFast(t *testing.T) {
	t.Parallel()
	p, s := launchPty(t, claudeui.LaunchProofCaps{Grace: time.Minute, UnknownGrace: 200 * time.Millisecond, ResendEvery: time.Hour}, agentfake.TUIUnknownSkillEnv+"=1")

	start := time.Now()
	err := p.PromptLaunch(s, "/gx-implement x/1", nil)
	if !errors.Is(err, agentrunner.ErrNotDelivered) || !strings.Contains(err.Error(), claudeui.UnknownCommandMarker) {
		t.Fatalf("PromptLaunch = %v; want ErrNotDelivered naming %q", err, claudeui.UnknownCommandMarker)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("PromptLaunch took %v; want seconds", elapsed)
	}
}

func TestPty_LaunchPromptLateRegistryIsResentThenStartsATurn(t *testing.T) {
	t.Parallel()
	p, s := launchPty(t, claudeui.LaunchProofCaps{Grace: 30 * time.Second, UnknownGrace: time.Minute, ResendEvery: 200 * time.Millisecond},
		agentfake.TUIRegistryDelayEnv+"=1s", agentfake.TUILaunchRecordEnv+"=1")
	var mu sync.Mutex
	var decisions []string

	err := p.PromptLaunch(s, "/gx-implement x/1", func(d string) {
		mu.Lock()
		decisions = append(decisions, d)
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("PromptLaunch = %v; want nil once the registry loaded", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !slices.Contains(decisions, string(claudeui.NudgePrompt)) {
		t.Errorf("decisions = %v; want %q", decisions, claudeui.NudgePrompt)
	}
	st, err := p.Status(s)
	if err != nil {
		t.Fatal(err)
	}
	if st.Turn < 1 {
		t.Errorf("Turn = %d; want a started turn", st.Turn)
	}
}

func TestPty_LaunchPromptPersonTypingWithNoProofIsUnconfirmed(t *testing.T) {
	t.Parallel()
	p, s := launchPty(t, claudeui.LaunchProofCaps{Grace: 30 * time.Second, ResendEvery: time.Hour}, agentfake.TUINoTurnEnv+"=1")
	a := attachTo(t, p, s)

	done := make(chan error, 1)
	go func() { done <- p.PromptLaunch(s, "/gx-implement x/1", nil) }()
	waitFakeKeys(t, p, s, "/gx-implement x/1\r")
	if err := a.SendKey(screen.Key{Code: 'x', Text: "x"}); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-done:
		if !errors.Is(err, agentrunner.ErrDeliveryUnconfirmed) {
			t.Fatalf("PromptLaunch = %v; want ErrDeliveryUnconfirmed", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("PromptLaunch kept waiting after a person typed")
	}
}
