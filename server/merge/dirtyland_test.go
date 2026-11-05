package merge_test

import (
	"github.com/elentok/gx/server/wire"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

// dirtyLandFixture is an epic whose branch changes shared.txt's first line
// and adds same.txt, while the checkout of the target holds a local edit of the
// last line and an untracked same.txt with the branch's content: the restore is
// clean. With conflict set, the local same.txt differs, so it would conflict.
func dirtyLandFixture(t *testing.T, conflict bool, setup func(f *schedFixture)) *schedFixture {
	t.Helper()
	local := "same"
	if conflict {
		local = "different"
	}
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(f *schedFixture, p *servertest.Turn, _ string) {
		testutil.WriteFile(t, p.Cwd, "shared.txt", "A\nb\nc\n")
		testutil.WriteFile(t, p.Cwd, "same.txt", "same")
		testutil.CommitAll(t, p.Cwd, "work")
		testutil.WriteFile(t, f.Repo, "shared.txt", "a\nb\nC\n")
		testutil.WriteFile(t, f.Repo, "same.txt", local)
	})
	testutil.WriteFile(t, f.Repo, "shared.txt", "a\nb\nc\n")
	testutil.CommitAll(t, f.Repo, "base")
	if setup != nil {
		setup(f)
	}
	return f
}

func (f *schedFixture) read(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(f.Repo, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func (f *schedFixture) backupRefs(t *testing.T) []string {
	t.Helper()
	return strings.Fields(f.Git(t, "for-each-ref", "--format=%(refname)", "refs/gx/backups/"))
}

func (f *schedFixture) landedEvents(t *testing.T) []runlog.Event {
	t.Helper()
	log, _, err := runlog.ReadEvents(filepath.Join(f.Store, "proj"), schedEpic)
	if err != nil {
		t.Fatal(err)
	}
	var got []runlog.Event
	for _, ev := range log {
		if events.Type(ev.Type) == events.DirtyTrunkLanded {
			got = append(got, ev)
		}
	}
	return got
}

func TestDirtyLand_CleanRestoreLandsWithoutAPauseAndKeepsTheLocalContent(t *testing.T) {
	t.Parallel()
	f := dirtyLandFixture(t, false, nil)

	f.runEpic(t, wire.EventRootCompleted)

	if got := f.Git(t, "rev-parse", "main"); got != f.Git(t, "rev-parse", schedEpic) {
		t.Errorf("main at %s, want the feature branch tip", got)
	}
	// The branch's first line and the local last line are both there.
	if got := f.read(t, "shared.txt"); got != "A\nb\nC\n" {
		t.Errorf("shared.txt = %q", got)
	}
	if got := f.read(t, "same.txt"); got != "same" {
		t.Errorf("same.txt = %q", got)
	}
	if md := f.epicMD(t); strings.Contains(md, "land_block") {
		t.Errorf("a pause was stored:\n%s", md)
	}
	f.assertStamps(t, true)
}

func TestDirtyLand_BackupRefHoldsTheFilesAndRefsStashIsUntouched(t *testing.T) {
	t.Parallel()
	f := dirtyLandFixture(t, false, nil)

	f.runEpic(t, wire.EventRootCompleted)

	refs := f.backupRefs(t)
	if len(refs) != 1 {
		t.Fatalf("backup refs = %v, want one", refs)
	}
	if got := f.Git(t, "show", refs[0]+":shared.txt"); got != "a\nb\nC\n" {
		t.Errorf("backup shared.txt = %q, want the local edit", got)
	}
	if got := f.Git(t, "show", refs[0]+":same.txt"); got != "same" {
		t.Errorf("backup same.txt = %q", got)
	}
	if got := strings.TrimSpace(f.Git(t, "stash", "list")); got != "" {
		t.Errorf("stash list = %q, want empty", got)
	}
	if out, _ := exec.Command("git", "-C", f.Repo, "rev-parse", "--verify", "--quiet", "refs/stash").Output(); len(out) != 0 {
		t.Errorf("refs/stash exists: %s", out)
	}
}

func TestDirtyLand_RunLogNamesTheFilesAndTheBackupRef(t *testing.T) {
	t.Parallel()
	f := dirtyLandFixture(t, false, nil)

	f.runEpic(t, wire.EventRootCompleted)

	got := f.landedEvents(t)
	refs := f.backupRefs(t)
	if len(got) != 1 || len(refs) != 1 {
		t.Fatalf("events = %+v, refs = %v", got, refs)
	}
	if got[0].BackupRef != refs[0] || !strings.Contains(got[0].Reason, "shared.txt") || !strings.Contains(got[0].Reason, "same.txt") {
		t.Errorf("event = %+v, want files and ref %s", got[0], refs[0])
	}
}

func TestDirtyLand_PruneDropsAnOldBackupAndKeepsAFreshOne(t *testing.T) {
	t.Parallel()
	f := dirtyLandFixture(t, false, func(f *schedFixture) {
		tree := strings.TrimSpace(f.Git(t, "rev-parse", "HEAD^{tree}"))
		backup := func(ref string, at time.Time) {
			cmd := exec.Command("git", "-C", f.Repo, "commit-tree", tree, "-m", "backup")
			cmd.Env = append(os.Environ(),
				"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
				"GIT_COMMITTER_DATE="+at.Format(time.RFC3339), "GIT_AUTHOR_DATE="+at.Format(time.RFC3339))
			out, err := cmd.Output()
			if err != nil {
				t.Fatal(err)
			}
			f.Git(t, "update-ref", ref, strings.TrimSpace(string(out)))
		}
		backup("refs/gx/backups/old", time.Now().Add(-31*24*time.Hour))
		backup("refs/gx/backups/fresh", time.Now().Add(-29*24*time.Hour))
	})

	f.runEpic(t, wire.EventRootCompleted)

	refs := f.backupRefs(t)
	if len(refs) != 2 || !slices.Contains(refs, "refs/gx/backups/fresh") || slices.Contains(refs, "refs/gx/backups/old") {
		t.Errorf("backup refs = %v, want fresh and the new one, not old", refs)
	}
}

func TestDirtyLand_CheckedOutTargetOtherThanTrunkLandsToo(t *testing.T) {
	t.Parallel()
	f := dirtyLandFixture(t, false, func(f *schedFixture) {
		f.Git(t, "branch", "develop")
		servertest.SetProjectTrunk(t, f.Store, "proj", f.Repo, "develop")
		f.Git(t, "checkout", "develop")
	})

	f.runEpic(t, wire.EventRootCompleted)

	if got := f.Git(t, "rev-parse", "develop"); got != f.Git(t, "rev-parse", schedEpic) {
		t.Errorf("develop at %s, want the feature branch tip", got)
	}
	if got := f.read(t, "shared.txt"); got != "A\nb\nC\n" {
		t.Errorf("shared.txt = %q", got)
	}
	if len(f.landedEvents(t)) != 1 {
		t.Errorf("no landed event")
	}
}

func TestDirtyLand_ConflictingRestoreStillPausesAndTouchesNothing(t *testing.T) {
	t.Parallel()
	f := dirtyLandFixture(t, true, nil)
	main := f.Git(t, "rev-parse", "main")

	ev := f.pauseRoot(t)

	if ev.Kind != string(events.DirtyTrunk) {
		t.Errorf("kind = %q, want %q", ev.Kind, events.DirtyTrunk)
	}
	if got := f.Git(t, "rev-parse", "main"); got != main {
		t.Errorf("main moved to %s", got)
	}
	if got := f.read(t, "shared.txt"); got != "a\nb\nC\n" {
		t.Errorf("shared.txt = %q", got)
	}
	if got := f.read(t, "same.txt"); got != "different" {
		t.Errorf("same.txt = %q", got)
	}
	if refs := f.backupRefs(t); len(refs) != 0 {
		t.Errorf("backup refs = %v, want none", refs)
	}
}
