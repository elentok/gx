package sched

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/tickets"
)

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
