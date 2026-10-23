package server_test

import (
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

func TestWeb_QueueAddEpicAddress(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicketWith(t, store, "proj", "epic-done", "01", "finished", servertest.TicketOpts{Status: "done"})
	servertest.WriteTicketWith(t, store, "proj", "epic-draft", "01", "wip", servertest.TicketOpts{Status: "draft"})
	servertest.WriteTicketWith(t, store, "proj", "epic-claimed", "01", "wip", servertest.TicketOpts{Status: "claimed"})
	servertest.WriteTicketWith(t, store, "proj", "epic-merged", "01", "old", servertest.TicketOpts{})
	if err := os.WriteFile(filepath.Join(store, "proj", "epic-merged", "ticket.md"), []byte("---\nstatus: done\nmerged_at: 2026-01-02T03:04:05Z\n---\n# Merged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	servertest.WriteTicket(t, store, "proj", "epic-drafting", "01", "first", "")
	if err := os.WriteFile(filepath.Join(store, "proj", "epic-drafting", "ticket.md"), []byte("---\nstatus: draft\n---\n# Drafting\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	servertest.WriteTicket(t, store, "proj", "map-epic", "01", "decide", "")
	if err := os.WriteFile(filepath.Join(store, "proj", "map-epic", "ticket.md"), []byte("---\nkind: map\nstatus: open\n---\n# Map\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.WebAddr = "127.0.0.1:0" })
	hs, err := h.Client.Handshake(t.Context())
	if err != nil || hs.WebURL == "" {
		t.Fatalf("handshake web_url = %q (err %v)", hs.WebURL, err)
	}
	base := hs.WebURL
	cookie := login(t, h.StateDir, base)
	add := func(address, state string) (int, string) {
		body := `{"address":"` + address + `"`
		if state != "" {
			body += `,"state":"` + state + `"`
		}
		return webPostJSON(t, base+"/v1/queue/add", body+"}", cookie, "", jsonPost(base))
	}

	if code, body := add("proj:epic-a", "1"); code != http.StatusOK || reasonOf(t, body) != "" {
		t.Fatalf("epic add = %d %s, want accepted", code, body)
	}
	if got := queueAddresses(t, h); !reflect.DeepEqual(got, []string{"proj:epic-a"}) {
		t.Errorf("queue = %v, want the one epic entry", got)
	}

	for _, c := range []struct{ name, address, reason string }{
		{"already queued", "proj:epic-a", server.ReasonAlreadyQueued},
		{"map epic", "proj:map-epic", server.ReasonMapEpic},
		{"draft epic", "proj:epic-drafting", server.ReasonEpicDraft},
		{"no open tickets", "proj:epic-done", server.ReasonNoOpenTickets},
		{"draft only", "proj:epic-draft", server.ReasonNoOpenTickets},
		{"claimed only", "proj:epic-claimed", server.ReasonNoOpenTickets},
		{"merged", "proj:epic-merged", server.ReasonAlreadyMerged},
	} {
		if code, body := add(c.address, "1"); code != http.StatusOK || reasonOf(t, body) != c.reason {
			t.Errorf("%s = %d %s, want refusal %s", c.name, code, body, c.reason)
		}
	}

	if code, body := add("proj:epic-a", ""); code != http.StatusOK || reasonOf(t, body) != server.ReasonStale {
		t.Errorf("without token = %d %s, want a stale refusal", code, body)
	}
	if code, _ := webPostJSON(t, base+"/v1/queue/add", `{"address":"proj:epic-a","state":"1"}`, nil, "", jsonPost(base)); code != http.StatusUnauthorized {
		t.Errorf("without cookie = %d, want 401", code)
	}
}

func TestQueue_AddEpicAddressQueuesOneEntry(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-a", "02", "second", "")
	h := servertest.StartWithStore(t, store)

	res, err := h.Client.QueueAdd(t.Context(), "proj:epic-a", "")
	if err != nil || res.Refused {
		t.Fatalf("add = %+v, %v; want accepted", res, err)
	}
	if got := queueAddresses(t, h); !reflect.DeepEqual(got, []string{"proj:epic-a"}) {
		t.Errorf("queue = %v, want the one epic entry", got)
	}
}
