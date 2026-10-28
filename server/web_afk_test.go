package server_test

import (
	"context"
	"net/http"
	"testing"
)

func TestWebAFK_PostSetsModeAndSnapshotShowsIt(t *testing.T) {
	t.Parallel()
	h, base := startWeb(t)
	cookie := login(t, h.StateDir, base)
	for _, tc := range []struct {
		body string
		want bool
	}{{`{"on":true}`, true}, {`{"on":false}`, false}} {
		code, res := webPostJSON(t, base+"/v1/afk", tc.body, cookie, "", jsonPost(base))
		if code != http.StatusOK {
			t.Fatalf("web afk %s = %d %s", tc.body, code, res)
		}
		snap, err := h.Client.Snapshot(context.Background())
		if err != nil || snap.AFK != tc.want {
			t.Errorf("snapshot afk after %s = %v, %v; want %v", tc.body, snap.AFK, err, tc.want)
		}
	}
}
