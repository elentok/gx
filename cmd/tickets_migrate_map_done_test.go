package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

func mapDoneStore(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	epics := map[string]string{
		"merged":        "kind: map\n",
		"merged-impl":   "kind: loop\nmerged_at: 2026-03-04T05:06:07Z\n",
		"unmerged":      "kind: map\n",
		"unmerged-impl": "kind: loop\n",
		"noimpl":        "kind: map\n",
	}
	for name, fm := range epics {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
		testutil.WriteFile(t, filepath.Join(root, name), "ticket.md", "---\nstatus: open\n"+fm+"---\n")
	}
	return root
}

func TestExecute_TicketsMigrate_MarksMapDoneWhenImplMerged(t *testing.T) {
	t.Parallel()
	root := mapDoneStore(t)

	run := func(args ...string) string {
		t.Helper()
		var stdout bytes.Buffer
		if err := execute(append([]string{"tickets", "migrate", root}, args...), deps{stdout: &stdout, stderr: &bytes.Buffer{}}); err != nil {
			t.Fatalf("execute tickets migrate: %v", err)
		}
		return stdout.String()
	}
	load := func(name string) tickets.Epic { return tickets.LoadEpicTicketMD(filepath.Join(root, name)) }

	if out := run("--dry-run"); !strings.Contains(out, filepath.Join(root, "merged", "ticket.md")) {
		t.Errorf("dry run output = %q, want the merged map listed", out)
	}
	if got := load("merged"); got.Status != "open" || !got.MergedAt.IsZero() {
		t.Errorf("dry run wrote: status %q, merged_at %v", got.Status, got.MergedAt)
	}

	run()
	want := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	if got := load("merged"); got.Status != "done" || !got.MergedAt.Equal(want) {
		t.Errorf("merged map: status %q, merged_at %v, want done, %v", got.Status, got.MergedAt, want)
	}
	for _, name := range []string{"unmerged", "noimpl"} {
		if got := load(name); got.Status != "open" || !got.MergedAt.IsZero() {
			t.Errorf("%s: status %q, merged_at %v, want untouched", name, got.Status, got.MergedAt)
		}
	}

	before := load("merged")
	run()
	if after := load("merged"); after.Status != before.Status || !after.MergedAt.Equal(before.MergedAt) {
		t.Errorf("second run changed the map: %+v -> %+v", before, after)
	}
}
