package diffview

import (
	"testing"
)

func TestModelInit(t *testing.T) {
	m := NewModel(false)
	if m.Init() != nil {
		t.Error("Init() should return nil")
	}
}

func TestModelSetDataAndViewport(t *testing.T) {
	m := NewModel(false)
	raw := "@@ -1 +1 @@\n-old\n+new\n"
	m.BuildFromRaw(raw, raw)
	data := m.Data()

	m2 := NewModel(false)
	m2.SetData(data)
	if !m2.DataRef().HasContent() {
		t.Error("SetData: expected content after setting data")
	}

	vp := m2.Viewport()
	if vp == nil {
		t.Error("Viewport() should not be nil")
	}
}

func TestModelReflow(t *testing.T) {
	m := NewModel(false)
	raw := "@@ -1 +1 @@\n-old\n+new\n"
	m.BuildFromRaw(raw, raw)
	m.SyncViewport(80, 10)
	m.Reflow(80) // should not panic
}

func TestModelEnsureActiveVisible(t *testing.T) {
	m := NewModel(false)
	raw := "@@ -1 +1 @@\n-old\n+new\n"
	m.BuildFromRaw(raw, raw)
	m.SyncViewport(80, 10)
	m.EnsureActiveVisible(NavModeHunk)
	m.EnsureActiveVisible(NavModeLine)
}

func TestModelComputeSearchMatches(t *testing.T) {
	m := NewModel(false)
	raw := "@@ -1 +1 @@\n-old text\n+new text\n"
	m.BuildFromRaw(raw, raw)

	matches := m.ComputeSearchMatches("text")
	if len(matches) == 0 {
		t.Error("expected search matches for 'text'")
	}

	empty := m.ComputeSearchMatches("")
	if len(empty) != 0 {
		t.Errorf("expected no matches for empty query, got %d", len(empty))
	}
}

func TestModelActiveRawLineIndex(t *testing.T) {
	m := NewModel(false)
	raw := "@@ -1 +1 @@\n-old\n+new\n"
	m.BuildFromRaw(raw, raw)
	m.SyncViewport(80, 10)
	idx := m.ActiveRawLineIndex()
	_ = idx // just verify no panic
}

func TestNearestIndex_FindsClosest(t *testing.T) {
	// displays at 0, 5, 10, 15
	displayAt := func(i int) int { return i * 5 }

	if got := nearestIndex(4, displayAt, 7); got != 1 { // 5 is nearer to 7 than 10
		t.Errorf("expected 1, got %d", got)
	}
	if got := nearestIndex(4, displayAt, 8); got != 2 { // 10 is nearer to 8 than 5... wait 8-5=3, 10-8=2 → index 2
		t.Errorf("expected 2, got %d", got)
	}
	if got := nearestIndex(4, displayAt, 0); got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
	if got := nearestIndex(4, displayAt, 100); got != 3 {
		t.Errorf("expected 3, got %d", got)
	}
}

func TestNearestIndex_EmptySlice(t *testing.T) {
	if got := nearestIndex(0, func(i int) int { return i }, 5); got != 0 {
		t.Errorf("expected 0 for empty, got %d", got)
	}
}

func TestModelBuildFromRawAndHasContent(t *testing.T) {
	m := NewModel(false)
	if m.DataRef().HasContent() {
		t.Fatal("expected empty model to have no content")
	}

	raw := "@@ -1 +1 @@\n-old\n+new\n"
	m.BuildFromRaw(raw, raw)
	if !m.DataRef().HasContent() {
		t.Fatal("expected model to have content")
	}
	if len(m.Data().ViewLines) == 0 {
		t.Fatal("expected view lines")
	}

	m.BuildFromRaw("", "")
	if m.DataRef().HasContent() {
		t.Fatal("expected cleared model to have no content")
	}
}

func TestModelDiffSettings(t *testing.T) {
	m := NewModel(false)
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

	m.EnableWrap(false)
	if m.WrapEnabled() {
		t.Fatal("expected wrap disabled")
	}
}
