package ralphloop

import (
	"path/filepath"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

// addressTrailerValue is the trailer a ticket in writeEpic's layout merges with.
func addressTrailerValue(scratchDir, epicName, id string) string {
	a, _ := tickets.AddressOfPath(filepath.Join(scratchDir, epicName, "issues", id+"-x.md"), id)
	return a.String()
}

func TestMergeTicket_TicketInTrackerLayout_StampsCanonicalAddress(t *testing.T) {
	dir, lp := mergeFixture(t, false)
	lp.TicketPath = filepath.Join(t.TempDir(), "proj", "main", "issues", "03-x.md")

	res, err := MergeTicket(mergeDepsFor(testDeps()), lp)
	if err != nil {
		t.Fatalf("MergeTicket: %v", err)
	}
	if want := "proj:main/03"; res.TrailerValue != want {
		t.Errorf("TrailerValue = %q, want %q", res.TrailerValue, want)
	}
	if found, err := git.TrailerCommitExists(dir, "HEAD", ticketTrailerKey, "proj:main/03"); err != nil || !found {
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
		if err := git.AppendTrailer(dir, ticketTrailerKey, value); err != nil {
			t.Fatalf("AppendTrailer: %v", err)
		}
	}

	got, err := mergedTickets(dir, "main")
	if err != nil {
		t.Fatalf("mergedTickets: %v", err)
	}
	if len(got) != 2 || !got["01"] || !got["02"] {
		t.Errorf("mergedTickets = %v, want exactly 01 and 02", got)
	}
}
