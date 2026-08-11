package ralphloop

import "github.com/elentok/gx/tickets"
import "testing"

// TestRunScope_Counts_BucketsEveryStatus pins the mapping from
// tickets.RenderedStatus to EpicCounts' buckets, including the
// waiting-for-children overlay (ticket 26) landing in Blocked rather than
// Done.
func TestRunScope_Counts_BucketsEveryStatus(t *testing.T) {
	epic := tickets.Epic{Name: "delivery", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Status: "done"},
		{Number: 2, Identifier: "02", Status: "claimed"},
		{Number: 3, Identifier: "03", Status: "needs-answer"},
		{Number: 4, Identifier: "04", Status: "needs-repair"},
		{Number: 5, Identifier: "05", Status: "open", BlockedBy: []string{"2"}},
		{Number: 6, Identifier: "06", Status: "open"},
		// A done parent still waiting on an open fork child: counts as
		// blocked, not done (Epic.RenderedStatus's waiting-for-children).
		{Number: 7, Identifier: "07", Status: "done"},
		{Number: 8, Identifier: "08", Status: "open", Parent: strPtr("07")},
	}}

	scope := RunScope{wholeEpic: true}
	counts := scope.Counts(epic)

	if counts.Done != 1 {
		t.Errorf("Done = %d, want 1", counts.Done)
	}
	if counts.InProgress != 1 {
		t.Errorf("InProgress = %d, want 1", counts.InProgress)
	}
	if len(counts.ParkedIdentifiers) != 2 {
		t.Errorf("ParkedIdentifiers = %v, want 2 entries", counts.ParkedIdentifiers)
	}
	if counts.Blocked != 2 {
		t.Errorf("Blocked = %d, want 2 (05 blocked-by 02, 07 waiting-for-children)", counts.Blocked)
	}
	if counts.Ready != 2 {
		t.Errorf("Ready = %d, want 2 (06 and 08)", counts.Ready)
	}
	if counts.Total != 8 {
		t.Errorf("Total = %d, want 8", counts.Total)
	}
}

func strPtr(s string) *string { return &s }
