package commit

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/diffview"
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

func newAIDiffModel(t *testing.T) Model {
	t.Helper()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "ai.txt", "old-1\nold-2\n")
	testutil.CommitAll(t, repo, "base")
	testutil.WriteFile(t, repo, "ai.txt", "new-1\nnew-2\n")
	testutil.CommitAll(t, repo, "change")

	m := newTestModel(repo, "HEAD")
	m.ready = true
	m.width = 100
	m.height = 24
	m.syncDiffViewport()
	m.focusDiff = true
	m.diffModel.SetNavMode(diffview.NavModeLine)
	return m
}

// 'ay' is the new primary binding for yank-for-AI (moved from 'ya').
func TestAIChord_AYYanksForAI(t *testing.T) {
	var got string
	prev := commitClipboardWrite
	commitClipboardWrite = func(s string) error {
		got = s
		return nil
	}
	t.Cleanup(func() { commitClipboardWrite = prev })

	m := newAIDiffModel(t)
	m = pressKeys(t, m, "ay")

	if !strings.Contains(got, "ai.txt") {
		t.Fatalf("expected 'ay' to yank file context for AI, got %q", got)
	}
}

// 'ya' is kept as a hidden back-compat alias for yank-for-AI.
func TestAIChord_LegacyYAStillYanksForAI(t *testing.T) {
	var got string
	prev := commitClipboardWrite
	commitClipboardWrite = func(s string) error {
		got = s
		return nil
	}
	t.Cleanup(func() { commitClipboardWrite = prev })

	m := newAIDiffModel(t)
	m = pressKeys(t, m, "ya")

	if !strings.Contains(got, "ai.txt") {
		t.Fatalf("expected legacy 'ya' to still yank file context for AI, got %q", got)
	}
}

// 'aa' is the new binding for Ask AI (formerly 'cm' comment).
func TestAIChord_AATriggersAskAI(t *testing.T) {
	m := newAIDiffModel(t)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = updated.(Model)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = updated.(Model)

	if cmd == nil {
		t.Fatal("expected 'aa' in diff focus to trigger Ask AI (open editor) cmd")
	}
}
