package tickets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/keys"
)

// openQueueTranscript chooses Transcript from a Queue tab menu opened on
// path, and returns what the action's Cmd produced.
func openQueueTranscript(t *testing.T, store, path string) tea.Msg {
	t.Helper()
	m := NewQueueModel(t.TempDir(), ui.Settings{TicketStore: store}, keys.Manager{})
	items := []components.MenuItem{{Label: "Transcript", Value: actionTranscript}}
	m.actionsMenu = m.actionsMenu.Open(path, "alpha", "01", "Suggested actions:", items)
	_, cmd := m.handleQueueActionsMenuKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Transcript returned no Cmd")
	}
	return cmd()
}

func TestQueueActionsMenu_TranscriptResolvesAnAddress(t *testing.T) {
	fakeSessionLookup(t, map[string]string{"s1": "hello from the agent"})
	store := t.TempDir()
	dir := filepath.Join(store, "gx", "alpha", "issues")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := "---\nid: 01\nstatus: done\ntype: implement\nsession_ids:\n  - s1\n---\n\n# 01 — x\n"
	if err := os.WriteFile(filepath.Join(dir, "01-first.md"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	msg, ok := openQueueTranscript(t, store, "gx:alpha/01").(watchLoadedMsg)
	if !ok || !strings.Contains(ansi.Strip(strings.Join(msg.lines, "\n")), "hello from the agent") {
		t.Errorf("msg = %#v, want the transcript of the address's ticket file", msg)
	}
}

func TestQueueActionsMenu_TranscriptAcceptsAFilePath(t *testing.T) {
	fakeSessionLookup(t, map[string]string{"s1": "hello from the agent"})
	path := writeTranscriptTicket(t, "session_ids:\n  - s1\n")

	if _, ok := openQueueTranscript(t, t.TempDir(), path).(watchLoadedMsg); !ok {
		t.Error("a file-shaped Path did not open its transcript")
	}
}

func TestResolveTicketPath(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	dir := filepath.Join(store, "gx", "alpha", "issues")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "01-first.md")
	if err := os.WriteFile(want, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	if file, address := resolveTicketPath(store, "gx:alpha/01"); file != want || address != "gx:alpha/01" {
		t.Errorf("address: got (%q, %q)", file, address)
	}
	if file, address := resolveTicketPath(store, "/some/01-x.md"); file != "/some/01-x.md" || address != "" {
		t.Errorf("file: got (%q, %q)", file, address)
	}
}
