package sidebar

import (
	"image/color"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestBuildVisibleRenderableRows_Basic(t *testing.T) {
	entries := []string{"a", "b", "c", "d", "e"}
	rows := BuildVisibleRenderableRows(entries, 0, 3, func(i int, e string) RenderableRow {
		return RenderableRow{NameRaw: e}
	})
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	if rows[0].NameRaw != "a" || rows[2].NameRaw != "c" {
		t.Errorf("unexpected rows: %+v", rows)
	}
}

func TestBuildVisibleRenderableRows_Offset(t *testing.T) {
	entries := []string{"a", "b", "c"}
	rows := BuildVisibleRenderableRows(entries, 2, 2, func(i int, e string) RenderableRow {
		return RenderableRow{NameRaw: e}
	})
	if len(rows) != 1 {
		t.Fatalf("expected 1 row (clamped), got %d", len(rows))
	}
	if rows[0].NameRaw != "c" {
		t.Errorf("expected 'c', got %q", rows[0].NameRaw)
	}
}

func TestBuildVisibleRenderableRows_EmptyEntries(t *testing.T) {
	rows := BuildVisibleRenderableRows([]string{}, 0, 5, func(i int, e string) RenderableRow {
		return RenderableRow{}
	})
	if rows != nil {
		t.Errorf("expected nil for empty entries, got %v", rows)
	}
}

func TestBuildVisibleRenderableRows_ZeroHeight(t *testing.T) {
	entries := []string{"a", "b"}
	rows := BuildVisibleRenderableRows(entries, 0, 0, func(i int, e string) RenderableRow {
		return RenderableRow{}
	})
	if rows != nil {
		t.Errorf("expected nil for zero height, got %v", rows)
	}
}

func TestRenderRows_PadsToInnerH(t *testing.T) {
	rows := []RenderableRow{
		{NameRaw: "file.go", Color: ""},
	}
	lines := RenderRows(rows, 5, "", color.White)
	if len(lines) != 5 {
		t.Fatalf("expected 5 lines (padded), got %d", len(lines))
	}
}

func TestRenderRows_EmptyUsesEmptyLine(t *testing.T) {
	lines := RenderRows(nil, 3, "(empty)", color.White)
	if len(lines) == 0 {
		t.Fatal("expected at least one line")
	}
	if ansi.Strip(lines[0]) != "(empty)" {
		t.Errorf("lines[0] = %q, want '(empty)'", ansi.Strip(lines[0]))
	}
}

func TestRenderRows_SelectedRowBold(t *testing.T) {
	rows := []RenderableRow{
		{NameRaw: "selected.go", Selected: true},
	}
	lines := RenderRows(rows, 1, "", color.White)
	if len(lines) != 1 {
		t.Fatal("expected 1 line")
	}
}
