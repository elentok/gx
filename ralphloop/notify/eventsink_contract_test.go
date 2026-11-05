package notify

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/elentok/gx/ralphloop/internal/fakes"
	"github.com/elentok/gx/ralphloop/internal/fixture"
)

// TestEpicComplete_ChatSinkIsNotificationOnly pins that EventSink.EpicComplete
// only notifies and never persists state. chatEventSink is the one implementer with enough behavior
// (reading scratchDir to render its message) to plausibly grow a write; this
// snapshots every ticket file's content and mtime around a call and fails if
// either changed — the shape any future non-notification state transition in
// this codebase would take (see StampEpicCompleted/the pause write, the
// existing examples of ticket state persisted to disk).
func TestEpicComplete_ChatSinkIsNotificationOnly(t *testing.T) {
	t.Parallel()
	scratchDir := fixture.WriteEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: done\ntype: implement\n---\n# A\n",
	})
	issuesDir := filepath.Join(scratchDir, "epic", "issues")

	before := snapshotDir(t, issuesDir)
	sink, _ := newFakeChatSink(t, &fakes.RecordingSink{})
	sink.scratchDir = scratchDir
	sink.EpicComplete("epic", 1, 5)
	sink.Close()
	after := snapshotDir(t, issuesDir)

	if !reflect.DeepEqual(before, after) {
		t.Fatalf("issuesDir changed by EpicComplete: before=%#v after=%#v", before, after)
	}
}

// snapshotDir maps every regular file directly under dir to its content and
// mtime, so a caller can assert a call left dir untouched.
func snapshotDir(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", dir, err)
	}
	snapshot := make(map[string]string, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			t.Fatalf("Info(%s): %v", entry.Name(), err)
		}
		content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", entry.Name(), err)
		}
		snapshot[entry.Name()] = info.ModTime().String() + "\n" + string(content)
	}
	return snapshot
}
