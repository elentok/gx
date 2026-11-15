package designs_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/elentok/gx/server/wire"
)

// Opens Designs while rescans run nonstop: a rescan's prune must not
// drop the workspace a new Design was just placed in.
func TestDesignsOpenNew_RescanNeverDropsTheFreshWorkspaceMember(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	if _, err := f.h.Client.Admin.CreateWorkspace(f.ctx, "Other"); err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					f.h.Server.Rescan()
				}
			}
		}()
	}
	defer func() { close(stop); wg.Wait() }()
	for i := range 25 {
		name := fmt.Sprintf("d%d", i)
		req := wire.DesignOpenRequest{Epic: name, Workspace: "Other", NewDesign: true, Idea: "idea", Project: "proj"}
		if res, err := f.h.Client.Sessions.OpenDesign(f.ctx, req); err != nil || res.Refused {
			t.Fatalf("open %s = %+v, %v", name, res, err)
		}
		l, err := f.h.Client.Reads.Workspaces(f.ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, w := range l.Workspaces {
			if w.Name == "Other" && w.Members != i+1 {
				t.Fatalf("after open %d: Other has %d members, want %d", i, w.Members, i+1)
			}
		}
	}
}
