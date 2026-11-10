package serverapi

import (
	"encoding/json"
	"testing"

	"github.com/elentok/gx/server/wire"
)

func TestServerEpicsArchived_JSONListsNewestFirst(t *testing.T) {
	f, _ := archiveFixture(t, "first", "second")
	// Archived one after the other, so "second" has the newer archived_at.
	for _, epic := range []string{"proj:first", "proj:second"} {
		if _, err := runServer(t, f.H, "epics", "archive", epic); err != nil {
			t.Fatal(err)
		}
	}

	out, err := runServer(t, f.H, "epics", "archived", "--json")
	var list wire.ArchivedEpicList
	if err != nil || json.Unmarshal([]byte(out), &list) != nil {
		t.Fatalf("out = %q, err = %v", out, err)
	}
	if len(list.Epics) != 2 || list.Epics[0].Address != "proj:second" || list.Epics[1].Address != "proj:first" {
		t.Errorf("epics = %+v, want second then first", list.Epics)
	}
}

func TestServerEpicsArchivedDetail_JSONHasArchivedAt(t *testing.T) {
	f, _ := archiveFixture(t, "foo")
	if _, err := runServer(t, f.H, "epics", "archive", "proj:foo"); err != nil {
		t.Fatal(err)
	}

	out, err := runServer(t, f.H, "epics", "archived-detail", "proj:foo", "--json")
	var d wire.EpicDetail
	if err != nil || json.Unmarshal([]byte(out), &d) != nil {
		t.Fatalf("out = %q, err = %v", out, err)
	}
	if d.Address != "proj:foo" || d.ArchivedAt.IsZero() {
		t.Errorf("detail = %+v, want proj:foo with archived_at", d)
	}
}

func TestServerEpicsArchivedDetail_UnknownAddressFails(t *testing.T) {
	f, _ := archiveFixture(t, "foo")
	if out, err := runServer(t, f.H, "epics", "archived-detail", "proj:foo"); err == nil {
		t.Errorf("out = %q, want an error for an epic that is not archived", out)
	}
}
