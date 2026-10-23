package tickets

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/tickets/schema"
)

func TestMigrate_BackfillsClaimedAtAndDoneAtOnce(t *testing.T) {
	dir := t.TempDir()
	issues := filepath.Join(dir, "my-epic", "issues")
	donePath := filepath.Join(issues, "01-done.md")
	openPath := filepath.Join(issues, "02-open.md")
	writeFile(t, donePath, "---\nid: \"01\"\nstatus: done\ntype: implement\ncompleted_at: 2026-01-02T03:04:05Z\n---\nBody.\n")
	writeFile(t, openPath, "---\nid: \"02\"\nstatus: open\ntype: implement\n---\nBody.\n")

	result, err := Migrate(dir)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if len(result.Changes) != 1 || result.Changes[0].Path != donePath {
		t.Fatalf("Changes = %v, want only the done ticket", result.Changes)
	}

	done, err := schema.ParseTicket(donePath)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if !done.DoneAt.Equal(want) || done.ClaimedAt.IsZero() {
		t.Errorf("DoneAt = %v, ClaimedAt = %v, want done_at %v and a claimed_at", done.DoneAt, done.ClaimedAt, want)
	}
	if open, _ := schema.ParseTicket(openPath); !open.ClaimedAt.IsZero() || !open.DoneAt.IsZero() {
		t.Errorf("open ticket got ClaimedAt = %v, DoneAt = %v, want none", open.ClaimedAt, open.DoneAt)
	}

	again, err := Migrate(dir)
	if err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	if len(again.Changes) != 0 {
		t.Errorf("second run Changes = %v, want none", again.Changes)
	}
}

func TestMigrate_IterationStatusRoundTripsUnchanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "my-epic", "issues", "01-first-ticket.md")
	original := "---\nid: \"01\"\nstatus: done\ntype: implement\niteration_status: working\nclaimed_at: 2026-01-02T03:04:05Z\ndone_at: 2026-01-02T03:04:05Z\n---\nBody.\n"
	writeFile(t, path, original)

	result, err := Migrate(dir)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if len(result.Changes) != 0 {
		t.Fatalf("Changes = %v, want none", result.Changes)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if string(raw) != original {
		t.Errorf("file changed by migration: got %q, want %q", string(raw), original)
	}
}

func TestMigrate_RewritesSupersededInTheArchive(t *testing.T) {
	dir := t.TempDir()
	livePath := filepath.Join(dir, "live-epic", "issues", "01-live.md")
	archivedPath := filepath.Join(dir, ".archive", "old-epic", "issues", "01-old.md")
	writeFile(t, livePath, "---\nid: \"01\"\nstatus: superseded\ntype: implement\n---\nBody.\n")
	writeFile(t, archivedPath, "---\nid: \"01\"\nstatus: superseded\ntype: implement\n---\nBody.\n")
	archivedDonePath := filepath.Join(dir, ".archive", "old-epic", "issues", "02-done.md")
	writeFile(t, archivedDonePath, "---\nid: \"02\"\nstatus: done\ntype: implement\n---\nBody.\n")

	result, err := Migrate(dir)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if len(result.Changes) != 2 {
		t.Fatalf("Changes = %v, want the live and the archived ticket", result.Changes)
	}
	for _, path := range []string{livePath, archivedPath} {
		got, err := schema.ParseTicket(path)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != schema.StatusDone || !got.Commitless {
			t.Errorf("%s: Status = %q, Commitless = %v, want done and commitless", path, got.Status, got.Commitless)
		}
	}
	// An archived file's mtime is the day it was rewritten, not the day the
	// work happened, so it never becomes done_at.
	for _, path := range []string{archivedPath, archivedDonePath} {
		got, err := schema.ParseTicket(path)
		if err != nil {
			t.Fatal(err)
		}
		if !got.DoneAt.IsZero() || !got.ClaimedAt.IsZero() {
			t.Errorf("%s: DoneAt = %v, ClaimedAt = %v, want none", path, got.DoneAt, got.ClaimedAt)
		}
	}

	again, err := Migrate(dir)
	if err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	if len(again.Changes) != 0 {
		t.Errorf("second run Changes = %v, want none", again.Changes)
	}
}
