package keys_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui/keys"
)

const (
	bindingMoveDown     keys.BindingID = "move-down"
	bindingGotoTop      keys.BindingID = "goto-top"
	bindingGotoLog      keys.BindingID = "goto-log"
	bindingYankContent  keys.BindingID = "yank-content"
	bindingYankLocation keys.BindingID = "yank-location"
)

func testManager() keys.Manager {
	return keys.New([]keys.Binding{
		{ID: bindingMoveDown, Seq: []string{"j"}, Categories: []string{"Navigation"}, Title: "move down"},
		{ID: bindingGotoTop, Seq: []string{"g", "g"}, Categories: []string{"Navigation"}, Title: "go to top"},
		{ID: bindingGotoLog, Seq: []string{"g", "l"}, Categories: []string{"Go to"}, Title: "goto log"},
		{ID: bindingYankContent, Seq: []string{"y", "y"}, Categories: []string{"Yank"}, Title: "yank content"},
		{ID: bindingYankLocation, Seq: []string{"y", "l"}, Categories: []string{"Yank"}, Title: "yank location"},
	})
}

func press(text string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Text: text}
}

func TestProcess_SingleKey_Match(t *testing.T) {
	m := testManager()
	match, consumed := m.Process(press("j"))
	if match == nil {
		t.Fatal("expected a match")
	}
	if !consumed {
		t.Fatal("expected consumed=true")
	}
	if match.ID != bindingMoveDown {
		t.Fatalf("match.ID=%q want move-down", match.ID)
	}
	if m.Prefix() != nil {
		t.Fatalf("expected prefix to be nil, got %v", m.Prefix())
	}
}

func TestProcess_Chord_FirstKey_Consumed(t *testing.T) {
	m := testManager()
	match, consumed := m.Process(press("g"))
	if match != nil {
		t.Fatal("expected no match for first chord key")
	}
	if !consumed {
		t.Fatal("expected consumed=true for chord prefix")
	}
	if len(m.Prefix()) != 1 || m.Prefix()[0] != "g" {
		t.Fatalf("expected prefix=[g], got %v", m.Prefix())
	}
}

func TestProcess_Chord_SecondKey_Match(t *testing.T) {
	m := testManager()
	m.Process(press("g"))
	match, consumed := m.Process(press("l"))
	if match == nil {
		t.Fatal("expected a match after full chord")
	}
	if !consumed {
		t.Fatal("expected consumed=true")
	}
	if match.ID != bindingGotoLog {
		t.Fatalf("match.ID=%q want goto-log", match.ID)
	}
	if m.Prefix() != nil {
		t.Fatalf("expected prefix cleared, got %v", m.Prefix())
	}
}

func TestProcess_Chord_Cancellation(t *testing.T) {
	m := testManager()
	m.Process(press("g"))
	match, consumed := m.Process(press("z"))
	if match != nil {
		t.Fatal("expected no match for unrecognized chord completion")
	}
	if consumed {
		t.Fatal("expected consumed=false on chord cancellation")
	}
	if m.Prefix() != nil {
		t.Fatalf("expected prefix cleared after cancellation, got %v", m.Prefix())
	}
}

func TestProcess_Unregistered_Key(t *testing.T) {
	m := testManager()
	match, consumed := m.Process(press("x"))
	if match != nil {
		t.Fatal("expected no match")
	}
	if consumed {
		t.Fatal("expected consumed=false for unregistered key")
	}
}

func TestProcess_ShiftModifier_Normalization(t *testing.T) {
	m := keys.New([]keys.Binding{
		{ID: "goto-bottom", Seq: []string{"G"}, Title: "go to bottom"},
	})
	// Some terminals send lowercase 'g' with ModShift instead of 'G'.
	msg := tea.KeyPressMsg{Text: "g", Mod: tea.ModShift}
	match, consumed := m.Process(msg)
	if match == nil {
		t.Fatal("expected shift+g to match 'G' binding")
	}
	if !consumed {
		t.Fatal("expected consumed=true")
	}
}

func TestChordHints_AfterFirstKey(t *testing.T) {
	m := testManager()
	m.Process(press("g"))
	hints := m.ChordHints()
	if len(hints) != 2 {
		t.Fatalf("expected 2 hints after 'g', got %d: %v", len(hints), hints)
	}
	keys := map[string]bool{hints[0].Key: true, hints[1].Key: true}
	if !keys["g"] || !keys["l"] {
		t.Fatalf("expected hints for g and l, got %v", hints)
	}
}

func TestChordHints_AfterYankPrefix(t *testing.T) {
	m := testManager()
	m.Process(press("y"))
	hints := m.ChordHints()
	if len(hints) != 2 {
		t.Fatalf("expected 2 hints after 'y', got %d: %v", len(hints), hints)
	}
	keys := map[string]bool{hints[0].Key: true, hints[1].Key: true}
	if !keys["y"] || !keys["l"] {
		t.Fatalf("expected hints for y and l, got %v", hints)
	}
}

func TestChordHints_NoPrefix(t *testing.T) {
	m := testManager()
	if m.ChordHints() != nil {
		t.Fatal("expected nil ChordHints with no prefix")
	}
}

func TestHintsForPrefix_WithNoInternalPrefix(t *testing.T) {
	m := testManager()
	hints := m.HintsForPrefix("g")
	if len(hints) != 2 {
		t.Fatalf("expected 2 hints for prefix 'g', got %d: %v", len(hints), hints)
	}
	keys := map[string]bool{hints[0].Key: true, hints[1].Key: true}
	if !keys["g"] || !keys["l"] {
		t.Fatalf("expected hints for g and l, got %v", hints)
	}
}

func TestBindings_ReturnsAll(t *testing.T) {
	m := testManager()
	all := m.Bindings()
	if len(all) != 5 {
		t.Fatalf("expected 5 bindings, got %d", len(all))
	}
	if all[0].ID != bindingMoveDown {
		t.Fatalf("all[0].ID=%q want move-down", all[0].ID)
	}
	if all[1].ID != bindingGotoTop {
		t.Fatalf("all[1].ID=%q want goto-top", all[1].ID)
	}
}

func TestReset_ClearsPrefix(t *testing.T) {
	m := testManager()
	m.Process(press("g"))
	if m.Prefix() == nil {
		t.Fatal("expected non-nil prefix after 'g'")
	}
	m.Reset()
	if m.Prefix() != nil {
		t.Fatalf("expected nil prefix after Reset, got %v", m.Prefix())
	}
}

func TestBinding_Keys_Default(t *testing.T) {
	b := keys.Binding{Seq: []string{"g", "l"}}
	if b.Keys() != "gl" {
		t.Fatalf("Keys()=%q want gl", b.Keys())
	}
}

func TestBinding_Keys_SingleKey(t *testing.T) {
	b := keys.Binding{Seq: []string{"j"}}
	if b.Keys() != "j" {
		t.Fatalf("Keys()=%q want j", b.Keys())
	}
}

func TestBinding_Keys_Override(t *testing.T) {
	b := keys.Binding{Seq: []string{"up", "k"}, Display: "↑/k"}
	if b.Keys() != "↑/k" {
		t.Fatalf("Keys()=%q want ↑/k", b.Keys())
	}
}
