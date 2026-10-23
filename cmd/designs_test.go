package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
)

func TestDesignOpenRequest(t *testing.T) {
	got, err := designOpenRequest("fresh", "an idea", "proj", false)
	want := server.DesignOpenRequest{Epic: "fresh", NewDesign: true, Idea: "an idea", Project: "proj"}
	if err != nil || got != want {
		t.Errorf("request = %+v, %v, want %+v", got, err, want)
	}
	if got, err := designOpenRequest("proj:map", "", "", true); err != nil || got != (server.DesignOpenRequest{Epic: "proj:map", New: true}) {
		t.Errorf("plain request = %+v, %v", got, err)
	}
}

func TestDesignsOpenCmd_UsageErrors(t *testing.T) {
	for name, args := range map[string][]string{
		"idea without project": {"fresh", "--idea", "x"},
		"project without idea": {"fresh", "--project", "proj"},
		"new with new design":  {"fresh", "--new", "--idea", "x", "--project", "proj"},
	} {
		cmd := newDesignsOpenCmd()
		cmd.SetArgs(args)
		cmd.SilenceUsage, cmd.SilenceErrors = true, true
		if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "--") {
			t.Errorf("%s: err = %v, want a usage error", name, err)
		}
	}
}

func TestDesignsOpen_NewDesignJSON(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "plain", "01", "first", "")
	servertest.SetProjectRepoNoAutoMerge(t, store, "proj", testutil.TempRepo(t))
	h := servertest.StartWithStore(t, store, servertest.WithTestHolder(), servertest.WithChatHarness(string(agentfake.HarnessKind)))

	for name, tc := range map[string]struct {
		slug   string
		reason string
	}{"started": {"fresh-idea", ""}, "bad slug": {"Bad Slug", server.ReasonSessionBadRequest}} {
		req, err := designOpenRequest(tc.slug, "an idea", "proj", false)
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := runDesignsOpen(context.Background(), h.Client, &out, true, req); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var res server.SessionResult
		if err := json.Unmarshal(out.Bytes(), &res); err != nil {
			t.Fatalf("%s: output %q: %v", name, out.String(), err)
		}
		if tc.reason == "" && (res.Refused || res.Session == nil || !res.Session.Design) {
			t.Errorf("%s: result = %+v, want a Design session", name, res)
		}
		if tc.reason != "" && (!res.Refused || res.Reason != tc.reason) {
			t.Errorf("%s: result = %+v, want refusal %q", name, res, tc.reason)
		}
	}
}

func TestDesignsOpen_JSON(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "map", "01", "first", "")
	if err := os.WriteFile(filepath.Join(store, "proj", "map", "ticket.md"), []byte("---\nkind: map\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	servertest.SetProjectRepoNoAutoMerge(t, store, "proj", testutil.TempRepo(t))
	h := servertest.StartWithStore(t, store, servertest.WithTestHolder(), servertest.WithChatHarness(string(agentfake.HarnessKind)))

	var first, second server.SessionResult
	for _, res := range []*server.SessionResult{&first, &second} {
		var out bytes.Buffer
		if err := runDesignsOpen(context.Background(), h.Client, &out, true, server.DesignOpenRequest{Epic: "proj:map"}); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(out.Bytes(), res); err != nil {
			t.Fatalf("output %q: %v", out.String(), err)
		}
	}
	if first.Refused || first.Session == nil || second.Session == nil || first.Session.ID != second.Session.ID {
		t.Errorf("open twice = %+v, %+v, want one session", first, second)
	}
}
