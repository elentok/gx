package keybindings_test

import (
	"testing"

	"github.com/elentok/gx/ui/keybindings"
)

const (
	bindingMoveDown     keybindings.BindingID = "move-down"
	bindingGotoTop      keybindings.BindingID = "goto-top"
	bindingGotoLog      keybindings.BindingID = "goto-log"
	bindingYankContent  keybindings.BindingID = "yank-content"
	bindingYankLocation keybindings.BindingID = "yank-location"
)

func testManager() keybindings.Manager {
	return keybindings.New([]keybindings.Binding{
		{ID: bindingMoveDown, Seq: []string{"j"}, Categories: []string{"Navigation"}, Title: "move down"},
		{ID: bindingGotoTop, Seq: []string{"g", "g"}, Categories: []string{"Navigation"}, Title: "go to top"},
		{ID: bindingGotoLog, Seq: []string{"g", "l"}, Categories: []string{"Go to"}, Title: "goto log"},
		{ID: bindingYankContent, Seq: []string{"y", "y"}, Categories: []string{"Yank"}, Title: "yank content"},
		{ID: bindingYankLocation, Seq: []string{"y", "l"}, Categories: []string{"Yank"}, Title: "yank location"},
	})
}

func TestProcess_SingleKey_Match(t *testing.T) {
	m := testManager()
	match, consumed := m.Process("j")
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
	match, consumed := m.Process("g")
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
	m.Process("g")
	match, consumed := m.Process("l")
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
	m.Process("g")
	match, consumed := m.Process("z")
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
	match, consumed := m.Process("x")
	if match != nil {
		t.Fatal("expected no match")
	}
	if consumed {
		t.Fatal("expected consumed=false for unregistered key")
	}
}

func TestChordHints_AfterFirstKey(t *testing.T) {
	m := testManager()
	m.Process("g")
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
	m.Process("y")
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
	m.Process("g")
	if m.Prefix() == nil {
		t.Fatal("expected non-nil prefix after 'g'")
	}
	m.Reset()
	if m.Prefix() != nil {
		t.Fatalf("expected nil prefix after Reset, got %v", m.Prefix())
	}
}

func TestBinding_Keys_Default(t *testing.T) {
	b := keybindings.Binding{Seq: []string{"g", "l"}}
	if b.Keys() != "g/l" {
		t.Fatalf("Keys()=%q want g/l", b.Keys())
	}
}

func TestBinding_Keys_Override(t *testing.T) {
	b := keybindings.Binding{Seq: []string{"up", "k"}, Display: "↑/k"}
	if b.Keys() != "↑/k" {
		t.Fatalf("Keys()=%q want ↑/k", b.Keys())
	}
}
