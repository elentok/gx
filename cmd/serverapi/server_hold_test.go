package serverapi

import (
	"bytes"
	"encoding/json"
	"errors"
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

func holdFixture(t *testing.T) rigs.DesignsFixture {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "map", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "plain", "01", "first", "")
	if err := os.WriteFile(filepath.Join(store, "proj", "map", "ticket.md"), []byte("---\nkind: map\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rigs.WriteEpicFiles(t, store, "map-impl", "---\nstatus: open\n---\n", map[string]string{"01-a.md": rigs.IssueMD("01", "open", "", "A")})
	return rigs.StartDesignsIn(t, store)
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
	var res wire.TargetsResult
	if err := json.Unmarshal([]byte(out), &res); err != nil || res.Results[0].Refused() || len(res.Held) != 0 {
		t.Errorf("unhold --json = %q (%+v, %v), want nothing held", out, res, err)
	}
}

func TestServerHold_JSONAndQueuedRefusal(t *testing.T) {
	f := holdFixture(t)
	out, err := runServer(t, f.H, "hold", "proj:plain", "--json")
	var res wire.TargetsResult
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

func TestServerHold_ManyTargetsPrintLinesSummaryAndExitOne(t *testing.T) {
	f := holdFixture(t)
	out, err := runServer(t, f.H, "hold", "proj:plain", "proj:nope")
	var exit *cmdbase.ExitError
	if !errors.As(err, &exit) || exit.Code != 1 {
		t.Fatalf("err = %v, want exit 1", err)
	}
	if !strings.HasPrefix(out, "done proj:plain\nrefused proj:nope: ") || !strings.HasSuffix(out, "1 done, 1 refused\n") {
		t.Errorf("out = %q, want a line per target and the summary", out)
	}
	if _, err := runServer(t, f.H, "hold", "proj:plain", "proj:map"); err != nil {
		t.Fatal(err)
	}
	out, err = runServer(t, f.H, "unhold", "proj:plain", "proj:map")
	if err != nil || out != "done proj:plain\ndone proj:map\n2 done, 0 refused\n" {
		t.Errorf("unhold = %q, %v", out, err)
	}
	// plain is no longer held, so a second unhold of it is refused beside a held map.
	if _, err := runServer(t, f.H, "hold", "proj:map"); err != nil {
		t.Fatal(err)
	}
	out, err = runServer(t, f.H, "unhold", "proj:map", "proj:plain", "--json")
	var res wire.TargetsResult
	if json.Unmarshal([]byte(out), &res) != nil || len(res.Results) != 2 ||
		res.Results[0].Status != wire.TargetDone || res.Results[1].Reason != wire.ReasonNotOnHold {
		t.Errorf("unhold mix = %q, want done then not-on-hold", out)
	}
	if !errors.As(err, &exit) || exit.Code != 1 {
		t.Errorf("unhold mix err = %v, want exit 1", err)
	}
}

func TestServerHold_JSONIsTheEnvelopeEvenForOneTarget(t *testing.T) {
	f := holdFixture(t)
	out, err := runServer(t, f.H, "hold", "proj:nope", "--json")
	var res wire.TargetsResult
	if json.Unmarshal([]byte(out), &res) != nil || len(res.Results) != 1 || res.Results[0].Reason != wire.ReasonHoldUnknown {
		t.Errorf("--json = %q, want one refused result", out)
	}
	if err == nil {
		t.Error("a refused target exited zero")
	}
}

func TestServerHold_DryRunChangesNothing(t *testing.T) {
	f := holdFixture(t)
	id := f.Open(t, "proj:map", false).Session.ID
	out, err := runServer(t, f.H, "hold", "proj:map", "--dry-run", "--json")
	var res wire.TargetsResult
	if err != nil || json.Unmarshal([]byte(out), &res) != nil || res.Results[0].Status != wire.TargetFits || len(res.Results[0].Sessions) != 1 || res.Results[0].Sessions[0] != id || len(res.Held) != 0 {
		t.Errorf("--dry-run = %q, %v, want fits listing %s and nothing held", out, err, id)
	}
}
