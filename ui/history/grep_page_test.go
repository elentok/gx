package history

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/claudehistory"
)

func typeQuery(t *testing.T, m Model, s string) (Model, tea.Cmd) {
	t.Helper()
	var lastCmd tea.Cmd
	for _, r := range s {
		m, lastCmd = press(m, string(r))
	}
	return m, lastCmd
}

// resolveDebounceMsg runs cmd (unpacking any tea.Batch it produced) and
// returns the grepDebounceMsg among its results.
func resolveDebounceMsg(t *testing.T, cmd tea.Cmd) grepDebounceMsg {
	t.Helper()
	if cmd == nil {
		t.Fatal("resolveDebounceMsg: nil cmd")
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, sub := range batch {
			if sub == nil {
				continue
			}
			if debounce, ok := sub().(grepDebounceMsg); ok {
				return debounce
			}
		}
		t.Fatal("resolveDebounceMsg: no grepDebounceMsg in batch")
	}
	debounce, ok := msg.(grepDebounceMsg)
	if !ok {
		t.Fatalf("resolveDebounceMsg: cmd produced %T, want grepDebounceMsg", msg)
	}
	return debounce
}

func TestCtrlF_OpensGrepFromProjects(t *testing.T) {
	m := newTestModel()
	m, _ = update(m, tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	if m.page != pageGrep {
		t.Fatalf("page = %v, want pageGrep", m.page)
	}
	if !m.grepFilter.InputFocused() {
		t.Fatal("expected grep filter to already be accepting keystrokes")
	}
	if m.grepScope != grepScopeGlobal {
		t.Fatal("expected global scope when opened from projects")
	}
}

func TestCtrlF_OpensGrepFromConversations_ScopedToProject(t *testing.T) {
	m := enterConversationsFixture(t)
	m, _ = update(m, tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	if m.page != pageGrep {
		t.Fatalf("page = %v, want pageGrep", m.page)
	}
	if m.grepScope != grepScopeProject {
		t.Fatal("expected project scope when opened from conversations")
	}
	if m.grepScopeProjDir != "gx-main" {
		t.Fatalf("grepScopeProjDir = %q, want gx-main", m.grepScopeProjDir)
	}
}

func TestEsc_ReturnsGrepToOriginPage(t *testing.T) {
	m := enterConversationsFixture(t)
	m, _ = update(m, tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.page != pageConversations {
		t.Fatalf("page = %v, want pageConversations after esc from grep", m.page)
	}
}

func TestGrepDebounce_LatestQueryWinsOverStaleDebounce(t *testing.T) {
	m := newTestModel()
	m, _ = update(m, tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})

	m, cmd1 := press(m, "a")
	m, cmd2 := typeQuery(t, m, "b") // query is now "ab"

	// The earlier debounce (for "a") resolves after being superseded.
	debounce1 := resolveDebounceMsg(t, cmd1)
	m, staleCmd := update(m, debounce1)
	if staleCmd != nil {
		t.Fatal("expected stale debounce to produce no cmd (query changed since)")
	}
	if m.grepRunning {
		t.Fatal("stale debounce must not start a search")
	}

	debounce2 := resolveDebounceMsg(t, cmd2)
	if debounce2.query != "ab" {
		t.Fatalf("debounce2.query = %q, want ab", debounce2.query)
	}
	m, searchCmd := update(m, debounce2)
	if searchCmd == nil {
		t.Fatal("expected the current debounce to start a search")
	}
	if !m.grepRunning {
		t.Fatal("expected grepRunning after the current debounce fires")
	}
}

func TestGrepResults_StaleSearchDiscarded(t *testing.T) {
	fixtureResults := []claudehistory.GrepResult{{SessionID: "sess-1", ConvTitle: "found it"}}
	m := newTestModel()
	m, _ = update(m, tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})

	// Simulate an in-flight search from an earlier query (seq=1) resolving
	// after a newer query has already bumped the sequence (seq=2).
	m.grepSeq = 2
	m, cmd := update(m, grepResultsMsg{results: fixtureResults, seq: 1})
	if cmd != nil {
		t.Fatal("stale results should produce no further cmd")
	}
	if len(m.grepResults) != 0 {
		t.Fatal("stale (seq=1) results must not overwrite current state (seq=2)")
	}

	m, _ = update(m, grepResultsMsg{results: fixtureResults, seq: 2})
	if len(m.grepResults) != 1 || m.grepResults[0].SessionID != "sess-1" {
		t.Fatalf("current-seq results not applied: %+v", m.grepResults)
	}
}

func TestGrepResults_RgNotFound(t *testing.T) {
	m := newTestModel()
	m, _ = update(m, tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	m.grepSeq = 1
	m, _ = update(m, grepResultsMsg{err: claudehistory.ErrRgNotFound, seq: 1})
	if !m.rgNotFound {
		t.Fatal("expected rgNotFound to be set")
	}
	if view := m.viewGrep(); !strings.Contains(view, "ripgrep") {
		t.Fatalf("viewGrep() = %q, want it to mention ripgrep", view)
	}
}

func TestToggleGrepScope_RequeriesUnderNewScope(t *testing.T) {
	m := enterConversationsFixture(t)
	m, _ = update(m, tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	if m.grepScope != grepScopeProject {
		t.Fatal("expected project scope by default from conversations")
	}
	if got := m.grepDirs(); len(got) != 1 || got[0] != "gx-main" {
		t.Fatalf("grepDirs() = %v, want just [gx-main]", got)
	}

	prevSeq := m.grepSeq
	m, cmd := update(m, tea.KeyPressMsg{Code: 'g', Mod: tea.ModCtrl})
	if m.grepScope != grepScopeGlobal {
		t.Fatal("expected ctrl+g to toggle to global scope")
	}
	if got := m.grepDirs(); len(got) != len(fixtureProjects) {
		t.Fatalf("grepDirs() = %v, want all project dirs", got)
	}
	if cmd == nil {
		t.Fatal("expected ctrl+g to re-run the current query")
	}
	msg := cmd()
	debounce, ok := msg.(grepDebounceMsg)
	if !ok {
		t.Fatalf("cmd() produced %T, want grepDebounceMsg", msg)
	}
	if debounce.seq == prevSeq {
		t.Fatal("expected scope toggle to bump the sequence number")
	}

	// Toggling back returns to project scope.
	m, _ = update(m, tea.KeyPressMsg{Code: 'g', Mod: tea.ModCtrl})
	if m.grepScope != grepScopeProject {
		t.Fatal("expected second ctrl+g to toggle back to project scope")
	}
}

func TestGrepDebounce_EmptyQueryClearsResults(t *testing.T) {
	m := newTestModel()
	m, _ = update(m, tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	m.grepResults = []claudehistory.GrepResult{{SessionID: "sess-1"}}
	m, cmd := update(m, grepDebounceMsg{seq: m.grepSeq, query: ""})
	if cmd != nil {
		t.Fatal("expected no search cmd for an empty query")
	}
	if len(m.grepResults) != 0 {
		t.Fatal("expected an empty query to clear existing results")
	}
}

func TestViewGrepPreview_ReflectsSelection(t *testing.T) {
	m := newTestModel()
	m, _ = update(m, tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	m.grepResults = []claudehistory.GrepResult{
		{SessionID: "sess-1", ConvTitle: "First conversation", Preview: "alpha preview text"},
		{SessionID: "sess-2", ConvTitle: "Second conversation", Preview: "bravo preview text"},
	}
	m.grepList.SetSelected(0, len(m.grepResults))

	preview := m.viewGrepPreview()
	if !strings.Contains(preview, "First conversation") || !strings.Contains(preview, "sess-1") || !strings.Contains(preview, "alpha preview text") {
		t.Fatalf("preview for selection 0 missing expected content: %q", preview)
	}
	if strings.Contains(preview, "bravo preview text") {
		t.Fatalf("preview for selection 0 leaked the other result's content: %q", preview)
	}

	m.grepList.SetSelected(1, len(m.grepResults))
	preview = m.viewGrepPreview()
	if !strings.Contains(preview, "Second conversation") || !strings.Contains(preview, "sess-2") || !strings.Contains(preview, "bravo preview text") {
		t.Fatalf("preview for selection 1 missing expected content: %q", preview)
	}
	if strings.Contains(preview, "alpha preview text") {
		t.Fatalf("preview for selection 1 leaked the other result's content: %q", preview)
	}
}

func TestViewGrepPreview_FallsBackToSnippet(t *testing.T) {
	m := newTestModel()
	m, _ = update(m, tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	m.grepResults = []claudehistory.GrepResult{{SessionID: "sess-1", Snippet: "only a snippet"}}
	m.grepList.SetSelected(0, 1)

	preview := m.viewGrepPreview()
	if !strings.Contains(preview, "only a snippet") {
		t.Fatalf("preview = %q, want it to fall back to Snippet when Preview is empty", preview)
	}
}

func TestGrepDebounce_UsesConfiguredDelay(t *testing.T) {
	if grepDebounce != 100*time.Millisecond {
		t.Fatalf("grepDebounce = %v, want 100ms", grepDebounce)
	}
}
