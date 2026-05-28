package diffarea

import (
	"testing"

	"github.com/elentok/gx/ui/diffview"
)

func TestNewModel(t *testing.T) {
	d := NewModel(false)
	if d.ActiveSection != SectionUnstaged {
		t.Errorf("expected SectionUnstaged, got %d", d.ActiveSection)
	}
	if d.RenderMode() != diffview.RenderModeUnified {
		t.Errorf("expected RenderModeUnified, got %v", d.RenderMode())
	}
	if d.NavMode() != diffview.NavModeHunk {
		t.Errorf("expected NavModeHunk, got %v", d.NavMode())
	}
	if !d.Wrap() {
		t.Error("expected Wrap=true by default")
	}
}

func TestSetRenderMode(t *testing.T) {
	d := NewModel(false)
	d.SetRenderMode(diffview.RenderModeSideBySide)
	if d.RenderMode() != diffview.RenderModeSideBySide {
		t.Errorf("RenderMode = %v, want SideBySide", d.RenderMode())
	}
	if d.Unstaged.RenderMode() != diffview.RenderModeSideBySide {
		t.Error("expected Unstaged to use SideBySide mode")
	}
}

func TestSetNavMode(t *testing.T) {
	d := NewModel(false)
	d.SetNavMode(diffview.NavModeLine)
	if d.NavMode() != diffview.NavModeLine {
		t.Errorf("NavMode = %v, want Line", d.NavMode())
	}
}

func TestToggleNavMode(t *testing.T) {
	d := NewModel(false)
	d.ToggleNavMode()
	if d.NavMode() != diffview.NavModeLine {
		t.Errorf("after toggle: NavMode = %v, want Line", d.NavMode())
	}
	d.ToggleNavMode()
	if d.NavMode() != diffview.NavModeHunk {
		t.Errorf("after second toggle: NavMode = %v, want Hunk", d.NavMode())
	}
}

func TestSetWrap(t *testing.T) {
	d := NewModel(false)
	d.SetWrap(false)
	if d.Wrap() {
		t.Error("expected Wrap=false after SetWrap(false)")
	}
}

func TestToggleSection(t *testing.T) {
	d := NewModel(false)
	d.ToggleSection()
	if d.ActiveSection != SectionStaged {
		t.Errorf("expected SectionStaged after toggle, got %d", d.ActiveSection)
	}
	d.ToggleSection()
	if d.ActiveSection != SectionUnstaged {
		t.Errorf("expected SectionUnstaged after second toggle, got %d", d.ActiveSection)
	}
}

func TestSectionModel(t *testing.T) {
	d := NewModel(false)
	unstaged := d.SectionModel(SectionUnstaged)
	staged := d.SectionModel(SectionStaged)
	if unstaged == staged {
		t.Error("expected different models for unstaged and staged")
	}
}

func TestActiveSectionModel(t *testing.T) {
	d := NewModel(false)
	active := d.ActiveSectionModel()
	if active == nil {
		t.Error("expected non-nil ActiveSectionModel")
	}
}

func TestResetSections(t *testing.T) {
	d := NewModel(false)
	raw := "@@ -1 +1 @@\n-old\n+new\n"
	d.Unstaged.BuildFromRaw(raw, raw)
	d.ResetSections()
	if d.Unstaged.DataRef().HasContent() {
		t.Error("expected Unstaged to be cleared after ResetSections")
	}
}

func TestSyncViewports(t *testing.T) {
	d := NewModel(false)
	d.SyncViewports(80, 20, 10) // should not panic
}

func TestDisableVisual(t *testing.T) {
	d := NewModel(false)
	d.DisableVisual() // should not panic
}

func TestKeys(t *testing.T) {
	d := NewModel(false)
	if d.Keys() == nil {
		t.Error("Keys() should not be nil")
	}
}

func TestUpdateActive_Unhandled(t *testing.T) {
	d := NewModel(false)
	cmd, result := d.UpdateActive(struct{}{})
	if result.Handled {
		t.Error("expected unhandled for unknown msg")
	}
	_ = cmd
}
