package server_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

// Two harnesses side by side each launch an agent through their own fake
// herdr: Start changes no process env, so the fakes do not collide.
func TestServertest_ParallelHarnessesEachDriveAHerdrLaunch(t *testing.T) {
	for i := range 2 {
		t.Run("harness"+strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()
			store := t.TempDir()
			servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
			servertest.SetProjectRepo(t, store, "proj", testutil.TempRepo(t))
			h := servertest.StartWithStore(t, store)
			prompted := make(chan servertest.Prompt, 1)
			h.RegisterLaunch(func(p servertest.Prompt) {
				select {
				case prompted <- p:
				default:
				}
			})

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
				t.Fatalf("add: %+v, %v", res, err)
			}
			select {
			case p := <-prompted:
				if p.Address != "proj:epic-a/01" {
					t.Errorf("prompt address = %q, want proj:epic-a/01", p.Address)
				}
			case <-ctx.Done():
				t.Fatal("no agent prompt reached this harness's herdr")
			}
		})
	}
}
