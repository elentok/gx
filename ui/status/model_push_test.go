package status

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/push"
)

const testPRURL = "https://github.com/owner/repo/pull/new/feature"

func TestInputFocused_FalseByDefault(t *testing.T) {
	repo := testutil.TempRepo(t)
	m := newTestModelDefault(repo)
	if m.InputFocused() {
		t.Error("expected InputFocused=false by default")
	}
}

// TestHandlePushUpdateForwardsCmdOnDone verifies that when push completes with a PR
// URL the URL-open command is not discarded.
func TestHandlePushUpdateForwardsCmdOnDone(t *testing.T) {
	repo := testutil.TempRepo(t)
	m := newTestModelDefault(repo)
	m.push = push.New()
	m.push.OpenAtPRPrompt(testPRURL)

	_, cmd := m.handlePushUpdate(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected non-nil cmd when push completes with PR URL")
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected tea.BatchMsg from handlePushUpdate, got %T — URL-open cmd may have been dropped", msg)
	}
	// cmd (URL opener) + notify = at least 2 (refresh may be nil for this minimal model)
	if len(batch) < 2 {
		t.Fatalf("expected at least 2 cmds in batch (URL opener + notify), got %d — URL-open cmd may have been dropped", len(batch))
	}
}
