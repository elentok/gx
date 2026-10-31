package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
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
		cmd := newDesignsOpenCmd(deps{env: envFor(t)})
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
	h := servertest.StartWithStore(t, store, servertest.WithTestHolder(), servertest.WithChatHarness(string(agentfake.HarnessKind)), servertest.WithDefaultHarness(string(agentfake.HarnessKind)))

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

func TestDesignsDelete_RefusalIsAnErrorAndSuccessDeletes(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "plain", "01", "first", "")
	bare := filepath.Join(store, "proj", "bare")
	if err := os.MkdirAll(bare, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bare, "ticket.md"), []byte("---\nkind: map\n---\n\n## Initial prompt\n\nidea\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	servertest.SetProjectRepoNoAutoMerge(t, store, "proj", testutil.TempRepo(t))
	h := servertest.StartWithStore(t, store, servertest.WithTestHolder(), servertest.WithChatHarness(string(agentfake.HarnessKind)), servertest.WithDefaultHarness(string(agentfake.HarnessKind)))

	var out bytes.Buffer
	err := runDesignsDelete(context.Background(), h.Client, &out, false, "proj:plain")
	if err == nil || !strings.Contains(err.Error(), server.ReasonDesignNotMapEpic) {
		t.Errorf("delete of a loop epic: err = %v, want a refusal", err)
	}
	if err := runDesignsDelete(context.Background(), h.Client, &out, false, "proj:bare"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(bare); !os.IsNotExist(err) {
		t.Errorf("epic dir still there: %v", err)
	}
}

func TestDesignsWorktree_JSON(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "map", "01", "first", "")
	mapTicket := filepath.Join(store, "proj", "map", "ticket.md")
	if err := os.WriteFile(mapTicket, []byte("---\nkind: map\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo := testutil.TempRepo(t)
	servertest.SetProjectRepoNoAutoMerge(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, servertest.WithTestHolder())

	run := func() server.DesignWorktreeResult {
		t.Helper()
		var out bytes.Buffer
		if err := runDesignsWorktree(context.Background(), h.Client, &out, true, server.DesignWorktreeRequest{Epic: "proj:map"}); err != nil {
			t.Fatal(err)
		}
		var res server.DesignWorktreeResult
		if err := json.Unmarshal(out.Bytes(), &res); err != nil {
			t.Fatalf("output %q: %v", out.String(), err)
		}
		return res
	}
	first := run()
	if first.Refused || first.Path == "" || first.Note != "" || first.AfterBranch {
		t.Fatalf("first = %+v", first)
	}
	for dir, content := range map[string]string{first.Path: "design\n", repo: "trunk\n"} {
		if err := os.WriteFile(filepath.Join(dir, "c.txt"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		for _, args := range [][]string{{"add", "c.txt"}, {"commit", "-m", "change"}} {
			if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
		}
	}
	if res := run(); res.Path != first.Path || !strings.Contains(res.Note, "conflict") {
		t.Errorf("after conflict = %+v", res)
	}

	if err := os.WriteFile(mapTicket, []byte("---\nkind: map\ndesign_step: branch\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if res := run(); !res.AfterBranch || res.Path != repo {
		t.Errorf("after Branch = %+v, want trunk checkout %s", res, repo)
	}
}

func TestDesignsOpen_JSON(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "map", "01", "first", "")
	if err := os.WriteFile(filepath.Join(store, "proj", "map", "ticket.md"), []byte("---\nkind: map\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	servertest.SetProjectRepoNoAutoMerge(t, store, "proj", testutil.TempRepo(t))
	h := servertest.StartWithStore(t, store, servertest.WithTestHolder(), servertest.WithChatHarness(string(agentfake.HarnessKind)), servertest.WithDefaultHarness(string(agentfake.HarnessKind)))

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
