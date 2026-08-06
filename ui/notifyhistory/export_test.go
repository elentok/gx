package notifyhistory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui/notify"
)

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		fallback string
		want     string
	}{
		{"already clean", "myrepo", "repo", "myrepo"},
		{"invalid chars replaced", "my repo/name!", "repo", "my-repo-name"},
		{"empty falls back", "", "repo", "repo"},
		{"only invalid chars falls back", "///", "worktree", "worktree"},
		{"whitespace only falls back", "   ", "worktree", "worktree"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeFilename(tt.input, tt.fallback)
			if got != tt.want {
				t.Errorf("sanitizeFilename(%q, %q) = %q, want %q", tt.input, tt.fallback, got, tt.want)
			}
		})
	}
}

func TestWriteExport_FileContentMatchesEntries(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	entries := sampleEntries()
	path, err := writeExport(entries, "myrepo", "mywt")
	if err != nil {
		t.Fatalf("writeExport: %v", err)
	}

	wantDir := filepath.Join(home, ".cache", "gx")
	if filepath.Dir(path) != wantDir {
		t.Fatalf("export dir = %q, want %q", filepath.Dir(path), wantDir)
	}
	if !strings.Contains(filepath.Base(path), "myrepo") || !strings.Contains(filepath.Base(path), "mywt") {
		t.Fatalf("export filename %q missing repo/worktree name", filepath.Base(path))
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)
	for _, e := range entries {
		if !strings.Contains(content, e.Message) {
			t.Errorf("export content missing entry message %q; content=%q", e.Message, content)
		}
	}
}

func TestUpdateKey_WWExportsVisibleEntries(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	m := New().Open(sampleEntries(), "repo", "wt")
	// Filter down to a single entry before exporting, so the export must
	// respect the filtered set, not the full entry list.
	m, _, _ = m.Update(keyMsg('/'))
	m, _, _ = m.Update(keyMsg('t'))
	m, _, _ = m.Update(keyMsg('h'))
	m, _, _ = m.Update(keyMsg('i'))
	m, _, _ = m.Update(keyMsg('r'))
	m, _, _ = m.Update(keyMsg('d'))
	// Dismiss search input mode (keeping the filtered results) so subsequent
	// keys are routed to the ww chord instead of the search text box.
	m, _, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	m, cmd, _ := m.Update(keyMsg('w'))
	if cmd != nil {
		t.Fatal("expected no cmd after first w of the chord")
	}
	m, cmd, _ = m.Update(keyMsg('w'))
	if cmd == nil {
		t.Fatal("expected export cmd after ww")
	}
	if m.pendingW {
		t.Fatal("expected pendingW=false after export chord completes")
	}

	msg := cmd()
	notifyMsg, ok := msg.(notify.NotifyMsg)
	if !ok {
		t.Fatalf("expected notify.NotifyMsg, got %T", msg)
	}
	if notifyMsg.Kind != notify.KindSuccess {
		t.Fatalf("expected KindSuccess, got %v: %s", notifyMsg.Kind, notifyMsg.Message)
	}

	entries, err := os.ReadDir(filepath.Join(home, ".cache", "gx"))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly 1 exported file, got %d", len(entries))
	}

	data, err := os.ReadFile(filepath.Join(home, ".cache", "gx", entries[0].Name()))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "third alert") {
		t.Errorf("export content missing filtered entry; content=%q", content)
	}
	if strings.Contains(content, "first message") || strings.Contains(content, "second message") {
		t.Errorf("export content should only include filtered entries; content=%q", content)
	}
}

func TestExport_ErrorWhenCacheDirCannotBeCreated(t *testing.T) {
	home := t.TempDir()
	// Occupy the .cache path with a regular file so MkdirAll fails.
	if err := os.WriteFile(filepath.Join(home, ".cache"), []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("HOME", home)

	m := New().Open(sampleEntries(), "repo", "wt")
	m, _, _ = m.Update(keyMsg('w'))
	_, cmd, _ := m.Update(keyMsg('w'))
	if cmd == nil {
		t.Fatal("expected error cmd when cache dir cannot be created")
	}
	msg := cmd()
	notifyMsg, ok := msg.(notify.NotifyMsg)
	if !ok {
		t.Fatalf("expected notify.NotifyMsg, got %T", msg)
	}
	if notifyMsg.Kind != notify.KindError {
		t.Fatalf("expected KindError, got %v", notifyMsg.Kind)
	}
}
