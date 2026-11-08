package triage_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/herdrfake"
	triagerec "github.com/elentok/gx/triage"
)

// The servertest harness runs this test binary as its session holder and as
// the fake herdr; without both calls the binary re-runs the suite in every
// child and forks without end.
func TestMain(m *testing.M) {
	servertest.RunSessionProcesses()
	herdrfake.RunHelperProcess()
	code := m.Run()
	servertest.RemoveFakeCodex()
	os.Exit(code)
}

func nextTriageEvent(t *testing.T, f servertest.DesignsFixture) wire.Event {
	t.Helper()
	timeout := time.After(10 * time.Second)
	for {
		select {
		case ev := <-f.Evs:
			if ev.Type == wire.EventTriageChanged {
				return ev
			}
		case <-timeout:
			t.Fatal("no triage-changed event")
		}
	}
}

func startFixture(t *testing.T) (servertest.DesignsFixture, *triagerec.Store) {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicketWith(t, store, "proj", "plain", "01", "first", servertest.TicketOpts{})
	f := servertest.StartDesignsIn(t, store)
	return f, triagerec.NewStore(filepath.Join(store, "proj"))
}

func TestTriageEvents_PingPublishesForTheProject(t *testing.T) {
	f, _ := startFixture(t)
	if err := f.H.Client.Triage.Changed(f.Ctx, "proj"); err != nil {
		t.Fatal(err)
	}
	if ev := nextTriageEvent(t, f); ev.Address != "proj" {
		t.Fatalf("address = %q, want proj", ev.Address)
	}
	if err := f.H.Client.Triage.Changed(f.Ctx, "nope"); err == nil {
		t.Fatal("ping for an unknown project succeeded")
	}
}

func TestTriageEvents_RescanBackstopPublishesOncePerRevision(t *testing.T) {
	f, st := startFixture(t)
	rec, err := st.Create()
	if err != nil {
		t.Fatal(err)
	}
	// A ticket ping rescans the store, which runs the backstop.
	if err := f.H.Client.Tickets.Changed(f.Ctx, "proj:plain/01"); err != nil {
		t.Fatal(err)
	}
	if ev := nextTriageEvent(t, f); ev.Address != "proj" {
		t.Fatalf("address = %q, want proj", ev.Address)
	}
	// An unchanged revision publishes nothing on the next rescan; the ping
	// after it is the one event.
	if err := f.H.Client.Tickets.Changed(f.Ctx, "proj:plain/01"); err != nil {
		t.Fatal(err)
	}
	if err := f.H.Client.Triage.Changed(f.Ctx, "proj"); err != nil {
		t.Fatal(err)
	}
	nextTriageEvent(t, f)
	select {
	case ev := <-f.Evs:
		if ev.Type == wire.EventTriageChanged {
			t.Fatalf("second triage-changed for one revision: %+v", ev)
		}
	case <-time.After(300 * time.Millisecond):
	}

	// A new revision publishes again.
	rec.Step = "write"
	if err := st.Save(rec); err != nil {
		t.Fatal(err)
	}
	if err := f.H.Client.Tickets.Changed(f.Ctx, "proj:plain/01"); err != nil {
		t.Fatal(err)
	}
	nextTriageEvent(t, f)
}
