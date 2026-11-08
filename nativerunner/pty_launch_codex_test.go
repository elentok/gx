package nativerunner_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/testutil/agentfake/codex"
)

func TestPtyCodex_LaunchPromptIsDeliveredByTheRolloutsUserRecord(t *testing.T) {
	t.Parallel()
	p, _ := codexRunner(t)
	p.LaunchCaps = claudeui.LaunchProofCaps{Grace: 10 * time.Second, ResendEvery: time.Hour}
	s := startCodex(t, p, realDir(t))

	if err := p.PromptLaunch(context.Background(), s, "$gx-implement x/1", nil); err != nil {
		t.Fatalf("PromptLaunch = %v; want nil once the rollout holds the prompt", err)
	}
}

func TestPtyCodex_DroppedLaunchPromptIsResentThenNotDelivered(t *testing.T) {
	t.Parallel()
	p, _ := codexRunner(t, codex.CodexDropPromptEnv+"=1")
	p.LaunchCaps = claudeui.LaunchProofCaps{Grace: 1500 * time.Millisecond, ResendEvery: 300 * time.Millisecond}
	s := startCodex(t, p, realDir(t))
	var mu sync.Mutex
	var decisions []string

	err := p.PromptLaunch(context.Background(), s, "$gx-implement x/1", func(d string) {
		mu.Lock()
		decisions = append(decisions, d)
		mu.Unlock()
	})

	if !errors.Is(err, agentrunner.ErrNotDelivered) || !strings.Contains(err.Error(), "no user record after") {
		t.Fatalf("PromptLaunch = %v; want ErrNotDelivered saying no user record after the grace", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !slices.Contains(decisions, string(claudeui.NudgePrompt)) {
		t.Errorf("decisions = %v; want %q", decisions, claudeui.NudgePrompt)
	}
}
