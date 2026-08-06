package notifyhistory

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRenderFrame_ClampsLargeScrollValue(t *testing.T) {
	m := New().Open(sampleEntries(), "repo", "wt")
	m.scroll = 1_000_000 // Update leaves this unclamped; renderFrame must clamp it.

	frame := ansi.Strip(m.renderFrame(80, 24))
	if !strings.Contains(frame, "third alert") {
		t.Fatalf("expected clamped render to still show the last entry; got:\n%s", frame)
	}
}

func TestRenderFrame_EmptyEntries(t *testing.T) {
	m := New().Open(nil, "repo", "wt")

	frame := ansi.Strip(m.renderFrame(80, 24))
	if !strings.Contains(frame, "(no notifications)") {
		t.Fatalf("expected placeholder text for empty entries; got:\n%s", frame)
	}
}

func TestRenderFrame_EmptyEntriesWithLargeScroll(t *testing.T) {
	m := New().Open(nil, "repo", "wt")
	m.scroll = 500

	frame := ansi.Strip(m.renderFrame(80, 24))
	if !strings.Contains(frame, "(no notifications)") {
		t.Fatalf("expected placeholder text with clamped scroll on empty entries; got:\n%s", frame)
	}
}
