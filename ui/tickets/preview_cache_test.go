package tickets

import (
	"strings"
	"testing"
)

func TestPreviewFocusSync_ReusesContentForSameKeyAndWidth(t *testing.T) {
	p := newPreviewFocus()
	calls := 0
	contentFor := func(width int) (string, int, bool) {
		calls++
		return "hello", 0, false
	}

	p.Sync(40, 5, "a", "key1", contentFor)
	p.Sync(40, 5, "a", "key1", contentFor)
	if calls != 1 {
		t.Fatalf("same key+width: contentFor called %d times, want 1", calls)
	}

	p.Sync(40, 5, "a", "key2", contentFor)
	if calls != 2 {
		t.Fatalf("changed key: contentFor called %d times, want 2", calls)
	}

	p.Sync(30, 5, "a", "key2", contentFor)
	if calls != 3 {
		t.Fatalf("changed width: contentFor called %d times, want 3", calls)
	}
}

func TestPreviewFocusSync_EmptyKeyNeverCaches(t *testing.T) {
	p := newPreviewFocus()
	calls := 0
	contentFor := func(width int) (string, int, bool) {
		calls++
		return "hello", 0, false
	}
	p.Sync(40, 5, "a", "", contentFor)
	p.Sync(40, 5, "a", "", contentFor)
	if calls != 2 {
		t.Fatalf("empty key: contentFor called %d times, want 2", calls)
	}
}

func TestPreviewFocusSync_CachedContentStillShownAfterSelectionChange(t *testing.T) {
	p := newPreviewFocus()
	contentFor := func(width int) (string, int, bool) { return "one\ntwo\nthree", 2, true }

	p.Sync(40, 2, "a", "key1", contentFor)
	p.Sync(40, 2, "b", "key1", contentFor)
	if !strings.Contains(p.previewVP.GetContent(), "three") {
		t.Fatalf("viewport lost its content: %q", p.previewVP.GetContent())
	}
	if p.previewVP.YOffset() == 0 {
		t.Fatal("cached scroll target ignored on selection change")
	}
}

func TestRenderTicketMarkdown_ReusesRendererPerWidth(t *testing.T) {
	a := renderTicketMarkdown("# Hi", 40)
	b := renderTicketMarkdown("# Hi", 40)
	if a != b || !strings.Contains(a, "Hi") {
		t.Fatalf("renders differ: %q vs %q", a, b)
	}
}
