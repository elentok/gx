package server_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

// startDesignsMap is startDesignsAt with extra map frontmatter lines.
func startDesignsMap(t *testing.T, frontmatter string) designsFixture {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "map", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "map-impl", "01", "first", "")
	if err := os.WriteFile(filepath.Join(store, "proj", "map", "ticket.md"), []byte("---\nkind: map\n"+frontmatter+"---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return startDesignsIn(t, store)
}

func TestDesignsOpen_LaunchTierFollowsTheMapsStep(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, frontmatter, tier string
	}{
		{"chart", "design_step: chart\n", "strongest"},
		{"empty step", "", "strongest"},
		{"resolve-tickets", "design_step: resolve-tickets\n", "worker"},
		{"unconfirmed destination past chart", "design_step: resolve-tickets\ndestination_unconfirmed: true\n", "strongest"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := startDesignsMap(t, tc.frontmatter)
			res := f.open(t, "proj:map", false)
			if res.Refused || res.Session == nil {
				t.Fatalf("open = %+v", res)
			}
			meta := readMeta(t, f.h, res.Session.ID)
			if meta["tier"] != tc.tier || meta["launch_design"] != true {
				t.Errorf("meta tier=%v launch_design=%v, want %s and true", meta["tier"], meta["launch_design"], tc.tier)
			}
		})
	}
}

func TestDesignsOpenNew_LaunchesOnStrongest(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	res := f.openNew(t, "fresh-idea", "an idea")
	if res.Refused || res.Session == nil {
		t.Fatalf("open = %+v", res)
	}
	if meta := readMeta(t, f.h, res.Session.ID); meta["tier"] != "strongest" || meta["launch_design"] != true {
		t.Errorf("meta tier=%v launch_design=%v", meta["tier"], meta["launch_design"])
	}
}

func TestLinkSession_KeepsTheLaunchTierAndSetsNoLaunchKind(t *testing.T) {
	t.Parallel()
	f := startDesignsMap(t, "design_step: resolve-tickets\n")
	res, err := f.h.Client.CreateSession(f.ctx, server.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind)})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	if _, err := f.h.Client.LinkSession(f.ctx, server.LinkSessionRequest{ID: res.Session.ID, Ticket: "map"}); err != nil {
		t.Fatal(err)
	}
	meta := readMeta(t, f.h, res.Session.ID)
	if meta["tier"] != "worker" || meta["design"] != true {
		t.Errorf("meta tier=%v design=%v, want worker and true", meta["tier"], meta["design"])
	}
	if _, ok := meta["launch_design"]; ok {
		t.Errorf("link set launch_design = %v", meta["launch_design"])
	}
}
