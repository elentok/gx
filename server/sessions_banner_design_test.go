package server_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
)

// exitDesign opens a Design on a map with the given frontmatter, exits its
// session with code, and returns the banners sent for it.
func exitDesign(t *testing.T, frontmatter string, code int) int {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "map", "01", "first", "")
	if err := os.WriteFile(filepath.Join(store, "proj", "map", "ticket.md"), []byte("---\nkind: map\n"+frontmatter+"---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	servertest.SetProjectRepoNoAutoMerge(t, store, "proj", testutil.TempRepo(t))
	h := servertest.StartWithStore(t, store, servertest.WithTestHolder(), servertest.WithChatHarness(string(agentfake.HarnessKind)), withAlerts(true, false))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	evs, err := h.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.Client.OpenDesign(ctx, server.DesignOpenRequest{Epic: "proj:map"})
	if err != nil || res.Session == nil {
		t.Fatalf("open = %+v, %v", res, err)
	}
	id := res.Session.ID
	conv, _ := readMeta(t, h, id)["conversation_id"].(string)
	reach(ctx, t, h, evs, id, conv, agentfake.Control{State: "working"}, server.SessionWorking)
	exitFake(t, conv, code)
	waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionExited })
	banners, _ := waitAlerts(h, 1)
	return len(banners)
}

func TestSessionBanner_DesignExit(t *testing.T) {
	tests := []struct {
		name        string
		frontmatter string
		code        int
		want        int
	}{
		{"clean exit of a Done Design", "status: done\n", 0, 0},
		{"clean exit of a Design in progress", "", 0, 1},
		{"crash of a Done Design", "status: done\n", 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := exitDesign(t, tt.frontmatter, tt.code); got != tt.want {
				t.Errorf("banners = %d, want %d", got, tt.want)
			}
		})
	}
}
