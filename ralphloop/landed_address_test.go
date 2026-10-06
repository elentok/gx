package ralphloop

import (
	"path/filepath"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

// addressTrailerValue is the trailer a ticket in writeEpic's layout lands with.
func addressTrailerValue(scratchDir, epicName, id string) string {
	a, _ := tickets.AddressOfPath(filepath.Join(scratchDir, epicName, "issues", id+"-x.md"), id)
	return a.String()
}

func TestLandTicket_TicketInTrackerLayout_StampsCanonicalAddress(t *testing.T) {
	dir, lp := landFixture(t, false)
	lp.TicketPath = filepath.Join(t.TempDir(), "proj", "main", "issues", "03-x.md")

	res, err := LandTicket(landDepsFor(testDeps()), lp)
	if err != nil {
		t.Fatalf("LandTicket: %v", err)
	}
	if want := "proj:main/03"; res.TrailerValue != want {
		t.Errorf("TrailerValue = %q, want %q", res.TrailerValue, want)
	}
	if found, err := git.TrailerCommitExists(dir, "HEAD", ticketTrailerKey, "proj:main/03"); err != nil || !found {
		t.Errorf("landed commit missing address trailer (found=%v err=%v)", found, err)
	}
}

func TestLandedTickets_ReadsOldAndNewTrailerForms(t *testing.T) {
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

	got, err := landedTickets(dir, "main")
	if err != nil {
		t.Fatalf("landedTickets: %v", err)
	}
	if len(got) != 2 || !got["01"] || !got["02"] {
		t.Errorf("landedTickets = %v, want exactly 01 and 02", got)
	}
}
