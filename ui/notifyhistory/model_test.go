package notifyhistory

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/notifylog"
)

func sampleEntries() []notifylog.Entry {
	base := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	return []notifylog.Entry{
		{Time: base, Kind: notify.KindInfo, Message: "first message"},
		{Time: base.Add(time.Minute), Kind: notify.KindSuccess, Message: "second message"},
		{Time: base.Add(2 * time.Minute), Kind: notify.KindError, Message: "third alert"},
	}
}

func TestOpen_SetsFieldsAndResetsState(t *testing.T) {
	m := New()
	entries := sampleEntries()
	m = m.Open(entries, "myrepo", "mywt")

	if !m.IsOpen {
		t.Fatal("expected IsOpen=true after Open")
	}
	if len(m.visibleEntries()) != len(entries) {
		t.Fatalf("visibleEntries len = %d, want %d", len(m.visibleEntries()), len(entries))
	}
	if m.repoName != "myrepo" || m.worktreeName != "mywt" {
		t.Fatalf("repoName/worktreeName = %q/%q, want myrepo/mywt", m.repoName, m.worktreeName)
	}
	if m.scroll != 0 {
		t.Fatalf("scroll = %d, want 0", m.scroll)
	}
	if m.pendingW {
		t.Fatal("expected pendingW=false after Open")
	}
	if m.search.HasQuery() {
		t.Fatal("expected fresh search state after Open")
	}
}

func TestOpen_ResetsStaleStateFromPriorSession(t *testing.T) {
	m := New()
	m = m.Open(sampleEntries(), "repo", "wt")
	m.scroll = 5
	m.pendingW = true
	m.search.Start("stale")

	m = m.Open(sampleEntries(), "repo2", "wt2")
	if m.scroll != 0 {
		t.Fatalf("scroll = %d, want reset to 0", m.scroll)
	}
	if m.pendingW {
		t.Fatal("expected pendingW reset to false")
	}
	if m.search.HasQuery() {
		t.Fatal("expected search query reset")
	}
}

func TestClose(t *testing.T) {
	m := New().Open(sampleEntries(), "repo", "wt")
	m = m.Close()
	if m.IsOpen {
		t.Fatal("expected IsOpen=false after Close")
	}
}

func keyMsg(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

func TestUpdateKey_EscCloses(t *testing.T) {
	m := New().Open(sampleEntries(), "repo", "wt")
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if next.IsOpen {
		t.Fatal("expected IsOpen=false after esc")
	}
}

func TestUpdateKey_QCloses(t *testing.T) {
	m := New().Open(sampleEntries(), "repo", "wt")
	next, _ := m.Update(keyMsg('q'))
	if next.IsOpen {
		t.Fatal("expected IsOpen=false after q")
	}
}

func TestUpdateKey_SlashStartsSearchInput(t *testing.T) {
	m := New().Open(sampleEntries(), "repo", "wt")
	next, _ := m.Update(keyMsg('/'))
	if !next.IsOpen {
		t.Fatal("expected IsOpen=true on /")
	}
	if !next.search.InputFocused() {
		t.Fatal("expected search input to be focused after /")
	}
}

func TestUpdateKey_SearchFiltersVisibleEntries(t *testing.T) {
	m := New().Open(sampleEntries(), "repo", "wt")
	m, _ = m.Update(keyMsg('/'))
	m, _ = m.Update(keyMsg('t'))
	m, _ = m.Update(keyMsg('h'))
	m, _ = m.Update(keyMsg('i'))
	m, _ = m.Update(keyMsg('r'))
	m, _ = m.Update(keyMsg('d'))

	visible := m.visibleEntries()
	if len(visible) != 1 {
		t.Fatalf("visibleEntries len = %d, want 1 (got %+v)", len(visible), visible)
	}
	if visible[0].Message != "third alert" {
		t.Fatalf("visible[0].Message = %q, want %q", visible[0].Message, "third alert")
	}
}

func TestUpdateKey_SearchQueryChangeResetsScroll(t *testing.T) {
	m := New().Open(sampleEntries(), "repo", "wt")
	m.scroll = 2
	m, _ = m.Update(keyMsg('/'))
	m, _ = m.Update(keyMsg('t'))
	if m.scroll != 0 {
		t.Fatalf("scroll = %d, want reset to 0 on query change", m.scroll)
	}
}

func TestVisibleEntries_NoQueryReturnsAll(t *testing.T) {
	entries := sampleEntries()
	m := New().Open(entries, "repo", "wt")
	visible := m.visibleEntries()
	if len(visible) != len(entries) {
		t.Fatalf("visibleEntries len = %d, want %d", len(visible), len(entries))
	}
}

func TestUpdateKey_PendingWResetsOnUnrelatedKey(t *testing.T) {
	m := New().Open(sampleEntries(), "repo", "wt")
	m, _ = m.Update(keyMsg('w'))
	if !m.pendingW {
		t.Fatal("expected pendingW=true after first w")
	}
	m, cmd := m.Update(keyMsg('x'))
	if m.pendingW {
		t.Fatal("expected pendingW=false after unrelated key")
	}
	if cmd != nil {
		t.Fatal("expected no export cmd for w followed by unrelated key")
	}
	if !m.IsOpen {
		t.Fatal("expected IsOpen=true after unrelated key")
	}
}

func TestClampScroll(t *testing.T) {
	tests := []struct {
		name    string
		offset  int
		total   int
		visible int
		want    int
	}{
		{"within bounds", 2, 10, 5, 2},
		{"huge offset clamps to max", 1000, 10, 5, 5},
		{"negative offset clamps to zero", -3, 10, 5, 0},
		{"empty entries clamps to zero", 5, 0, 5, 0},
		{"visible exceeds total clamps to zero", 3, 2, 5, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clampScroll(tt.offset, tt.total, tt.visible)
			if got != tt.want {
				t.Errorf("clampScroll(%d, %d, %d) = %d, want %d", tt.offset, tt.total, tt.visible, got, tt.want)
			}
		})
	}
}
