package tickets

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
)

func pressKeys(t *testing.T, m tea.Model, keys ...rune) tea.Model {
	t.Helper()
	for _, k := range keys {
		m, _ = m.Update(tea.KeyPressMsg{Code: k, Text: string(k)})
	}
	return m
}

func TestModel_TVHidesPreviewAndGivesListFullWidth(t *testing.T) {
	t.Parallel()
	for _, width := range []int{120, 80} {
		root := t.TempDir()
		writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")

		m := deliverLoad(t, NewModel(root, ui.Settings{}, keys.New(nil)))
		updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
		m = pressKeys(t, updated, 't', 'v').(Model)

		if !m.previewHidden {
			t.Fatalf("width %d: expected \"tv\" to hide the preview", width)
		}
		if _, _, w, h := m.sidebarRect(); w != width || h != m.contentHeight() {
			t.Fatalf("width %d: expected list to fill %dx%d, got %dx%d", width, width, m.contentHeight(), w, h)
		}
		if strings.Contains(m.View().Content, "Preview") {
			t.Fatalf("width %d: expected no preview panel while hidden", width)
		}

		// rows: [section header, epic (selected), ticket] - "l" on the leaf
		// ticket would normally focus the preview.
		m = pressKeys(t, m, 'j', 'l').(Model)
		if m.focus != focusSidebar {
			t.Fatalf("width %d: expected focus to stay on the list while preview hidden, got %v", width, m.focus)
		}
		updated, _ = m.Update(tea.MouseClickMsg{X: width - 2, Y: m.contentHeight() - 2, Button: tea.MouseLeft})
		if updated.(Model).focus != focusSidebar {
			t.Fatalf("width %d: expected a click to never focus the hidden preview", width)
		}

		m = pressKeys(t, m, 't', 'v').(Model)
		if m.previewHidden || !strings.Contains(m.View().Content, "Preview") {
			t.Fatalf("width %d: expected a second \"tv\" to show the preview again", width)
		}
	}
}

func TestPreviewFocus_HidingFocusedPreviewReturnsFocusToList(t *testing.T) {
	t.Parallel()
	p := newPreviewFocus()
	p.focus = focusPreview
	p.togglePreview()
	if p.focus != focusSidebar {
		t.Fatalf("expected focus back on the list, got %v", p.focus)
	}
}

func TestQueueModel_TVHidesPreviewAndGivesListFullWidth(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-solo.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{ticketPath(root, "alpha", "01-solo.md"): true}

	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = pressKeys(t, updated, 't', 'v').(QueueModel)

	if !m.previewHidden {
		t.Fatal("expected \"tv\" to hide the preview")
	}
	if r := m.queueTreeRect(); r.W != 120 {
		t.Fatalf("expected list to fill the full width, got %d", r.W)
	}
	if strings.Contains(m.View().Content, "Preview") {
		t.Fatal("expected no preview panel while hidden")
	}

	m = pressKeys(t, m, 'l').(QueueModel)
	if m.focus != focusSidebar {
		t.Fatalf("expected focus to stay on the list while preview hidden, got %v", m.focus)
	}

	m = pressKeys(t, m, 't', 'v').(QueueModel)
	if m.previewHidden || !strings.Contains(m.View().Content, "Preview") {
		t.Fatal("expected a second \"tv\" to show the preview again")
	}
}

func TestPreviewToggle_ListedInHelp(t *testing.T) {
	t.Parallel()
	for name, km := range map[string]keys.Manager{"tickets": newTicketsManager(), "queue": newQueueKeysManager()} {
		found := false
		for _, b := range km.Bindings() {
			if strings.Join(b.Seq, " ") == "t v" && b.Title != "" {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: expected a titled \"t v\" binding", name)
		}
	}
}
