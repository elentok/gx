package sched

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// A trunk-conflict launch stores its fixed note in the ticket store, for Claude
// (read through a prompt file) and for Codex (named in its developer instructions).
func TestApplyLaunchNote_TrunkConflictNoteIsWrittenToTheStore(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	if err := os.MkdirAll(filepath.Join(store, "proj"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, "proj", "project.json"), []byte(`{"name":"proj"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	a := tickets.Address{Project: "proj", Epic: "e", ID: "03"}
	note := launchNote(tickets.Ticket{Type: string(schema.TypeTrunkConflict)}, t.TempDir())
	if note != trunkConflictNote {
		t.Fatalf("note = %q, want the trunk-conflict note", note)
	}
	for _, codex := range []bool{false, true} {
		l := NewLauncher(LauncherDeps{Projects: projects.NewRegistry(
			func() projects.Env { return projects.Env{TicketStore: store} },
			slog.New(slog.NewTextHandler(io.Discard, nil)), nil, &settings.Holder{}, nil, nil)})
		_, noteFile, err := l.applyLaunchNote(a, note, codex, nil)
		if err != nil {
			t.Fatal(err)
		}
		files := launchNoteFiles(filepath.Join(store, "proj"), a)
		got, err := os.ReadFile(files.note)
		if err != nil || string(got) != trunkConflictNote+"\n" {
			t.Errorf("codex=%v: note file = %q (%v)", codex, got, err)
		}
		if codex && noteFile != files.note {
			t.Errorf("codex note file = %q, want %q", noteFile, files.note)
		}
	}
}

func TestLaunchNoteFiles_LiveInTheStoreBesideTheEpic(t *testing.T) {
	t.Parallel()
	got := launchNoteFiles("/store/proj", tickets.Address{Project: "proj", Epic: "e", ID: "03"})
	if got.note != "/store/proj/e/launch/03-note.md" || got.prompt != "/store/proj/e/launch/03-prompt.md" {
		t.Errorf("files = %+v", got)
	}
}

func TestNoteFilesRemove_DropsBothAndToleratesMissing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	f := noteFiles{note: filepath.Join(dir, "n.md"), prompt: filepath.Join(dir, "p.md")}
	if err := os.WriteFile(f.note, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.remove()
	f.remove()
	for _, p := range []string{f.note, f.prompt} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s still there: %v", p, err)
		}
	}
}
