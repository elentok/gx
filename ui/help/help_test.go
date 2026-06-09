package help

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/ui/keys"
)

func TestHelpNewModel(t *testing.T) {
	m := NewModel(nil)
	if m.IsOpen {
		t.Error("expected IsOpen=false initially")
	}
	if m.Init() != nil {
		t.Error("Init() should return nil")
	}
}

func TestHelpOpenAndView(t *testing.T) {
	m := NewModel([]KeySection{
		{Title: "Navigation", Bindings: []keys.Binding{{Seq: []string{"j"}, Title: "down"}}},
	})
	m.Open(120, 40)
	if !m.IsOpen {
		t.Error("expected IsOpen=true after Open")
	}
	view := m.View()
	if view == "" {
		t.Error("expected non-empty View when open")
	}
}

func TestHelpViewClosedIsEmpty(t *testing.T) {
	m := NewModel(nil)
	if got := m.View(); got != "" {
		t.Errorf("View() on closed help = %q, want empty", got)
	}
}

func TestHelpCloseKeys(t *testing.T) {
	msgs := []tea.KeyPressMsg{
		{Code: tea.KeyEscape},
		{Code: 'q', Text: "q"},
		{Code: '?', Text: "?"},
		{Code: tea.KeyEnter},
	}
	for _, msg := range msgs {
		m := NewModel(nil)
		m.Open(120, 40)
		next, _ := m.Update(msg)
		if next.IsOpen {
			t.Errorf("key %v: expected IsOpen=false after close key", msg)
		}
	}
}

func TestHelpWindowSizeMsg(t *testing.T) {
	m := NewModel(nil)
	m.Open(120, 40)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	_ = next // just ensure no panic
}

func TestBuildSections_DeduplicatesAndSorts(t *testing.T) {
	b1 := keys.Binding{ID: "nav-down", Seq: []string{"j"}, Title: "down", Categories: []string{"Navigation"}}
	b2 := keys.Binding{ID: "nav-up", Seq: []string{"k"}, Title: "up", Categories: []string{"Navigation"}}
	b3 := keys.Binding{ID: "action", Seq: []string{"enter"}, Title: "open", Categories: []string{"Actions"}}

	m1 := keys.New([]keys.Binding{b1, b2})
	m2 := keys.New([]keys.Binding{b3})
	sections := BuildSections(m1, m2)

	if len(sections) != 2 {
		t.Fatalf("expected 2 sections (Actions, Navigation), got %d", len(sections))
	}
	if sections[0].Title != "Actions" {
		t.Errorf("expected 'Actions' first (sorted), got %q", sections[0].Title)
	}
}

func TestBuildSections_MergesTwinsNumberFirst(t *testing.T) {
	// Number key registered before its chord twin (same BindingID) → merged
	// display is number-first: "1/gw".
	num := keys.Binding{ID: "goto-wt", Seq: []string{"1"}, Title: "worktrees tab", Categories: []string{"App"}}
	chord := keys.Binding{ID: "goto-wt", Seq: []string{"g", "w"}, Title: "worktrees tab", Categories: []string{"App"}}
	distinct := keys.Binding{ID: "next-tab", Seq: []string{"g", "."}, Title: "next tab", Categories: []string{"App"}}

	sections := BuildSections(keys.New([]keys.Binding{num, chord, distinct}))

	if len(sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(sections))
	}
	bs := sections[0].Bindings
	if len(bs) != 2 {
		t.Fatalf("expected 2 merged bindings, got %d: %+v", len(bs), bs)
	}
	if got := bs[0].Keys(); got != "1/gw" {
		t.Errorf("merged twin Keys()=%q want 1/gw", got)
	}
	if got := bs[1].Keys(); got != "g." {
		t.Errorf("distinct binding Keys()=%q want g.", got)
	}
}

func TestRenderView_ContainsBindings(t *testing.T) {
	sections := []KeySection{
		{Title: "Nav", Bindings: []keys.Binding{{Seq: []string{"j"}, Title: "down"}}},
	}
	out := RenderView(sections)
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "Nav") || !strings.Contains(plain, "down") {
		t.Errorf("RenderView missing expected content: %q", plain)
	}
}

func TestNewKeySection(t *testing.T) {
	b := keys.Binding{Seq: []string{"j"}, Title: "down"}
	s := NewKeySection("Navigation", b)
	if s.Title != "Navigation" || len(s.Bindings) != 1 {
		t.Errorf("NewKeySection = %+v", s)
	}
}
