package server_test

import (
	"net/http"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

func TestWeb_EpicMergeChecksTheShownState(t *testing.T) {
	store := t.TempDir()
	// A done commitless epic is awaiting merge and has no branch to merge.
	servertest.WriteTicketWith(t, store, "proj", "research", "01", "t", servertest.TicketOpts{Status: "done", Type: "research"})
	servertest.SetProjectRepoNoAutoMerge(t, store, "proj", testutil.TempRepo(t))
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.WebAddr = "127.0.0.1:0" })
	hs, err := h.Client.Handshake(t.Context())
	if err != nil || hs.WebURL == "" {
		t.Fatalf("handshake web_url = %q (err %v)", hs.WebURL, err)
	}
	base := hs.WebURL
	cookie := login(t, h.StateDir, base)
	merge := func(state string, headers map[string]string) (int, string) {
		body := `{"address":"proj:research"`
		if state != "" {
			body += `,"state":"` + state + `"`
		}
		return webPostJSON(t, base+"/v1/epics/merge", body+"}", cookie, "", headers)
	}
	merged := func() bool {
		return !tickets.LoadEpicTicketMD(filepath.Join(store, "proj", "research")).MergedAt.IsZero()
	}

	if code, body := merge(string(server.EpicStateRunning), jsonPost(base)); code != http.StatusOK || reasonOf(t, body) != server.ReasonStale {
		t.Errorf("stale state = %d %s, want the stale refusal", code, body)
	}
	if code, body := merge("", jsonPost(base)); code != http.StatusOK || reasonOf(t, body) != server.ReasonStale {
		t.Errorf("no state = %d %s, want the stale refusal", code, body)
	}
	foreign := map[string]string{"Origin": "http://evil.example", "Content-Type": "application/json"}
	if code, _ := merge(string(server.EpicStateAwaitingMerge), foreign); code != http.StatusForbidden {
		t.Errorf("foreign origin = %d, want 403", code)
	}
	if merged() {
		t.Fatal("a refused request merged the epic")
	}

	code, body := merge(string(server.EpicStateAwaitingMerge), jsonPost(base))
	if code != http.StatusOK || reasonOf(t, body) != "" || !merged() {
		t.Errorf("matching state = %d %s (merged %v), want a merge", code, body, merged())
	}
}
