package serverapi

import (
	"encoding/json"
	"testing"

	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
)

func TestInitiatives_VerbsPrintTheRoutePayloadAsJSON(t *testing.T) {
	store := t.TempDir()
	rigs.SeedInitiatives(t, store)
	f := rigs.StartDesignsIn(t, store)

	out, err := runServer(t, f.H, "initiatives", "--json")
	var list wire.InitiativeList
	if err != nil || json.Unmarshal([]byte(out), &list) != nil {
		t.Fatalf("out = %q, err = %v", out, err)
	}
	if len(list.Projects) != 1 || list.Projects[0].Project != "proj" || len(list.Projects[0].Initiatives) != 3 {
		t.Errorf("list = %+v, want proj with three Initiatives", list)
	}

	out, err = runServer(t, f.H, "initiative", "proj:alpha", "--json")
	var page wire.InitiativePage
	if err != nil || json.Unmarshal([]byte(out), &page) != nil {
		t.Fatalf("out = %q, err = %v", out, err)
	}
	if page.Slug != "alpha" || len(page.Milestones) != 4 || len(page.Unparsed) != 4 {
		t.Errorf("page = %+v", page)
	}

	if out, err := runServer(t, f.H, "initiative", "proj:plain"); err == nil {
		t.Errorf("out = %q, want an error for an epic that is not an Initiative", out)
	}
}
