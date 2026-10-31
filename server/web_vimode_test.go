package server_test

import (
	"context"
	"net/http"
	"testing"
)

func TestWebViMode_PostSetsModeAndSnapshotShowsIt(t *testing.T) {
	t.Parallel()
	h, base := startWeb(t)
	cookie := login(t, h.StateDir, base)
	for _, tc := range []struct {
		body string
		want bool
	}{{`{"on":true}`, true}, {`{"on":false}`, false}, {`{}`, true}} {
		code, res := webPostJSON(t, base+"/v1/vimode", tc.body, cookie, "", jsonPost(base))
		if code != http.StatusOK {
			t.Fatalf("web vimode %s = %d %s", tc.body, code, res)
		}
		snap, err := h.Client.Snapshot(context.Background())
		if err != nil || snap.ViMode != tc.want {
			t.Errorf("snapshot vi_mode after %s = %v, %v; want %v", tc.body, snap.ViMode, err, tc.want)
		}
	}
}
