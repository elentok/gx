package nativerunner_test

import (
	"context"
	"errors"
	"log/slog"
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
	// A later entry wins: undo newPty's opt-out, then apply the test's own.
	p.Env = append(p.Env, agentfake.TUIReadyDelayEnv+"=0s")
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

	if err := p.PromptLaunch(context.Background(), s, "/gx-implement x/1", nil); !errors.Is(err, agentrunner.ErrNotDelivered) {
		t.Fatalf("PromptLaunch = %v; want ErrNotDelivered", err)
	}
}

func TestPty_LaunchPromptUnknownCommandFailsFast(t *testing.T) {
	t.Parallel()
	p, s := launchPty(t, claudeui.LaunchProofCaps{Grace: time.Minute, UnknownGrace: 200 * time.Millisecond, ResendEvery: time.Hour}, agentfake.TUIUnknownSkillEnv+"=1")

	start := time.Now()
	err := p.PromptLaunch(context.Background(), s, "/gx-implement x/1", nil)
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
		agentfake.TUIRegistryDelayEnv+"=1s")
	var mu sync.Mutex
	var decisions []string

	err := p.PromptLaunch(context.Background(), s, "/gx-implement x/1", func(d string) {
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

func TestPty_LaunchPromptWaitsForTheReadySignal(t *testing.T) {
	t.Parallel()
	start := time.Now()
	p, s := launchPty(t, claudeui.LaunchProofCaps{Grace: 30 * time.Second, ResendEvery: time.Hour},
		agentfake.TUIReadyDelayEnv+"=1s")

	if err := p.PromptLaunch(context.Background(), s, "/gx-implement x/1", nil); err != nil {
		t.Fatalf("PromptLaunch = %v; want nil", err)
	}
	if elapsed := time.Since(start); elapsed < 900*time.Millisecond {
		t.Errorf("PromptLaunch returned after %v; want it to wait for the ready signal", elapsed)
	}
}

func TestPty_LaunchPromptSendsAnywayWhenNeverReady(t *testing.T) {
	t.Parallel()
	p, s := launchPty(t, claudeui.LaunchProofCaps{Grace: 30 * time.Second, ResendEvery: time.Hour},
		agentfake.TUIReadyDelayEnv+"="+agentfake.TUIReadyOff)
	p.ReadyWait = 400 * time.Millisecond
	var logs strings.Builder
	p.SetLog(slog.New(slog.NewTextHandler(&logs, nil)))

	start := time.Now()
	if err := p.PromptLaunch(context.Background(), s, "/gx-implement x/1", nil); err != nil {
		t.Fatalf("PromptLaunch = %v; want nil", err)
	}
	if elapsed := time.Since(start); elapsed < p.ReadyWait {
		t.Errorf("PromptLaunch returned after %v; want it to wait out %v", elapsed, p.ReadyWait)
	}
	if !strings.Contains(logs.String(), "ready signal missing") {
		t.Errorf("log = %q; want it to say the ready signal was missing", logs.String())
	}
}

func TestPty_LaunchPromptStopsWaitingForReadyWhenCancelled(t *testing.T) {
	t.Parallel()
	p, s := launchPty(t, claudeui.LaunchProofCaps{Grace: 30 * time.Second, ResendEvery: time.Hour},
		agentfake.TUIReadyDelayEnv+"="+agentfake.TUIReadyOff)
	var logs strings.Builder
	p.SetLog(slog.New(slog.NewTextHandler(&logs, nil)))
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- p.PromptLaunch(ctx, s, "/gx-implement x/1", nil) }()
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("PromptLaunch = %v; want context.Canceled", err)
		}
	case <-time.After(nativerunner.DefaultReadyWait / 3):
		t.Fatal("PromptLaunch kept waiting for the ready signal after the cancel")
	}
	if strings.Contains(logs.String(), "ready signal missing") {
		t.Errorf("log = %q; a cancel is not a missing ready signal", logs.String())
	}
}

func TestPty_LaunchPromptStopsDeliveryWaitWhenCancelled(t *testing.T) {
	t.Parallel()
	p, s := launchPty(t, claudeui.LaunchProofCaps{Grace: time.Minute, UnknownGrace: time.Minute, ResendEvery: time.Hour},
		agentfake.TUILaunchRecordEnv+"=0")
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- p.PromptLaunch(ctx, s, "/gx-implement x/1", nil) }()
	// Let the launch reach the delivery wait before cancelling.
	time.Sleep(2 * time.Second)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) || errors.Is(err, agentrunner.ErrNotDelivered) {
			t.Fatalf("PromptLaunch = %v; want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("PromptLaunch kept waiting for delivery after the cancel")
	}
}

func TestPty_LaunchPromptPersonTypingWithNoProofIsUnconfirmed(t *testing.T) {
	t.Parallel()
	p, s := launchPty(t, claudeui.LaunchProofCaps{Grace: 30 * time.Second, ResendEvery: time.Hour}, agentfake.TUINoTurnEnv+"=1")
	a := attachTo(t, p, s)

	done := make(chan error, 1)
	go func() { done <- p.PromptLaunch(context.Background(), s, "/gx-implement x/1", nil) }()
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
