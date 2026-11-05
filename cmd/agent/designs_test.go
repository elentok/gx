package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/cmd/cmdtest"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
)

func TestDesignOpenRequest(t *testing.T) {
	got, err := designOpenRequest("fresh", "an idea", "proj", false)
	want := wire.DesignOpenRequest{Epic: "fresh", NewDesign: true, Idea: "an idea", Project: "proj"}
	if err != nil || got != want {
		t.Errorf("request = %+v, %v, want %+v", got, err, want)
	}
	if got, err := designOpenRequest("proj:map", "", "", true); err != nil || got != (wire.DesignOpenRequest{Epic: "proj:map", New: true}) {
		t.Errorf("plain request = %+v, %v", got, err)
	}
}

func TestDesignsOpenCmd_UsageErrors(t *testing.T) {
	for name, args := range map[string][]string{
		"idea without project": {"fresh", "--idea", "x"},
		"project without idea": {"fresh", "--project", "proj"},
		"new with new design":  {"fresh", "--new", "--idea", "x", "--project", "proj"},
	} {
		cmd := newDesignsOpenCmd(Deps{Env: cmdbase.Env{Env: cmdtest.EnvFor(t)}})
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
	}{"started": {"fresh-idea", ""}, "bad slug": {"Bad Slug", wire.ReasonSessionBadRequest}} {
		req, err := designOpenRequest(tc.slug, "an idea", "proj", false)
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := runDesignsOpen(context.Background(), h.Client, &out, true, req); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var res wire.SessionResult
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

func TestDesignsOpen_FailedStart(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "plain", "01", "first", "")
	servertest.SetProjectRepoNoAutoMerge(t, store, "proj", testutil.TempRepo(t))
	h := servertest.StartWithStore(t, store, servertest.WithTestHolder(), servertest.WithChatHarness("no-such-harness"), servertest.WithDefaultHarness("no-such-harness"))
	req := wire.DesignOpenRequest{Epic: "fresh-idea", NewDesign: true, Idea: "idea", Project: "proj"}

	var out bytes.Buffer
	err := runDesignsOpen(context.Background(), h.Client, &out, false, req)
	if err == nil {
		t.Fatal("text mode: err = nil, want non-zero exit")
	}
	if !strings.HasPrefix(out.String(), "proj:fresh-idea\n") || strings.Count(out.String(), "\n") < 2 {
		t.Errorf("text output = %q, want the address then the start error", out.String())
	}

	req.Epic = "other-idea"
	out.Reset()
	if err := runDesignsOpen(context.Background(), h.Client, &out, true, req); err != nil {
		t.Fatalf("json mode: %v", err)
	}
	var res wire.SessionResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil || !res.EpicCreated || res.StartError == "" {
		t.Errorf("json result = %+v, %v, want epic_created and start_error", res, err)
	}
}

func TestDesignsOpenCmd_IdeaFromStdin(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "plain", "01", "first", "")
	servertest.SetProjectRepoNoAutoMerge(t, store, "proj", testutil.TempRepo(t))
	h := servertest.StartWithStore(t, store, servertest.WithTestHolder(), servertest.WithChatHarness(string(agentfake.HarnessKind)), servertest.WithDefaultHarness(string(agentfake.HarnessKind)))

	text, err := readIdea("-", strings.NewReader("an idea from stdin"))
	if err != nil || text != "an idea from stdin" {
		t.Fatalf("readIdea = %q, %v", text, err)
	}
	if text, _ := readIdea("literal", strings.NewReader("ignored")); text != "literal" {
		t.Errorf("literal idea = %q", text)
	}

	for name, tc := range map[string]struct {
		slug, idea, reason string
	}{
		"fits":     {"stdin-idea", "an idea", ""},
		"too long": {"long-idea", strings.Repeat("a", wire.MaxSessionPromptBytes+1), wire.ReasonPromptTextTooLong},
	} {
		got, err := readIdea("-", strings.NewReader(tc.idea))
		if err != nil {
			t.Fatal(err)
		}
		req, err := designOpenRequest(tc.slug, got, "proj", false)
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		err = runDesignsOpen(context.Background(), h.Client, &out, false, req)
		if tc.reason == "" && err != nil {
			t.Errorf("%s: %v", name, err)
		}
		if tc.reason != "" && (err == nil || !strings.Contains(err.Error(), tc.reason)) {
			t.Errorf("%s: err = %v, want refusal %q", name, err, tc.reason)
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
	if err == nil || !strings.Contains(err.Error(), wire.ReasonDesignNotMapEpic) {
		t.Errorf("delete of a loop epic: err = %v, want a refusal", err)
	}
	out.Reset()
	if err := runDesignsDelete(context.Background(), h.Client, &out, false, "proj:bare"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("successful delete printed %q, want nothing", out.String())
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

	run := func() wire.DesignWorktreeResult {
		t.Helper()
		var out bytes.Buffer
		if err := runDesignsWorktree(context.Background(), h.Client, &out, true, wire.DesignWorktreeRequest{Epic: "proj:map"}); err != nil {
			t.Fatal(err)
		}
		var res wire.DesignWorktreeResult
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

	var first, second wire.SessionResult
	for _, res := range []*wire.SessionResult{&first, &second} {
		var out bytes.Buffer
		if err := runDesignsOpen(context.Background(), h.Client, &out, true, wire.DesignOpenRequest{Epic: "proj:map"}); err != nil {
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
