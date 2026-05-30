package search

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestNewModel_InitialState(t *testing.T) {
	m := NewModel()
	if m.Query() != "" {
		t.Errorf("Query = %q, want empty", m.Query())
	}
	if m.HasQuery() {
		t.Error("HasQuery should be false for empty query")
	}
	if m.Cursor() != 0 {
		t.Errorf("Cursor = %d, want 0", m.Cursor())
	}
	if m.Mode() != SearchModeNone {
		t.Errorf("Mode = %d, want SearchModeNone", m.Mode())
	}
	if m.IsActive() {
		t.Error("IsActive should be false initially")
	}
	if m.MatchesCount() != 0 {
		t.Errorf("MatchesCount = %d, want 0", m.MatchesCount())
	}
}

func TestStart_SetsInputMode(t *testing.T) {
	m := NewModel()
	m.Start("foo")
	if m.Mode() != SearchModeInput {
		t.Errorf("Mode = %d, want SearchModeInput", m.Mode())
	}
	if m.Query() != "foo" {
		t.Errorf("Query = %q, want 'foo'", m.Query())
	}
	if !m.IsActive() {
		t.Error("IsActive should be true after Start")
	}
	if !m.HasQuery() {
		t.Error("HasQuery should be true after Start with non-empty query")
	}
}

func TestDismissAndClear_ResetsAll(t *testing.T) {
	m := NewModel()
	m.Start("bar")
	m.SetMatches([]Match{{DataIndex: 1}})
	m.DismissAndClear()
	if m.Mode() != SearchModeNone {
		t.Errorf("Mode = %d, want SearchModeNone", m.Mode())
	}
	if m.Query() != "" {
		t.Errorf("Query = %q, want empty", m.Query())
	}
	if m.MatchesCount() != 0 {
		t.Errorf("MatchesCount = %d, want 0", m.MatchesCount())
	}
}

func TestDismissAndKeepResults_WithMatches_GoesToResults(t *testing.T) {
	m := NewModel()
	m.Start("baz")
	m.SetMatches([]Match{{DataIndex: 2, ViewportRow: 2}})
	m.DismissAndKeepResults()
	if m.Mode() != SearchModeResults {
		t.Errorf("Mode = %d, want SearchModeResults", m.Mode())
	}
	if m.Query() != "baz" {
		t.Errorf("Query = %q, want 'baz'", m.Query())
	}
}

func TestDismissAndKeepResults_NoMatches_Clears(t *testing.T) {
	m := NewModel()
	m.Start("x")
	// no matches set
	m.DismissAndKeepResults()
	if m.Mode() != SearchModeNone {
		t.Errorf("Mode = %d, want SearchModeNone with no matches", m.Mode())
	}
}

func TestSetCursor(t *testing.T) {
	m := NewModel()
	m.SetCursor(3)
	if m.Cursor() != 3 {
		t.Errorf("Cursor = %d, want 3", m.Cursor())
	}
}

func TestMatch_OutOfRange(t *testing.T) {
	m := NewModel()
	_, ok := m.Match(0)
	if ok {
		t.Error("Match(0) should return ok=false when no matches")
	}
	_, ok = m.Match(-1)
	if ok {
		t.Error("Match(-1) should return ok=false")
	}
}

func TestMatch_Valid(t *testing.T) {
	m := NewModel()
	m.SetMatches([]Match{{DataIndex: 5, ViewportRow: 3}})
	match, ok := m.Match(0)
	if !ok {
		t.Fatal("Match(0) should return ok=true")
	}
	if match.DataIndex != 5 {
		t.Errorf("match.DataIndex = %d, want 5", match.DataIndex)
	}
}

func TestSetMatches_ClampsCursor(t *testing.T) {
	m := NewModel()
	m.SetCursor(10) // out of future range
	m.SetMatches([]Match{{DataIndex: 1}, {DataIndex: 2}})
	if m.Cursor() != 0 {
		t.Errorf("cursor should be clamped to 0, got %d", m.Cursor())
	}
}

func TestSetPassiveResults(t *testing.T) {
	m := NewModel()
	m.SetPassiveResults("query", []Match{{DataIndex: 4}})
	if m.Query() != "query" {
		t.Errorf("Query = %q, want 'query'", m.Query())
	}
	if m.Mode() != SearchModeNone {
		t.Errorf("Mode = %d, want SearchModeNone", m.Mode())
	}
	if m.MatchesCount() != 1 {
		t.Errorf("MatchesCount = %d, want 1", m.MatchesCount())
	}
}

func TestMatches_ReturnsSlice(t *testing.T) {
	m := NewModel()
	m.SetMatches([]Match{{DataIndex: 1}, {DataIndex: 2}})
	got := m.Matches()
	if len(got) != 2 {
		t.Errorf("Matches() len = %d, want 2", len(got))
	}
}

func TestKeys_NonEmpty(t *testing.T) {
	m := NewModel()
	k := m.Keys()
	if len(k.Bindings()) == 0 {
		t.Error("Keys() should return non-empty bindings")
	}
}

func TestHighlight_NoQuery(t *testing.T) {
	got := Highlight("hello world", "", false)
	if got != "hello world" {
		t.Errorf("Highlight with empty query = %q, want unchanged", got)
	}
}

func TestHighlight_NoMatch(t *testing.T) {
	got := Highlight("hello world", "xyz", false)
	if got != "hello world" {
		t.Errorf("Highlight no match = %q, want unchanged", got)
	}
}

func TestHighlight_MatchFound(t *testing.T) {
	got := Highlight("hello world", "world", false)
	if got == "hello world" {
		t.Error("expected Highlight to add styling when match found")
	}
}

func TestHighlight_ActiveStyle(t *testing.T) {
	inactive := Highlight("hello", "hello", false)
	active := Highlight("hello", "hello", true)
	if inactive == active {
		t.Error("expected different styles for active vs inactive highlight")
	}
}

func TestInit_ReturnsNil(t *testing.T) {
	m := NewModel()
	if m.Init() != nil {
		t.Error("Init() should return nil")
	}
}

func TestSetWidth(t *testing.T) {
	m := NewModel()
	m.SetWidth(80)
	// just verify it doesn't panic; width is stored internally
}

func TestSetMatchesAndJump_WithMatches(t *testing.T) {
	m := NewModel()
	m.SetCursor(0)
	m.SetMatches([]Match{{DataIndex: 5, ViewportRow: 2}})
	cmd := m.SetMatchesAndJump([]Match{{DataIndex: 5, ViewportRow: 2}})
	if cmd == nil {
		t.Error("expected non-nil cmd when matches exist")
	}
}

func TestSetMatchesAndJump_Empty(t *testing.T) {
	m := NewModel()
	cmd := m.SetMatchesAndJump([]Match{})
	if cmd != nil {
		t.Error("expected nil cmd for empty matches")
	}
}

func TestUpdate_SlashActivates(t *testing.T) {
	m := NewModel()
	next, _, result := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	if next.Mode() != SearchModeInput {
		t.Errorf("expected SearchModeInput, got %d", next.Mode())
	}
	if !result.Activated {
		t.Error("expected Activated=true")
	}
}

func TestUpdate_EscClears(t *testing.T) {
	m := NewModel()
	m.Start("foo")
	next, _, result := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if next.Mode() != SearchModeNone {
		t.Errorf("expected SearchModeNone, got %d", next.Mode())
	}
	if !result.Handled {
		t.Error("expected Handled=true on esc")
	}
}

func TestUpdate_EnterDismissesWithResults(t *testing.T) {
	m := NewModel()
	m.Start("foo")
	m.SetMatches([]Match{{DataIndex: 1}})
	next, _, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if next.Mode() != SearchModeResults {
		t.Errorf("expected SearchModeResults after enter, got %d", next.Mode())
	}
}

func TestUpdate_ResultsNavNext(t *testing.T) {
	m := NewModel()
	m.Start("foo")
	m.SetMatches([]Match{{DataIndex: 1}, {DataIndex: 2}})
	m.DismissAndKeepResults()
	next, cmd, result := m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if !result.CursorChanged {
		t.Error("expected CursorChanged=true on n")
	}
	if next.Cursor() != 1 {
		t.Errorf("cursor = %d, want 1", next.Cursor())
	}
	if cmd == nil {
		t.Error("expected non-nil cmd after n in results mode")
	}
}

func TestUpdate_ResultsNavPrev(t *testing.T) {
	m := NewModel()
	m.Start("foo")
	m.SetMatches([]Match{{DataIndex: 1}, {DataIndex: 2}})
	m.SetCursor(1)
	m.DismissAndKeepResults()
	next, _, result := m.Update(tea.KeyPressMsg{Code: 'N', Text: "N"})
	if !result.CursorChanged {
		t.Error("expected CursorChanged=true on N")
	}
	if next.Cursor() != 0 {
		t.Errorf("cursor = %d, want 0", next.Cursor())
	}
}

func TestUpdate_NonKeyMsg(t *testing.T) {
	m := NewModel()
	next, cmd, result := m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	if result.Handled {
		t.Error("non-key msg should not be handled")
	}
	_ = next
	_ = cmd
}

func TestUpdate_ResultsSlashReopensWithQuery(t *testing.T) {
	m := NewModel()
	m.Start("foo")
	m.SetMatches([]Match{{DataIndex: 1}})
	m.DismissAndKeepResults()
	next, _, result := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	if next.Mode() != SearchModeInput {
		t.Errorf("expected SearchModeInput after / in results, got %d", next.Mode())
	}
	if next.Query() != "foo" {
		t.Errorf("query = %q, want 'foo'", next.Query())
	}
	if !result.Handled {
		t.Error("expected Handled=true")
	}
}

func TestView_Open(t *testing.T) {
	m := NewModel()
	m.Start("hello")
	view := m.View()
	if view == "" {
		t.Error("expected non-empty view when active")
	}
}
