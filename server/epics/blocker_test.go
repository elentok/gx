package epics

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/server/wire"
)

const blockerIssue = "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# x\n"

// blockerStore builds project p with epics landed (merged), live, other, and
// target, whose ticket.md holds targetBlockedBy.
func blockerStore(t *testing.T, targetBlockedBy string) (*Epics, string) {
	t.Helper()
	store := t.TempDir()
	write := func(path, body string) {
		t.Helper()
		full := filepath.Join(store, "p", path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"landed", "live", "other"} {
		write(name+"/ticket.md", "---\ntype: epic\n---\n# "+name+"\n")
		write(name+"/issues/01-x.md", blockerIssue)
	}
	write("target/ticket.md", "---\ntype: epic\nkind: feature\nblocked_by: "+targetBlockedBy+"\n---\n# target\n\nbody text\n")
	write("target/issues/01-x.md", blockerIssue)
	return New(Deps{TicketStore: store}), filepath.Join(store, "p", "target", "ticket.md")
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func blocker(t *testing.T, e *Epics, address, other string, remove bool) wire.EpicBlockerResult {
	t.Helper()
	res, err := e.EpicBlocker(wire.EpicBlockerRequest{Address: address, Blocker: other, Remove: remove})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestEpicBlocker_AddKeepsOtherEntries(t *testing.T) {
	t.Parallel()
	e, md := blockerStore(t, "[landed]")
	res := blocker(t, e, "p:target", "p:live", false)
	if res.Refused || !slices.Equal(res.BlockedBy, []string{"landed", "live"}) {
		t.Fatalf("add: %+v", res)
	}
	if got := blocker(t, e, "p:target", "p:live", false); !slices.Equal(got.BlockedBy, []string{"landed", "live"}) {
		t.Errorf("second add is not a no-op: %+v", got)
	}
	res = blocker(t, e, "p:target", "p:live", true)
	if res.Refused || !slices.Equal(res.BlockedBy, []string{"landed"}) {
		t.Fatalf("remove: %+v", res)
	}
	got := readFile(t, md)
	for _, want := range []string{"kind: feature", "# target\n\nbody text\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("ticket.md lost %q:\n%s", want, got)
		}
	}
}

func TestEpicBlocker_StaleEntryDoesNotBlockOthers(t *testing.T) {
	t.Parallel()
	e, _ := blockerStore(t, "[gone, other/01]")
	res := blocker(t, e, "p:target", "p:live", false)
	if res.Refused || !slices.Equal(res.BlockedBy, []string{"gone", "other/01", "live"}) {
		t.Fatalf("add beside stale entries: %+v", res)
	}
	res = blocker(t, e, "p:target", "p:live", true)
	if res.Refused || !slices.Equal(res.BlockedBy, []string{"gone", "other/01"}) {
		t.Fatalf("remove beside stale entries: %+v", res)
	}
}

func TestEpicBlocker_RemovesStaleEntryByStoredRef(t *testing.T) {
	t.Parallel()
	for _, stale := range []string{"other/06", "gone"} {
		e, _ := blockerStore(t, "[landed, "+stale+", live]")
		res := blocker(t, e, "p:target", stale, true)
		if res.Refused || !slices.Equal(res.BlockedBy, []string{"landed", "live"}) {
			t.Errorf("remove %q: %+v", stale, res)
		}
	}
}

func TestEpicBlocker_CycleRefusalWritesNothing(t *testing.T) {
	t.Parallel()
	e, md := blockerStore(t, "[]")
	if res := blocker(t, e, "p:target", "p:live", false); res.Refused {
		t.Fatalf("setup: %+v", res)
	}
	before := readFile(t, md)
	// live now waits on nothing, but target waits on live: live blocked by target closes a cycle.
	live := filepath.Join(filepath.Dir(filepath.Dir(md)), "live", "ticket.md")
	liveBefore := readFile(t, live)
	res := blocker(t, e, "p:live", "p:target", false)
	if !res.Refused || res.Reason != wire.ReasonBlockedByCycle || res.Blocker != "target" || len(res.Chain) == 0 {
		t.Fatalf("res = %+v", res)
	}
	if readFile(t, live) != liveBefore || readFile(t, md) != before {
		t.Error("a refusal wrote a file")
	}
}

func TestEpicBlocker_RefusesUnknownEpicAndOtherProject(t *testing.T) {
	t.Parallel()
	e, md := blockerStore(t, "[]")
	before := readFile(t, md)
	for _, tc := range []struct{ address, other, reason string }{
		{"p:target", "p:nope", "unknown-epic"},
		{"p:target", "q:live", "blocked-by-other-project"},
		{"p:target", "p:target", "blocked-by-own-epic"},
		{"p:nope", "p:live", "unknown-epic"},
		{"target", "p:live", wire.ReasonInvalidAddress},
		{"p:target", "live", wire.ReasonInvalidAddress},
	} {
		res := blocker(t, e, tc.address, tc.other, false)
		if !res.Refused || res.Reason != tc.reason {
			t.Errorf("%s + %s: %+v, want reason %s", tc.address, tc.other, res, tc.reason)
		}
	}
	if readFile(t, md) != before {
		t.Error("a refusal wrote a file")
	}
}
