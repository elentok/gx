package queue_test

import (
	"github.com/elentok/gx/server/servertest/rigs"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

func TestWeb_QueueAddEpicAddress(t *testing.T) {
	t.Parallel()
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
	for epic, status := range map[string]string{
		"follow-ups": "open", "follow-ups-archived": "open", "epic-cancelled": "cancelled", "epic-closed": "done",
	} {
		servertest.WriteTicket(t, store, "proj", epic, "01", "first", "")
		if err := os.WriteFile(filepath.Join(store, "proj", epic, "ticket.md"), []byte("---\nstatus: "+status+"\n---\n# E\n"), 0o644); err != nil {
			t.Fatal(err)
		}
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
	cookie := servertest.Login(t, h.StateDir, base)
	add := func(address, state string) (int, string) {
		body := `{"address":"` + address + `"`
		if state != "" {
			body += `,"state":"` + state + `"`
		}
		return servertest.WebPostJSON(t, base+"/v1/queue/add", body+"}", cookie, "", servertest.JSONPost(base))
	}

	if code, body := add("proj:epic-a", "1"); code != http.StatusOK || servertest.ReasonOf(t, body) != "" {
		t.Fatalf("epic add = %d %s, want accepted", code, body)
	}
	if got := rigs.QueueAddresses(t, h); !reflect.DeepEqual(got, []string{"proj:epic-a"}) {
		t.Errorf("queue = %v, want the one epic entry", got)
	}

	for _, c := range []struct{ name, address, reason string }{
		{"already queued", "proj:epic-a", wire.ReasonAlreadyQueued},
		{"map epic", "proj:map-epic", wire.ReasonMapEpic},
		{"draft epic", "proj:epic-drafting", wire.ReasonEpicDraft},
		{"follow-ups", "proj:follow-ups", wire.ReasonFollowUpsEpic},
		{"follow-ups archived", "proj:follow-ups-archived", wire.ReasonFollowUpsEpic},
		{"cancelled epic", "proj:epic-cancelled", wire.ReasonEpicNotOpen},
		{"done epic", "proj:epic-closed", wire.ReasonEpicNotOpen},
		{"no open tickets", "proj:epic-done", wire.ReasonNoOpenTickets},
		{"draft only", "proj:epic-draft", wire.ReasonNoOpenTickets},
		{"claimed only", "proj:epic-claimed", wire.ReasonNoOpenTickets},
		{"merged", "proj:epic-merged", wire.ReasonAlreadyMerged},
	} {
		if code, body := add(c.address, "1"); code != http.StatusOK || servertest.ReasonOf(t, body) != c.reason {
			t.Errorf("%s = %d %s, want refusal %s", c.name, code, body, c.reason)
		}
	}

	if code, body := add("proj:epic-a", ""); code != http.StatusOK || servertest.ReasonOf(t, body) != wire.ReasonStale {
		t.Errorf("without token = %d %s, want a stale refusal", code, body)
	}
	if code, _ := servertest.WebPostJSON(t, base+"/v1/queue/add", `{"address":"proj:epic-a","state":"1"}`, nil, "", servertest.JSONPost(base)); code != http.StatusUnauthorized {
		t.Errorf("without cookie = %d, want 401", code)
	}
}

func TestQueue_MoveAndRemoveAcceptAnEpicAddress(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-b", "01", "second", "")
	h := servertest.StartWithStore(t, store)
	for _, a := range []string{"proj:epic-a", "proj:epic-b"} {
		if res, err := h.Client.Queue.Add(t.Context(), a, ""); err != nil || res.Refused {
			t.Fatalf("add %s = %+v, %v", a, res, err)
		}
	}

	if res, err := h.Client.Queue.Move(t.Context(), "proj:epic-b", 1); err != nil || res.Refused {
		t.Fatalf("move = %+v, %v; want accepted", res, err)
	}
	if got := rigs.QueueAddresses(t, h); !reflect.DeepEqual(got, []string{"proj:epic-b", "proj:epic-a"}) {
		t.Errorf("queue after move = %v", got)
	}

	if res, err := h.Client.Queue.Remove(t.Context(), "proj:epic-b"); err != nil || res.Refused {
		t.Fatalf("remove = %+v, %v; want accepted", res, err)
	}
	if got := rigs.QueueAddresses(t, h); !reflect.DeepEqual(got, []string{"proj:epic-a"}) {
		t.Errorf("queue after remove = %v", got)
	}
}

func TestQueue_AddEpicAddressQueuesOneEntry(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-a", "02", "second", "")
	h := servertest.StartWithStore(t, store)

	res, err := h.Client.Queue.Add(t.Context(), "proj:epic-a", "")
	if err != nil || res.Refused {
		t.Fatalf("add = %+v, %v; want accepted", res, err)
	}
	if got := rigs.QueueAddresses(t, h); !reflect.DeepEqual(got, []string{"proj:epic-a"}) {
		t.Errorf("queue = %v, want the one epic entry", got)
	}
}
