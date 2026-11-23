package nativerunner_test

import (
	"context"
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
	"github.com/elentok/gx/agentrunner/deliver"
	"github.com/elentok/gx/agentrunner/screen"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/testutil/claudeenv"
)

func launchPty(t *testing.T, caps deliver.LaunchTimings, env ...string) (*nativerunner.Pty, agentrunner.Session) {
	t.Helper()
	p := newPty(t)
	// newPty's plain screen is dropped: a launch prompt waits for the input box.
	p.Env = slices.DeleteFunc(p.Env, func(kv string) bool { return strings.HasPrefix(kv, agentfake.TUIPlainScreenEnv+"=") })
	// Drop what env overrides: the child's os.Getenv reads the first duplicate.
	p.Env = slices.DeleteFunc(p.Env, func(kv string) bool {
		k, _, _ := strings.Cut(kv, "=")
		return slices.ContainsFunc(env, func(e string) bool { return strings.HasPrefix(e, k+"=") })
	})
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
	p, s := launchPty(t, deliver.LaunchTimings{Grace: 600 * time.Millisecond, RetypeEvery: 50 * time.Millisecond}, agentfake.TUINoTurnEnv+"=1")
	var decisions []string

	if err := p.PromptLaunch(context.Background(), s, "/gx-implement x/1", func(d string) { decisions = append(decisions, d) }); !errors.Is(err, agentrunner.ErrNotDelivered) {
		t.Fatalf("PromptLaunch = %v; want ErrNotDelivered", err)
	}
	if typed := strings.Count(fakeKeysTyped(t, p, s), "/gx-implement x/1\r"); typed != 1 || len(decisions) != 0 {
		t.Errorf("typed %d times with decisions %v; want the command once and no retype", typed, decisions)
	}
}

func TestPty_LaunchPromptUnknownCommandFailsFast(t *testing.T) {
	t.Parallel()
	p, s := launchPty(t, deliver.LaunchTimings{Grace: time.Minute, Window: 200 * time.Millisecond, RetypeEvery: time.Hour}, agentfake.TUIUnknownSkillEnv+"=1")

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
	p, s := launchPty(t, deliver.LaunchTimings{Grace: 30 * time.Second, Window: time.Minute, RetypeEvery: 200 * time.Millisecond, Poll: 20 * time.Millisecond},
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
	// Each retype answers one Unknown command: the keys hold the first type plus one per decision.
	if typed := strings.Count(fakeKeysTyped(t, p, s), "/gx-implement x/1\r"); typed != 1+len(decisions) {
		t.Errorf("command typed %d times for %d retypes; want one more type than retypes", typed, len(decisions))
	}
	st, err := p.Status(s)
	if err != nil {
		t.Fatal(err)
	}
	if st.Turn < 1 {
		t.Errorf("Turn = %d; want a started turn", st.Turn)
	}
}

func TestPty_LaunchPromptPastedAsPlainTextIsItsOwnNotDeliveredError(t *testing.T) {
	t.Parallel()
	p, s := launchPty(t, deliver.LaunchTimings{Grace: 30 * time.Second, RetypeEvery: time.Hour}, agentfake.TUIPasteOverEnv+"=5")
	var decisions []string

	err := p.PromptLaunch(context.Background(), s, "/gx-implement x/1", func(d string) { decisions = append(decisions, d) })

	if !errors.Is(err, claudeui.ErrPastedLaunch) || !errors.Is(err, agentrunner.ErrNotDelivered) {
		t.Fatalf("PromptLaunch = %v; want ErrPastedLaunch wrapping ErrNotDelivered", err)
	}
	if _, hung := errors.AsType[*claudeui.HungPaneError](err); hung {
		t.Errorf("PromptLaunch = %v; must not be a hung pane", err)
	}
	if len(decisions) != 0 {
		t.Errorf("decisions = %v; want none", decisions)
	}
}

func TestPty_LaunchPromptThatLandsAtOnceLogsNoNudge(t *testing.T) {
	t.Parallel()
	p, s := launchPty(t, deliver.LaunchTimings{Grace: 30 * time.Second, RetypeEvery: 50 * time.Millisecond})
	var decisions []string

	if err := p.PromptLaunch(context.Background(), s, "/gx-implement x/1", func(d string) { decisions = append(decisions, d) }); err != nil {
		t.Fatalf("PromptLaunch = %v; want nil", err)
	}
	if len(decisions) != 0 {
		t.Errorf("decisions = %v; want no prompt-nudged for a launch whose record lands", decisions)
	}
}

func TestPty_LaunchPromptIsDeliveredWithoutWaitingForATranscript(t *testing.T) {
	t.Parallel()
	p, s := launchPty(t, deliver.LaunchTimings{Grace: 30 * time.Second, RetypeEvery: time.Hour})
	if matches, _ := filepath.Glob(filepath.Join(ptyHome(p), ".claude", "projects", "*", s.SessionID+".jsonl")); len(matches) != 0 {
		t.Fatalf("transcripts before the prompt = %v; want none", matches)
	}

	start := time.Now()
	if err := p.PromptLaunch(context.Background(), s, "/gx-implement x/1", nil); err != nil {
		t.Fatalf("PromptLaunch = %v; want nil", err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("PromptLaunch took %v; want seconds, not a transcript wait", elapsed)
	}
}

func TestPty_NeverReadyAgentFailsStartAndSendsNoPrompt(t *testing.T) {
	t.Parallel()
	p := newPty(t)
	p.Env = append(p.Env, agentfake.TUINoPidFileEnv+"=1")
	p.StartTimeout = 500 * time.Millisecond

	// A failed Start returns no session, so there is nothing to prompt.
	if _, err := p.Start(agentrunner.StartOptions{Label: "e-01", Epic: "e", Cwd: t.TempDir(), Kind: "claude"}); err == nil {
		t.Fatal("Start succeeded; want a failure when the agent never turns idle")
	}
}

func TestPty_LaunchPromptProvesDeliveryUnderACustomConfigDir(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	env := claudeenv.In(home)
	cfg := ""
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, "CLAUDE_CONFIG_DIR="); ok {
			cfg = v
		}
	}
	p, s := launchPty(t, deliver.LaunchTimings{Grace: 30 * time.Second, RetypeEvery: time.Hour}, env...)

	if err := p.PromptLaunch(context.Background(), s, "/gx-implement x/1", nil); err != nil {
		t.Fatalf("PromptLaunch = %v; want nil", err)
	}
	if matches, _ := filepath.Glob(filepath.Join(cfg, "projects", "*", s.SessionID+".jsonl")); len(matches) != 1 {
		t.Errorf("transcripts under %s = %v; want one", cfg, matches)
	}
}

func TestPty_LaunchPromptStopsDeliveryWaitWhenCancelled(t *testing.T) {
	t.Parallel()
	p, s := launchPty(t, deliver.LaunchTimings{Grace: time.Minute, Window: time.Minute, RetypeEvery: time.Hour},
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
	p, s := launchPty(t, deliver.LaunchTimings{Grace: 30 * time.Second, RetypeEvery: time.Hour}, agentfake.TUINoTurnEnv+"=1")
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
