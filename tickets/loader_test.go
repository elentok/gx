package tickets

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestLoad_MissingScratchDirReturnsEmpty(t *testing.T) {
	epics, err := Load(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(epics) != 0 {
		t.Fatalf("expected no epics, got %v", epics)
	}
}

func TestLoad_EmptyScratchDirReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	epics, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(epics) != 0 {
		t.Fatalf("expected no epics, got %v", epics)
	}
}

func TestLoad_DiscoversEpicsAndTickets(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "my-epic", "issues", "01-first-ticket.md"), "Status: done\n\nBody.\n")
	writeFile(t, filepath.Join(dir, "my-epic", "issues", "02-second-ticket.md"), "Type: task\n\nBody.\n")

	epics, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(epics) != 1 {
		t.Fatalf("expected 1 epic, got %d", len(epics))
	}

	epic := epics[0]
	if epic.Name != "my-epic" {
		t.Errorf("Name = %q, want %q", epic.Name, "my-epic")
	}
	if epic.IsMap {
		t.Error("epic without map.md should not be IsMap")
	}
	if epic.TotalCount() != 2 {
		t.Fatalf("expected 2 tickets, got %d", epic.TotalCount())
	}
	if epic.OpenCount() != 1 {
		t.Errorf("OpenCount = %d, want 1 (one done, one open)", epic.OpenCount())
	}

	byNumber := map[int]Ticket{}
	for _, tk := range epic.Tickets {
		byNumber[tk.Number] = tk
	}
	if byNumber[1].Title != "First ticket" {
		t.Errorf("ticket 1 Title = %q, want %q", byNumber[1].Title, "First ticket")
	}
	if byNumber[2].Title != "Second ticket" {
		t.Errorf("ticket 2 Title = %q, want %q", byNumber[2].Title, "Second ticket")
	}
}

func TestLoad_EpicWithMapMdIsFlagged(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "wayfinder-epic", "map.md"), "# Map\n")

	epics, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(epics) != 1 || !epics[0].IsMap {
		t.Fatalf("expected 1 IsMap epic, got %+v", epics)
	}
	if epics[0].TotalCount() != 0 {
		t.Errorf("expected 0 tickets for map-only epic, got %d", epics[0].TotalCount())
	}
}

func TestLoad_EpicWithNoIssuesDirHasZeroTickets(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "bare-epic"), 0755); err != nil {
		t.Fatal(err)
	}

	epics, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(epics) != 1 || epics[0].TotalCount() != 0 {
		t.Fatalf("expected 1 zero-ticket epic, got %+v", epics)
	}
}

func TestLoad_UnreadableTicketFileShowsErrorRow(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: unreadable-file permissions aren't enforced")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "epic", "issues", "01-broken.md")
	writeFile(t, path, "Status: open\n\nBody.\n")
	if err := os.Chmod(path, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0644) })

	epics, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(epics) != 1 || epics[0].TotalCount() != 1 {
		t.Fatalf("expected 1 epic with 1 ticket, got %+v", epics)
	}

	tk := epics[0].Tickets[0]
	if tk.Number != 1 {
		t.Errorf("Number = %d, want 1", tk.Number)
	}
	if tk.Title != "Broken" {
		t.Errorf("Title = %q, want %q", tk.Title, "Broken")
	}
	if tk.ReadErr == "" {
		t.Error("expected ReadErr to be set for an unreadable ticket file")
	}
}

func TestLoad_IgnoresNonTicketFilesInIssuesDir(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "epic", "issues", "01-a-ticket.md"), "Status: open\n")
	writeFile(t, filepath.Join(dir, "epic", "issues", "README.md"), "not a ticket\n")

	epics, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if epics[0].TotalCount() != 1 {
		t.Fatalf("expected 1 ticket (README.md ignored), got %d", epics[0].TotalCount())
	}
}
