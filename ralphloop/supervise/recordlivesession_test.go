package supervise

import (
	"testing"

	"github.com/elentok/gx/ralphloop/deps"
	"github.com/elentok/gx/ralphloop/internal/fakes"
	"github.com/elentok/gx/ralphloop/internal/fixture"
	"github.com/elentok/gx/testutil/runnerfake"
)

func TestRecordLiveSession_AppendsOnceAndReturnsSession(t *testing.T) {
	t.Parallel()
	path := fixture.WriteFrontmatterTicket(t, "claimed")
	d := deps.Deps{Runner: fakes.IdleRunner("iter-01")}

	for range 2 { // a reclaimed run finishes twice
		if got := RecordLiveSession(d, "iter-01", path); got != "sess-1" {
			t.Fatalf("RecordLiveSession = %q, want sess-1", got)
		}
	}

	if got := fixture.MustParse(t, path).SessionIDs; len(got) != 1 || got[0] != "sess-1" {
		t.Errorf("SessionIDs = %v, want [sess-1] once", got)
	}
}

func TestRecordLiveSession_NoLiveAgentLeavesTicketAlone(t *testing.T) {
	t.Parallel()
	path := fixture.WriteFrontmatterTicket(t, "claimed")
	d := deps.Deps{Runner: runnerfake.NewRunner()}

	if got := RecordLiveSession(d, "iter-01", path); got != "" {
		t.Errorf("RecordLiveSession = %q, want empty", got)
	}
	if got := fixture.MustParse(t, path).SessionIDs; len(got) != 0 {
		t.Errorf("SessionIDs = %v, want none", got)
	}
}
