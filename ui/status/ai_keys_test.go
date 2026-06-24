package status

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/status/diffarea"
)

// pressKeys feeds a sequence of single-character key presses to the model.
func pressKeys(t *testing.T, m Model, keys string) Model {
	t.Helper()
	for _, r := range keys {
		updated, _ := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = updated.(Model)
	}
	return m
}

func newDiffLineModeModel(t *testing.T) Model {
	t.Helper()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "c.txt", "old-1\nold-2\n")
	testutil.MustGitExported(t, repo, "add", "c.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
	testutil.WriteFile(t, repo, "c.txt", "new-1\nnew-2\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusDiff
	m.diffarea.ActiveSection = diffarea.SectionUnstaged
	m.diffarea.SetNavMode(diffview.NavModeLine)
	return m
}

// 'ay' is the new primary binding for yank-for-AI (moved from 'ya').
func TestAIChord_AYYanksForAI(t *testing.T) {
	var got string
	prev := stageClipboardWrite
	stageClipboardWrite = func(s string) error {
		got = s
		return nil
	}
	t.Cleanup(func() { stageClipboardWrite = prev })

	m := newDiffLineModeModel(t)
	m = pressKeys(t, m, "jay")

	if !strings.Contains(got, "@c.txt L") {
		t.Fatalf("expected 'ay' to yank file+line for AI, got %q", got)
	}
}

// 'ya' is kept as a hidden back-compat alias for yank-for-AI.
func TestAIChord_LegacyYAStillYanksForAI(t *testing.T) {
	var got string
	prev := stageClipboardWrite
	stageClipboardWrite = func(s string) error {
		got = s
		return nil
	}
	t.Cleanup(func() { stageClipboardWrite = prev })

	m := newDiffLineModeModel(t)
	m = pressKeys(t, m, "jya")

	if !strings.Contains(got, "@c.txt L") {
		t.Fatalf("expected legacy 'ya' to still yank file+line for AI, got %q", got)
	}
}

// 'aa' is the new binding for Ask AI (formerly 'cm' comment).
func TestAIChord_AATriggersAskAI(t *testing.T) {
	m := newDiffLineModeModel(t)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = updated.(Model)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = updated.(Model)

	if cmd == nil {
		t.Fatal("expected 'aa' in diff focus to trigger Ask AI (open editor) cmd")
	}
}
