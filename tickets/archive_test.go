package tickets

import (
	"path/filepath"
	"testing"
)

func TestCountArchivedEpics_MissingArchiveDirReturnsZero(t *testing.T) {
	count, err := CountArchivedEpics(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0, got %d", count)
	}
}

func TestCountArchivedEpics_EmptyArchiveDirReturnsZero(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(ArchiveDir(dir), ".keep"), "")

	count, err := CountArchivedEpics(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0, got %d", count)
	}
}

func TestCountArchivedEpics_CountsEpicDirectories(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(ArchiveDir(dir), "epic-one", "issues", "01-first.md"),
		"---\nid: \"01\"\nstatus: done\ntype: task\n---\nBody.\n")
	writeFile(t, filepath.Join(ArchiveDir(dir), "epic-two", "issues", "01-first.md"),
		"---\nid: \"01\"\nstatus: done\ntype: task\n---\nBody.\n")

	count, err := CountArchivedEpics(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2, got %d", count)
	}
}

func TestLoadArchived_MissingArchiveDirReturnsEmpty(t *testing.T) {
	epics, err := LoadArchived(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(epics) != 0 {
		t.Fatalf("expected no epics, got %v", epics)
	}
}

func TestLoadArchived_EmptyArchiveDirReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(ArchiveDir(dir), ".keep"), "")

	epics, err := LoadArchived(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(epics) != 0 {
		t.Fatalf("expected no epics, got %v", epics)
	}
}

func TestLoadArchived_DiscoversEpicsAndTicketsMatchingCount(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(ArchiveDir(dir), "my-epic", "issues", "01-first-ticket.md"),
		"---\nid: \"01\"\nstatus: done\ntype: task\n---\nBody.\n")
	writeFile(t, filepath.Join(ArchiveDir(dir), "my-epic", "issues", "02-second-ticket.md"),
		"---\nid: \"02\"\nstatus: open\ntype: task\n---\nBody.\n")

	epics, err := LoadArchived(dir)
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
	if epic.TotalCount() != 2 {
		t.Fatalf("expected 2 tickets, got %d", epic.TotalCount())
	}

	count, err := CountArchivedEpics(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != len(epics) {
		t.Fatalf("CountArchivedEpics = %d, want %d (LoadArchived epic count)", count, len(epics))
	}
}
