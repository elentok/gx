package merge

import (
	"path/filepath"
	"testing"

	"github.com/elentok/gx/git/history"
	"github.com/elentok/gx/ralphloop/internal/fakes"
	"github.com/elentok/gx/testutil"
)

func TestMergeTicket_TicketInTrackerLayout_StampsCanonicalAddress(t *testing.T) {
	dir, lp := mergeFixture(t, false)
	lp.TicketPath = filepath.Join(t.TempDir(), "proj", "main", "issues", "03-x.md")

	res, err := MergeTicket(MergeDepsOf(fakes.TestDeps()), lp)
	if err != nil {
		t.Fatalf("MergeTicket: %v", err)
	}
	if want := "proj:main/03"; res.TrailerValue != want {
		t.Errorf("TrailerValue = %q, want %q", res.TrailerValue, want)
	}
	if found, err := history.TrailerCommitExists(dir, "HEAD", TicketTrailerKey, "proj:main/03"); err != nil || !found {
		t.Errorf("merged commit missing address trailer (found=%v err=%v)", found, err)
	}
}

func TestMergedTickets_ReadsOldAndNewTrailerForms(t *testing.T) {
	dir := testutil.TempRepo(t)
	for i, value := range []string{
		"main/01",       // legacy <featureBranch>/<id>
		"proj:main/02",  // canonical address
		"proj:other/03", // another epic
		"other/04",      // another epic, legacy
		"not-a-trailer", // unparsable
	} {
		testutil.WriteFile(t, dir, "f.txt", value)
		testutil.CommitAll(t, dir, "c"+string(rune('a'+i)))
		if err := history.AppendTrailer(dir, TicketTrailerKey, value); err != nil {
			t.Fatalf("AppendTrailer: %v", err)
		}
	}

	got, err := MergedTickets(dir, "main")
	if err != nil {
		t.Fatalf("MergedTickets: %v", err)
	}
	if len(got) != 2 || !got["01"] || !got["02"] {
		t.Errorf("MergedTickets = %v, want exactly 01 and 02", got)
	}
}
