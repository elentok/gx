package diffview

import (
	"testing"

	"github.com/elentok/gx/ui/explorer"
	"github.com/elentok/gx/ui/search"

	tea "charm.land/bubbletea/v2"
)

func TestModelBuildFromRawAndHasContent(t *testing.T) {
	m := NewModel()
	if m.HasContent() {
		t.Fatal("expected empty model to have no content")
	}

	raw := "@@ -1 +1 @@\n-old\n+new\n"
	m.BuildFromRaw(raw, raw)
	if !m.HasContent() {
		t.Fatal("expected model to have content")
	}
	if len(m.Data().ViewLines) == 0 {
		t.Fatal("expected view lines")
	}

	m.BuildFromRaw("", "")
	if m.HasContent() {
		t.Fatal("expected cleared model to have no content")
	}
}

func TestModelDiffSettings(t *testing.T) {
	m := NewModel()
	if m.RenderMode() != RenderModeUnified {
		t.Fatalf("render mode=%v want unified", m.RenderMode())
	}
	if m.NavMode() != NavModeHunk {
		t.Fatalf("nav mode=%v want hunk", m.NavMode())
	}
	if !m.WrapEnabled() {
		t.Fatal("expected wrap enabled by default")
	}

	m.SetRenderMode(RenderModeSideBySide)
	if !m.IsSideBySide() {
		t.Fatal("expected side-by-side mode")
	}

	m.SetNavMode(NavModeLine)
	if m.NavMode() != NavModeLine {
		t.Fatalf("nav mode=%v want line", m.NavMode())
	}
	if m.ExplorerNavMode() != explorer.NavLine {
		t.Fatalf("explorer nav mode=%v want line", m.ExplorerNavMode())
	}

	m.EnableWrap(false)
	if m.WrapEnabled() {
		t.Fatal("expected wrap disabled")
	}
}

func TestModelUpdate_SearchLifecycle(t *testing.T) {
	m := NewModel()

	next, _, handled := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	if !handled {
		t.Fatal("expected / to be handled by search")
	}
	if next.Search().Mode() != search.SearchModeInput {
		t.Fatalf("mode=%v want input", next.Search().Mode())
	}

	next, cmd, handled := next.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	if !handled {
		t.Fatal("expected query key to be handled by search")
	}
	if cmd == nil {
		t.Fatal("expected search batch cmd")
	}

	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("unexpected cmd msg type %T", msg)
	}

	found := false
	for _, batchCmd := range batch {
		if queryMsg, ok := batchCmd().(search.SearchQueryUpdatedMsg); ok {
			if queryMsg.Query != "a" {
				t.Fatalf("query=%q want=a", queryMsg.Query)
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected SearchQueryUpdatedMsg in batch")
	}
}
