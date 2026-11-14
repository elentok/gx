package migrations

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

func gitRev(t *testing.T, dir, ref string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", ref)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse %s: %v", ref, err)
	}
	return strings.TrimSpace(string(out))
}

// mergeEpics fast-forwards main over one branch holding a trailer commit for
// each named epic and returns the reflog entry's old and new tip.
func mergeEpics(t *testing.T, repo, branch string, epics ...string) (old, tip string) {
	t.Helper()
	testutil.MustGitExported(t, repo, "checkout", "-b", branch, "main")
	for _, e := range epics {
		testutil.WriteFile(t, repo, e+".txt", e)
		testutil.CommitAll(t, repo, "work\n\nRalph-Loop-Ticket: "+e+"/01")
	}
	testutil.MustGitExported(t, repo, "checkout", "main")
	old = gitRev(t, repo, "main")
	testutil.MustGitExported(t, repo, "merge", "--ff-only", branch)
	return old, gitRev(t, repo, "main")
}

func writeRangeEpic(t *testing.T, dir, name, frontmatter string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nkind: loop\nstatus: open\ncreated_at: 2026-01-02T03:04:05Z\nmerged_at: 2026-02-02T03:04:05Z\n" + frontmatter + "---\n# E\n"
	if err := os.WriteFile(filepath.Join(p, "ticket.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(p, "ticket.md")
}

func logEvent(t *testing.T, logRoot, epic string, typ events.Type) {
	t.Helper()
	if err := runlog.AppendEvent(logRoot, epic, runlog.Event{Type: string(typ), Ticket: "01"}); err != nil {
		t.Fatal(err)
	}
}

func rangeProject(t *testing.T, repo string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "proj")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	pj := fmt.Sprintf(`{"repo": %q, "trunk": "main"}`, repo)
	if err := os.WriteFile(filepath.Join(root, "project.json"), []byte(pj), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func rangeOf(t *testing.T, ticketMD string) tickets.Epic {
	t.Helper()
	return tickets.LoadEpicTicketMD(filepath.Dir(ticketMD))
}

func TestDefault_EpicRange(t *testing.T) {
	repo := testutil.TempRepo(t)
	exactOld, exactTip := mergeEpics(t, repo, "b-exact", "exact")
	archOld, archTip := mergeEpics(t, repo, "b-arch", "arch")
	mergeEpics(t, repo, "b-shared", "shareda", "sharedb")
	trunkTip := gitRev(t, repo, "main")

	root := rangeProject(t, repo)
	exact := writeRangeEpic(t, root, "exact", "")
	logEvent(t, root, "exact", events.CherryPicked)
	empty := writeRangeEpic(t, root, "empty", "")
	logEvent(t, root, "empty", events.Type("claimed"))
	sa := writeRangeEpic(t, root, "shareda", "")
	logEvent(t, root, "shareda", events.CherryPicked)
	notOnTrunk := writeRangeEpic(t, root, "ghost", "")
	logEvent(t, root, "ghost", events.CherryPicked)
	noLog := writeRangeEpic(t, root, "nolog", "")
	hasRange := writeRangeEpic(t, root, "ranged", "range_base: aaa\nrange_tip: bbb\n")
	logEvent(t, root, "ranged", events.CherryPicked)
	mapEpic := writeRangeEpic(t, root, "themap", "")
	if err := os.WriteFile(mapEpic, []byte(strings.Replace(mustRead(t, mapEpic), "kind: loop", "kind: map", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	archive := tickets.ArchiveDir(root)
	arch := writeRangeEpic(t, archive, "arch", "")
	logEvent(t, archive, "arch", events.CherryPicked)

	otherRepo := testutil.TempRepo(t)
	otherOld, otherTip := mergeEpics(t, otherRepo, "b-two", "two")
	other := rangeProject(t, otherRepo)
	two := writeRangeEpic(t, other, "two", "")
	logEvent(t, other, "two", events.CherryPicked)

	var logs bytes.Buffer
	run := func(root string) []tickets.MigrationChange {
		env := Env{
			Dirs: config.DirsFrom([]string{"HOME=" + t.TempDir()}), Root: root, LookupRoot: root,
			AppliedPath: filepath.Join(root, AppliedFile), SkipApplied: true,
			Log: slog.New(slog.NewTextHandler(&logs, nil)),
		}
		changes, _, err := Default.Run(env)
		if err != nil {
			t.Fatal(err)
		}
		return changes
	}
	run(root)
	run(other)

	for _, c := range []struct {
		name, path, base, tip string
	}{
		{"exact", exact, exactOld, exactTip},
		{"empty", empty, trunkTip, trunkTip},
		{"archived", arch, archOld, archTip},
		{"second project", two, otherOld, otherTip},
	} {
		e := rangeOf(t, c.path)
		if e.RangeBase != c.base || e.RangeTip != c.tip {
			t.Errorf("%s: range = %s..%s, want %s..%s", c.name, e.RangeBase, e.RangeTip, c.base, c.tip)
		}
	}
	for _, p := range []string{sa, notOnTrunk, noLog, mapEpic} {
		if e := rangeOf(t, p); e.RangeBase != "" || e.RangeTip != "" {
			t.Errorf("%s stamped %s..%s, want nothing", p, e.RangeBase, e.RangeTip)
		}
	}
	if e := rangeOf(t, hasRange); e.RangeBase != "aaa" || e.RangeTip != "bbb" {
		t.Errorf("existing range became %s..%s", e.RangeBase, e.RangeTip)
	}
	for _, want := range []string{"epic=proj/shareda", "reason=shared-entry", "epic=proj/ghost", `reason="commits not on trunk"`, "epic=proj/nolog", `reason="no run log"`} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("log lacks %q:\n%s", want, logs.String())
		}
	}
	if n := strings.Count(logs.String(), "epic=proj/shareda "); n != 1 {
		t.Errorf("shareda logged %d times, want once:\n%s", n, logs.String())
	}

	snapshot := map[string]string{}
	for _, p := range []string{exact, empty, arch, two, sa, hasRange} {
		snapshot[p] = mustRead(t, p)
	}
	applied := readLines(t, filepath.Join(root, AppliedFile))
	if changes := run(root); len(changes) != 0 {
		t.Errorf("second run changed %v", changes)
	}
	for p, want := range snapshot {
		if mustRead(t, p) != want {
			t.Errorf("second run changed %s", p)
		}
	}
	if got := readLines(t, filepath.Join(root, AppliedFile)); !slices.Equal(got, applied) || slices.Index(got, "epic-range-backfill") < 0 {
		t.Errorf("applied list = %v", got)
	}
}

func mustRead(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
