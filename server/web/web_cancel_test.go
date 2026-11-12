package web_test

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
)

func startCancelWeb(t *testing.T) (*servertest.Harness, string, string) {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicketWith(t, store, "proj", queue.OneOffsEpic, "01", "queued", servertest.TicketOpts{Type: "prompt"})
	servertest.WriteTicketWith(t, store, "proj", queue.OneOffsEpic, "02", "finished", servertest.TicketOpts{Type: "prompt", Status: "done"})
	servertest.WriteTicketWith(t, store, "proj", "epic-a", "01", "plain", servertest.TicketOpts{})
	servertest.SetProjectRepo(t, store, "proj", testutil.TempRepo(t))
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.WebAddr = "127.0.0.1:0" })
	hs, err := h.Client.Handshake(context.Background())
	if err != nil || hs.WebURL == "" {
		t.Fatalf("handshake %+v, %v", hs, err)
	}
	return h, hs.WebURL, store
}

func webCancel(t *testing.T, h *servertest.Harness, base, address string) (int, string) {
	t.Helper()
	return servertest.WebPostJSON(t, base+"/v1/tickets/cancel", `{"address":"`+address+`"}`,
		servertest.Login(t, h.StateDir, servertest.WebBase(base)), "", servertest.JSONPost(base))
}

func ticketStatus(t *testing.T, store, epic, file string) string {
	t.Helper()
	b, err := os.ReadFile(store + "/proj/" + epic + "/issues/" + file)
	if err != nil {
		t.Fatal(err)
	}
	for l := range strings.SplitSeq(string(b), "\n") {
		if s, ok := strings.CutPrefix(l, "status: "); ok {
			return s
		}
	}
	return ""
}

func TestWeb_CancelEndsAQueuedOneOffWithoutAStateToken(t *testing.T) {
	t.Parallel()
	h, base, store := startCancelWeb(t)
	code, body := webCancel(t, h, base, "proj:one-offs/01")
	if code != http.StatusOK || servertest.ReasonOf(t, body) != "" {
		t.Fatalf("cancel = %d %s, want accepted", code, body)
	}
	if got := ticketStatus(t, store, queue.OneOffsArchivedEpic, "01-queued.md"); got != "cancelled" {
		t.Errorf("status = %q, want cancelled", got)
	}
}

func TestWeb_CancelRefusesWhatIsNotAnActiveOneOff(t *testing.T) {
	t.Parallel()
	h, base, store := startCancelWeb(t)
	for addr, want := range map[string]string{
		"proj:epic-a/01":   wire.ReasonNotOneOff,
		"proj:one-offs/02": wire.ReasonTicketDone,
	} {
		code, body := webCancel(t, h, base, addr)
		if code != http.StatusOK || servertest.ReasonOf(t, body) != want {
			t.Errorf("cancel %s = %d %s, want reason %q", addr, code, body, want)
		}
	}
	if got := ticketStatus(t, store, "epic-a", "01-plain.md"); got != "open" {
		t.Errorf("epic ticket status = %q, want untouched", got)
	}
}

func TestWeb_CancelNeedsAnOrigin(t *testing.T) {
	t.Parallel()
	h, base, _ := startCancelWeb(t)
	code, _ := servertest.WebPostJSON(t, base+"/v1/tickets/cancel", `{"address":"proj:one-offs/01"}`,
		servertest.Login(t, h.StateDir, servertest.WebBase(base)), "", map[string]string{"Content-Type": "application/json"})
	if code != http.StatusForbidden {
		t.Errorf("no Origin: status %d, want 403", code)
	}
}
