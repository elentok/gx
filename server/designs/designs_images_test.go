package designs_test

import (
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

func (f designsFixture) serveImage(t *testing.T, session, image string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, f.base+"/v1/sessions/images?session="+url.QueryEscape(session)+"&id="+url.QueryEscape(image), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(f.ck)
	resp, err := servertest.WebClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestWebDesigns_FreshRunDoesNotResolveEarlierImageIDs(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	old := f.open(t, "proj:map", false).Session
	waitForHolder(t, filepath.Join(f.h.SessionsDir, old.ID))
	conv, _ := readMeta(t, f.h, old.ID)["conversation_id"].(string)
	servertest.Report(t, conv, agentfake.Control{State: "idle"})
	servertest.WaitForSession(f.ctx, t, f.h, f.evs, old.ID, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionIdle })

	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...)
	saved, err := f.h.Client.Sessions.AddImage(f.ctx, old.ID, png)
	if err != nil || saved.Refused || saved.ID == "" {
		t.Fatalf("upload = %+v, %v", saved, err)
	}
	if code := f.serveImage(t, old.ID, saved.ID); code != http.StatusOK {
		t.Fatalf("serve in the first session = %d, want 200", code)
	}

	body := fmt.Sprintf(`{"epic":"proj:map","new":true,"state":%q}`, sessionToken(f.ctx, t, f.h, old.ID))
	code, out := servertest.WebPostJSON(t, f.base+"/v1/designs/open", body, f.ck, "", servertest.JSONPost(f.base))
	if code != http.StatusOK || servertest.ReasonOf(t, out) != "" {
		t.Fatalf("fresh run = %d %s", code, out)
	}
	var fresh wire.SessionInfo
	for _, s := range f.live(t) {
		if s.ID != old.ID {
			fresh = s
		}
	}
	if fresh.ID == "" {
		t.Fatal("no new session after the fresh run")
	}
	if code := f.serveImage(t, fresh.ID, saved.ID); code != http.StatusNotFound {
		t.Errorf("old image id in the fresh session = %d, want 404", code)
	}
}
