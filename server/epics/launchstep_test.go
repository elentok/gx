package epics_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
)

// Seam: a queued epic held by a cap carries the cap verdict of its next ready
// ticket on its card and its summary; an epic nothing holds carries none.
func TestEpicList_CapHeldEpicCarriesTheCapLine(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	open := map[string]string{"01-a.md": rigs.IssueMD("01", "open", "", "A")}
	rigs.WriteProjectEpic(t, store, "proj", "busy", "---\nstatus: open\n---\n", open)
	rigs.WriteProjectEpic(t, store, "proj", "held", "---\nstatus: open\n---\n", open)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.WebAddr = "127.0.0.1:0"
		c.MaxAgents = 1
	})
	servertest.RegisterTurn(h)
	h.Server.Registry().PutRun("proj:busy", runs.Run{Address: "proj:busy/00", Runner: "herdr"})
	if res, err := h.Client.Queue.Add(t.Context(), "proj:held/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	hs, err := h.Client.Handshake(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	code, body := servertest.WebDo(t, h, http.MethodGet, hs.WebURL+"/v1/epics")
	got := map[string]wire.EpicListRow{}
	for _, r := range rigs.GetEpicList(t, code, body).Epics {
		got[r.Name] = r
	}
	if r := got["held"]; r.Verdict != wire.VerdictConcurrencyCap || r.Reason != "global cap 1/1" {
		t.Errorf("held card = verdict %q reason %q, want %q and the global cap", r.Verdict, r.Reason, wire.VerdictConcurrencyCap)
	}
	if r := got["busy"]; r.Verdict != "" {
		t.Errorf("unqueued card verdict = %q, want none", r.Verdict)
	}
}

// Seam: a launching run's step rides on the epic detail row and the summary
// ticket, and is gone once the launch ends.
func TestEpicDetail_LaunchingRowCarriesItsStep(t *testing.T) {
	t.Parallel()
	h, base := rigs.StartEpicDetail(t, func(store string) {
		rigs.WriteEpicFiles(t, store, "work", "---\nstatus: open\n---\n", map[string]string{
			"01-a.md": rigs.IssueMD("01", "claimed", "", "A"),
		})
	})
	reg := h.Server.Registry()
	reg.PutRunAt("proj:work", runs.Run{Address: "proj:work/01", Runner: "pty"}, time.Now())
	reg.SetLaunchStep("proj:work/01", runs.StepInstallingDeps)

	code, d, body := getEpicDetail(t, h, base, "proj:work")
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	want := wire.LaunchStep{Label: "installing dependencies", Position: 2}
	if got := rowOf(t, d, "proj:work/01").LaunchStep; got != want {
		t.Errorf("launching row step = %+v, want %+v", got, want)
	}

	reg.SetLaunchStep("proj:work/01", "")
	_, d, _ = getEpicDetail(t, h, base, "proj:work")
	if got := rowOf(t, d, "proj:work/01").LaunchStep; got != (wire.LaunchStep{}) {
		t.Errorf("step after the launch = %+v, want none", got)
	}
}
