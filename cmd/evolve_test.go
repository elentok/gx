package cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/tickets"
)

type evolveFixture struct {
	env   evolveEnv
	epic  string // path of proj:epic
	sent  []server.OneOffRequest
	reply server.OneOffResult
}

func newEvolveFixture(t *testing.T, mode string, withGx bool, epicFM string) *evolveFixture {
	t.Helper()
	store := t.TempDir()
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(store, "proj", "project.json"), "{}")
	if withGx {
		write(filepath.Join(store, "gx", "project.json"), "{}")
	}
	epic := filepath.Join(store, "proj", "epic")
	write(filepath.Join(epic, "ticket.md"), "---\nstatus: open\n"+epicFM+"---\n\n# epic\n")
	write(filepath.Join(epic, "issues", "01-a.md"), "---\nid: \"01\"\nstatus: done\ntype: implement\n---\nBody.\n")

	f := &evolveFixture{epic: epic, reply: server.OneOffResult{Address: "proj:evolve-epic-x/01"}}
	f.env = evolveEnv{
		storePath: store,
		cfg:       config.Config{Evolve: mode},
		now:       func() time.Time { return time.Date(2026, 3, 4, 5, 6, 0, 0, time.UTC) },
		submit: func(_ context.Context, req server.OneOffRequest) (server.OneOffResult, error) {
			f.sent = append(f.sent, req)
			return f.reply, nil
		},
	}
	return f
}

func (f *evolveFixture) loadEpic(t *testing.T) tickets.Epic {
	t.Helper()
	epics, err := tickets.Load(filepath.Dir(f.epic))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range epics {
		if e.Name == "epic" {
			return e
		}
	}
	t.Fatal("epic not found")
	return tickets.Epic{}
}

func TestEvolveRun_SubmitsOnceWithDedupeKey(t *testing.T) {
	t.Parallel()
	f := newEvolveFixture(t, config.EvolveContributor, false, "")
	var out, errOut bytes.Buffer
	if err := runEvolveRun(context.Background(), f.env, &out, &errOut, "proj:epic", false); err != nil {
		t.Fatal(err)
	}
	if len(f.sent) != 1 {
		t.Fatalf("submits = %d, want 1", len(f.sent))
	}
	want := server.RetroSubmit("proj", "epic", f.env.now())
	if !reflect.DeepEqual(f.sent[0], want) {
		t.Errorf("request = %+v, want %+v", f.sent[0], want)
	}
	if f.sent[0].Unique != "evolve:proj/epic" {
		t.Errorf("unique = %q", f.sent[0].Unique)
	}
}

func TestEvolveRun_EvolvedEpicNeedsForce(t *testing.T) {
	t.Parallel()
	f := newEvolveFixture(t, config.EvolveContributor, false, "evolved_at: 2026-01-02T03:04:05Z\n")
	var out, errOut bytes.Buffer
	err := runEvolveRun(context.Background(), f.env, &out, &errOut, "proj:epic", false)
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("err = %v, want one naming --force", err)
	}
	if len(f.sent) != 0 {
		t.Fatalf("submitted %d without --force", len(f.sent))
	}
	if err := runEvolveRun(context.Background(), f.env, &out, &errOut, "proj:epic", true); err != nil {
		t.Fatal(err)
	}
	if len(f.sent) != 1 {
		t.Fatalf("submits with --force = %d, want 1", len(f.sent))
	}
}

func TestEvolveSkip_StampsBothFieldsAndNoTicket(t *testing.T) {
	t.Parallel()
	f := newEvolveFixture(t, config.EvolveContributor, false, "")
	var out bytes.Buffer
	if err := runEvolveSkip(f.env, &out, "proj:epic"); err != nil {
		t.Fatal(err)
	}
	e := f.loadEpic(t)
	if !e.EvolveSkipped || !e.EvolvedAt.Equal(f.env.now()) || e.EvolveTicket != "" {
		t.Errorf("epic = evolved_at %v skipped %v ticket %q", e.EvolvedAt, e.EvolveSkipped, e.EvolveTicket)
	}
	if len(f.sent) != 0 {
		t.Errorf("skip submitted %d", len(f.sent))
	}
}

func TestEvolve_MaintainerWithoutGxProjectRefuses(t *testing.T) {
	t.Parallel()
	f := newEvolveFixture(t, config.EvolveMaintainer, false, "")
	var out, errOut bytes.Buffer
	check := func(verb string, err error) {
		t.Helper()
		var ae *tickets.AddressError
		if !errors.As(err, &ae) || ae.Code != CodeEvolveNoGxProject {
			t.Errorf("%s: err = %v, want code %s", verb, err, CodeEvolveNoGxProject)
		}
	}
	check("run", runEvolveRun(context.Background(), f.env, &out, &errOut, "proj:epic", false))
	check("skip", runEvolveSkip(f.env, &out, "proj:epic"))
	if len(f.sent) != 0 || f.loadEpic(t).EvolveSkipped {
		t.Error("a refused verb still acted")
	}

	withGx := newEvolveFixture(t, config.EvolveMaintainer, true, "")
	if err := runEvolveRun(context.Background(), withGx.env, &out, &errOut, "proj:epic", false); err != nil {
		t.Errorf("with a gx project: %v", err)
	}
}
