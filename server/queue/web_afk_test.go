package queue_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/elentok/gx/server/servertest"
)

func TestWebAFK_PostSetsModeAndSnapshotShowsIt(t *testing.T) {
	t.Parallel()
	h, base := servertest.StartWeb(t)
	cookie := servertest.Login(t, h.StateDir, base)
	for _, tc := range []struct {
		body string
		want bool
	}{{`{"on":true}`, true}, {`{"on":false}`, false}} {
		code, res := servertest.WebPostJSON(t, base+"/v1/afk", tc.body, cookie, "", servertest.JSONPost(base))
		if code != http.StatusOK {
			t.Fatalf("web afk %s = %d %s", tc.body, code, res)
		}
		snap, err := h.Client.Snapshot(context.Background())
		if err != nil || snap.AFK != tc.want {
			t.Errorf("snapshot afk after %s = %v, %v; want %v", tc.body, snap.AFK, err, tc.want)
		}
	}
}
