package ptycodex

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
	"github.com/elentok/gx/agentrunner/codexui"
	gxharness "github.com/elentok/gx/harness"
	"github.com/elentok/gx/testutil/agentfake/codex"
)

const testNoteFile = "/store/e/launch/01-note.md"

// launchedInstructions starts a Codex session with opts and returns the
// developer_instructions value of its recorded launch argv ("" when absent).
func launchedInstructions(t *testing.T, noteFile string) string {
	t.Helper()
	p, home := codexRunner(t)
	s, err := p.Start(agentrunner.StartOptions{
		Label: "e-01", Epic: "e", Cwd: realDir(t), Kind: gxharness.Codex,
		Args:     gxharness.CodexArgs(gxharness.CodexArgsInput{}),
		NoteFile: noteFile,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Stop(s) })
	st, err := p.Wait(s, []agentrunner.State{agentrunner.StateIdle}, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	launches, err := codex.ReadCodexLaunches(home, st.SessionID)
	if err != nil || len(launches) != 1 {
		t.Fatalf("launches = %v, %v; want one", launches, err)
	}
	for i, a := range launches[0] {
		if i > 0 && launches[0][i-1] == "-c" && strings.HasPrefix(a, "developer_instructions=") {
			return strings.TrimPrefix(a, "developer_instructions=")
		}
	}
	return ""
}

func TestPtyCodex_NoteFileSentenceFollowsTheLoopPrompt(t *testing.T) {
	t.Parallel()
	got := launchedInstructions(t, testNoteFile)
	want := `"` + gxharness.GXSystemPromptFor(gxharness.Codex) + " " + gxharness.GXNoteSentence(testNoteFile) + `"`
	if got != want {
		t.Errorf("developer_instructions = %s; want %s", got, want)
	}
	if strings.Contains(got, "\n") {
		t.Errorf("developer_instructions has a newline: %q", got)
	}
}

func TestPtyCodex_NoNoteKeepsTheLoopPromptAlone(t *testing.T) {
	t.Parallel()
	got := launchedInstructions(t, "")
	if want := `"` + gxharness.GXSystemPromptFor(gxharness.Codex) + `"`; got != want {
		t.Errorf("developer_instructions = %s; want %s", got, want)
	}
}

func TestPtyCodex_LaunchPromptIsDeliveredByTheRolloutsUserRecord(t *testing.T) {
	t.Parallel()
	p, _ := codexRunner(t)
	p.LaunchCaps = claudeui.LaunchProofCaps{Grace: 10 * time.Second, ResendEvery: time.Hour}
	s := startCodex(t, p, realDir(t))

	if err := p.PromptLaunch(context.Background(), s, "$gx-implement x/1", nil); err != nil {
		t.Fatalf("PromptLaunch = %v; want nil once the rollout holds the prompt", err)
	}
}

func TestPtyCodex_UnknownSkillLaunchFailsFastWithErrUnknownSkill(t *testing.T) {
	t.Parallel()
	p, _ := codexRunner(t, codex.CodexUnknownSkillEnv+"=1")
	p.LaunchCaps = claudeui.LaunchProofCaps{Grace: time.Minute, UnknownGrace: 200 * time.Millisecond, ResendEvery: time.Hour}
	s := startCodex(t, p, realDir(t))

	start := time.Now()
	err := p.PromptLaunch(context.Background(), s, "$gx-implement x/1", nil)

	if !errors.Is(err, agentrunner.ErrUnknownSkill) || !strings.Contains(err.Error(), codexui.UnknownCommandMarker) {
		t.Fatalf("PromptLaunch = %v; want ErrUnknownSkill naming %q", err, codexui.UnknownCommandMarker)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("PromptLaunch took %v; want seconds", elapsed)
	}
}

func TestPtyCodex_UnknownUntilInstalledSkillLaunchesOnceInstalled(t *testing.T) {
	t.Parallel()
	p, codexHome := codexRunner(t, codex.CodexUnknownUntilInstalledEnv+"=1")
	p.LaunchCaps = claudeui.LaunchProofCaps{Grace: time.Minute, UnknownGrace: 200 * time.Millisecond, ResendEvery: time.Hour}
	s := startCodex(t, p, realDir(t))

	if err := p.PromptLaunch(context.Background(), s, "$gx-implement x/1", nil); !errors.Is(err, agentrunner.ErrUnknownSkill) {
		t.Fatalf("first PromptLaunch = %v; want ErrUnknownSkill", err)
	}
	skillDir := filepath.Join(filepath.Dir(codexHome), ".agents", "skills", "gx-implement")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p.LaunchCaps = claudeui.LaunchProofCaps{Grace: 10 * time.Second, UnknownGrace: 5 * time.Second, ResendEvery: time.Hour}

	if err := p.PromptLaunch(context.Background(), s, "$gx-implement x/1", nil); err != nil {
		t.Fatalf("PromptLaunch after the install = %v; want nil", err)
	}
}

func TestPtyCodex_DroppedLaunchPromptIsResentThenNotDelivered(t *testing.T) {
	t.Skip("the PTY launch has no retype until deliver-primitive-impl/21 (pty-launch-policy)")
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
