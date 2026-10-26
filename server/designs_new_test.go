package server_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

func (f designsFixture) openNew(t *testing.T, slug, idea string) server.SessionResult {
	t.Helper()
	res, err := f.h.Client.OpenDesign(f.ctx, server.DesignOpenRequest{Epic: slug, NewDesign: true, Idea: idea, Project: "proj"})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func (f designsFixture) mapPath(slug string) string {
	return filepath.Join(f.h.TicketStore, "proj", slug, "ticket.md")
}

func TestDesignsOpenNew_WritesTheMapThenStartsTheSession(t *testing.T) {
	f := startDesigns(t)
	res := f.openNew(t, "fresh-idea", "a thing\nwith lines")
	if res.Refused || res.Session == nil || res.EpicCreated {
		t.Fatalf("open = %+v", res)
	}
	meta := readMeta(t, f.h, res.Session.ID)
	want := map[string]any{"dir": f.repo, "name": "fresh-idea", "ticket": "proj:fresh-idea", "design": true, "first_prompt": "/gx-design proj:fresh-idea"}
	for k, v := range want {
		if meta[k] != v {
			t.Errorf("meta[%s] = %v, want %v", k, meta[k], v)
		}
	}
	raw, err := os.ReadFile(f.mapPath("fresh-idea"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	for _, w := range []string{"kind: map", "status: open", "design_step: chart", "## Initial prompt\n\na thing\nwith lines"} {
		if !strings.Contains(got, w) {
			t.Errorf("map lacks %q:\n%s", w, got)
		}
	}
	if strings.Contains(got, "Destination") || strings.Count(got, "\n## ") != 1 {
		t.Errorf("map has more than Initial prompt:\n%s", got)
	}
}

func TestDesignsOpenNew_StartFailureReturnsJSONWithTheMarker(t *testing.T) {
	store := t.TempDir()
	repo := testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "plain", "01", "first", "")
	servertest.SetProjectRepoNoAutoMerge(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, servertest.WithTestHolder(), servertest.WithChatHarness("no-such-harness"))
	res, err := h.Client.OpenDesign(context.Background(), server.DesignOpenRequest{Epic: "fresh-idea", NewDesign: true, Idea: "idea", Project: "proj"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Refused || !res.EpicCreated || res.Session != nil {
		t.Fatalf("open = %+v, want refusal with epic_created", res)
	}
	if _, err := os.Stat(filepath.Join(store, "proj", "fresh-idea", "ticket.md")); err != nil {
		t.Errorf("map missing: %v", err)
	}
}

func TestDesignsOpenNew_LinkingToItsOwnAddressKeepsDesign(t *testing.T) {
	f := startDesigns(t)
	res := f.openNew(t, "fresh-idea", "a thing")
	linked, err := f.h.Client.LinkSession(f.ctx, server.LinkSessionRequest{ID: res.Session.ID, Ticket: "fresh-idea"})
	if err != nil || linked.Refused || !linked.Session.Design {
		t.Fatalf("link = %+v, %v; want Design kept", linked.Session, err)
	}
}

func TestDesignsOpenNew_SecondCallIsRefusedAsEpicExists(t *testing.T) {
	f := startDesigns(t)
	f.openNew(t, "fresh-idea", "one")
	if second := f.openNew(t, "fresh-idea", "two"); !second.Refused || second.Reason != server.ReasonDesignEpicExists || second.EpicCreated {
		t.Fatalf("second = %+v, want epic-exists with no marker", second)
	}
	if got := len(f.live(t)); got != 1 {
		t.Errorf("live sessions = %d, want 1", got)
	}
}

func TestDesignsOpenNew_Refusals(t *testing.T) {
	f := startDesigns(t)
	for name, tc := range map[string]struct{ slug, idea, want string }{
		"bad slug":     {"Bad Slug", "idea", server.ReasonSessionBadRequest},
		"path slug":    {"a/b", "idea", server.ReasonSessionBadRequest},
		"epic exists":  {"plain", "idea", server.ReasonDesignEpicExists},
		"map exists":   {"map", "idea", server.ReasonDesignEpicExists},
		"empty idea":   {"fresh-idea", " \n", server.ReasonEmptyPrompt},
		"idea too big": {"fresh-idea", strings.Repeat("a", 32*1024+1), server.ReasonPromptTextTooLong},
	} {
		if res := f.openNew(t, tc.slug, tc.idea); !res.Refused || res.Reason != tc.want || res.EpicCreated {
			t.Errorf("%s: open = %+v, want reason %q with no marker", name, res, tc.want)
		}
	}
	if got := len(f.live(t)); got != 0 {
		t.Errorf("live sessions = %d, want 0", got)
	}
	if _, err := os.Stat(f.mapPath("fresh-idea")); err == nil {
		t.Error("a refusal wrote the map")
	}
}
