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

func TestExecute_TicketsMigrate_BackfillsMergedAtForLandedLoopEpics(t *testing.T) {
	t.Parallel()
	repoDir, _ := mergeTestRepo(t, map[string]string{"unlanded": "unlanded"})
	commitFile(t, filepath.Join(repoDir, "unlanded"), "unlanded.txt", "hi")
	testutil.MustGitExported(t, repoDir, "branch", "landed", "main")

	root := t.TempDir()
	testutil.WriteFile(t, root, "project.json", `{"repo": "`+repoDir+`", "trunk": "main"}`)
	const completed = "completed_at: 2026-01-02T03:04:05Z\n"
	epics := map[string]string{
		"landed":     completed,
		"gone":       completed,
		"unlanded":   completed,
		"mapped":     "kind: map\n" + completed,
		"incomplete": "",
		"already":    completed + "merged_at: 2026-05-06T07:08:09Z\n",
	}
	for name, fm := range epics {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
		testutil.WriteFile(t, filepath.Join(root, name), "ticket.md", "---\nstatus: open\n"+fm+"---\n")
	}

	var stdout bytes.Buffer
	if err := execute([]string{"tickets", "migrate", root}, deps{stdout: &stdout, stderr: &bytes.Buffer{}}); err != nil {
		t.Fatalf("execute tickets migrate: %v", err)
	}

	completedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	wantMerged := map[string]time.Time{
		"landed":  completedAt,
		"gone":    completedAt,
		"already": time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC),
	}
	for name := range epics {
		got := tickets.LoadEpicTicketMD(filepath.Join(root, name)).MergedAt
		if !got.Equal(wantMerged[name]) {
			t.Errorf("%s merged_at = %v, want %v", name, got, wantMerged[name])
		}
	}
	out := stdout.String()
	for _, name := range []string{"landed", "gone"} {
		if !strings.Contains(out, filepath.Join(root, name, "ticket.md")+": merged_at backfilled") {
			t.Errorf("stdout = %q, want a backfill line for %s", out, name)
		}
	}

	before := map[string][]byte{}
	for name := range epics {
		before[name], _ = os.ReadFile(filepath.Join(root, name, "ticket.md"))
	}
	var second bytes.Buffer
	if err := execute([]string{"tickets", "migrate", root}, deps{stdout: &second, stderr: &bytes.Buffer{}}); err != nil {
		t.Fatalf("second execute tickets migrate: %v", err)
	}
	if strings.TrimSpace(second.String()) != "no changes" {
		t.Errorf("second run stdout = %q, want %q", second.String(), "no changes")
	}
	for name, b := range before {
		after, _ := os.ReadFile(filepath.Join(root, name, "ticket.md"))
		if !bytes.Equal(after, b) {
			t.Errorf("%s ticket.md changed on second run: %q -> %q", name, b, after)
		}
	}
}
