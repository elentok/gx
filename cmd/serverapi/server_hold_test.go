package serverapi

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
)

func runServer(t *testing.T, h *servertest.Harness, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	d := Deps{Env: cmdbase.Env{
		Stdout:    &out,
		NewClient: func() (*apiclient.Client, error) { return h.Client, nil },
	}}
	err := execute(t, append([]string{"server"}, args...), d)
	return out.String(), err
}

func holdFixture(t *testing.T) servertest.DesignsFixture {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "map", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "plain", "01", "first", "")
	if err := os.WriteFile(filepath.Join(store, "proj", "map", "ticket.md"), []byte("---\nkind: map\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rigs.WriteEpicFiles(t, store, "map-impl", "---\nstatus: open\n---\n", map[string]string{"01-a.md": rigs.IssueMD("01", "open", "", "A")})
	return servertest.StartDesignsIn(t, store)
}

func TestServerHold_NeedsYesWhileDesignSessionsAreLive(t *testing.T) {
	f := holdFixture(t)
	id := f.Open(t, "proj:map", false).Session.ID

	_, err := runServer(t, f.H, "hold", "proj:map")
	if err == nil || !strings.Contains(err.Error(), id) {
		t.Fatalf("hold without --yes err = %v, want a refusal naming %s", err, id)
	}

	out, err := runServer(t, f.H, "hold", "proj:map", "--yes")
	if err != nil || !strings.Contains(out, "ended sessions: "+id) {
		t.Fatalf("hold --yes = %q, %v, want the ended session listed", out, err)
	}

	out, err = runServer(t, f.H, "unhold", "proj:map", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var res wire.HoldResult
	if err := json.Unmarshal([]byte(out), &res); err != nil || res.Refused || len(res.Held) != 0 {
		t.Errorf("unhold --json = %q (%+v, %v), want nothing held", out, res, err)
	}
}

func TestServerHold_JSONAndQueuedRefusal(t *testing.T) {
	f := holdFixture(t)
	out, err := runServer(t, f.H, "hold", "proj:plain", "--json")
	var res wire.HoldResult
	if err != nil || json.Unmarshal([]byte(out), &res) != nil || len(res.Held) != 1 || res.Held[0] != "proj:plain" {
		t.Fatalf("hold --json = %q, %v, want proj:plain held", out, err)
	}
	if _, err := runServer(t, f.H, "unhold", "proj:plain"); err != nil {
		t.Fatal(err)
	}
	if r, err := f.H.Client.Queue.Add(f.Ctx, "proj:plain", ""); err != nil || r.Refused {
		t.Fatalf("queue add = %+v, %v", r, err)
	}
	if _, err := runServer(t, f.H, "hold", "proj:plain"); err == nil || !strings.Contains(err.Error(), "Remove it from the queue first") {
		t.Errorf("hold of a queued epic err = %v, want the queue text", err)
	}
}
