package serverapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

func TestServerEpicsUnarchive_ManyTargetsExitsOneOnUnknown(t *testing.T) {
	f, store := archiveFixture(t, "one", "two")
	if _, err := runServer(t, f.H, "epics", "archive", "proj:one", "proj:two"); err != nil {
		t.Fatal(err)
	}

	out, err := runServer(t, f.H, "epics", "unarchive", "proj:one", "proj:nope", "proj:two")
	if exit, ok := err.(*cmdbase.ExitError); !ok || exit.Code != 1 {
		t.Fatalf("err = %v, want exit 1", err)
	}
	for _, want := range []string{"done proj:one", "refused proj:nope", "done proj:two", "2 done, 1 refused"} {
		if !strings.Contains(out, want) {
			t.Errorf("out = %q, missing %q", out, want)
		}
	}
	if got := archivedNames(t, store); len(got) != 0 {
		t.Errorf("archived = %v, want none", got)
	}
}

func TestServerEpicsUnarchive_JSONIsAnEnvelopeForOneTargetWithNewAddress(t *testing.T) {
	f, store := archiveFixture(t, "foo")
	if _, err := runServer(t, f.H, "epics", "archive", "proj:foo"); err != nil {
		t.Fatal(err)
	}
	servertest.WriteCompleteEpic(t, store, "foo")

	out, err := runServer(t, f.H, "epics", "unarchive", "proj:foo", "--json")
	var res wire.TargetsResult
	if err != nil || json.Unmarshal([]byte(out), &res) != nil || len(res.Results) != 1 {
		t.Fatalf("out = %q, err = %v, want a results envelope", out, err)
	}
	if r := res.Results[0]; r.Status != wire.TargetDone || r.NewAddress != "proj:foo-2" {
		t.Errorf("result = %+v, want done with new_address proj:foo-2", r)
	}
}
