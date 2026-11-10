package prflow_test

import (
	"testing"
	"time"

	"github.com/elentok/gx/server/prflow"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

func TestList_FixEntriesLinkTheirTicketAndSortNewestFirst(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	if err := prflow.Append(dir, prflow.Entry{Time: base, Kind: prflow.KindCIFailed, Checks: []string{"go-test"}}); err != nil {
		t.Fatal(err)
	}
	if err := prflow.Append(dir, prflow.Entry{Time: base.Add(2 * time.Hour), Kind: prflow.KindMarkedReady}); err != nil {
		t.Fatal(err)
	}
	e := tickets.Epic{Name: "epic", Path: dir, Tickets: []tickets.Ticket{
		{Identifier: "02", Type: "ci-fix", Status: "done", DoneAt: base.Add(time.Hour),
			Body: "\n## Resolution\n\nCheck: go-test\nCause: the cache key\nCommit: abc123\nOutcome: fixed\n"},
		{Identifier: "03", Type: "ci-fix", Status: "done", DoneAt: base.Add(30 * time.Minute),
			Body: "\n## Resolution\n\nOutcome: unfixable\n"},
	}}
	got := prflow.List("proj", e)
	if len(got) != 3 {
		t.Fatalf("entries = %+v, want 3 (the unfixable fix is no entry)", got)
	}
	if got[0].Text != "Marked ready for review" || got[0].Kind != wire.PRActivityGood {
		t.Errorf("newest = %+v", got[0])
	}
	fix := got[1]
	if fix.Text != "Fix pushed" || fix.Ticket != "proj:epic/02" || fix.Detail != "Cause: the cache key · Commit: abc123" {
		t.Errorf("fix entry = %+v", fix)
	}
	if got[2].Detail != "go-test" || got[2].Kind != wire.PRActivityBad {
		t.Errorf("oldest = %+v", got[2])
	}
}
