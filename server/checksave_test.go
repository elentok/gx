package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/check"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/testutil"
)

func saveChecks(t *testing.T, h interface {
	SaveChecks(context.Context, server.SaveChecksRequest) (server.SaveChecksResult, error)
}, target, cfg string) server.SaveChecksResult {
	t.Helper()
	res, err := h.SaveChecks(context.Background(), server.SaveChecksRequest{Project: "demo", Target: target, Checks: json.RawMessage(cfg)})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestSaveChecks_RepoMakesOneCommitOfTheFileAlone(t *testing.T) {
	t.Parallel()
	h, repo := checksFixture(t, "")
	testutil.WriteFile(t, repo, "other.txt", "wip")
	gitIn(t, repo, "add", "other.txt")
	testutil.WriteFile(t, repo, "untracked.txt", "x")
	before := gitIn(t, repo, "rev-parse", "HEAD")
	if !demoChecks(t, h).Missing {
		t.Fatal("fixture should start missing")
	}

	res := saveChecks(t, h.Client, "repo", `{"test": {"full": "make test"}}`)
	if res.Refused || res.Commit == "" || res.Trunk == "" {
		t.Fatalf("result = %+v", res)
	}
	if got := gitIn(t, repo, "rev-list", "--count", before+"..HEAD"); got != "1" {
		t.Errorf("new commits = %s", got)
	}
	if files := gitIn(t, repo, "show", "--name-only", "--format=", "HEAD"); files != check.RepoConfigPath {
		t.Errorf("commit files = %q", files)
	}
	data, err := os.ReadFile(filepath.Join(repo, check.RepoConfigPath))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{\n  \"test\": {\n    \"full\": \"make test\"\n  }\n}\n" {
		t.Errorf("file = %q", data)
	}
	if st := gitIn(t, repo, "status", "--porcelain"); !strings.Contains(st, "A  other.txt") || !strings.Contains(st, "?? untracked.txt") {
		t.Errorf("other changes disturbed: %q", st)
	}
	if demoChecks(t, h).Missing {
		t.Error("project still missing after save")
	}
}

func TestSaveChecks_WritesConfigAsSent(t *testing.T) {
	t.Parallel()
	for _, cfg := range []string{`{}`, `{"test":false}`, `{"test":{"full":"make test"}}`} {
		h, repo := checksFixture(t, "")
		if res := saveChecks(t, h.Client, "repo", cfg); res.Refused {
			t.Fatalf("%s: %+v", cfg, res)
		}
		data, err := os.ReadFile(filepath.Join(repo, check.RepoConfigPath))
		if err != nil {
			t.Fatal(err)
		}
		var got, want any
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatal(err)
		}
		_ = json.Unmarshal([]byte(cfg), &want)
		a, _ := json.Marshal(got)
		b, _ := json.Marshal(want)
		if string(a) != string(b) {
			t.Errorf("sent %s, file %s", b, a)
		}
		if demoChecks(t, h).Missing {
			t.Errorf("%s: project still missing", cfg)
		}
	}
}

func TestSaveChecks_ProjectJSONKeepsOtherKeys(t *testing.T) {
	t.Parallel()
	h, repo := checksFixture(t, `,"trunk":"main"`)
	res := saveChecks(t, h.Client, "project.json", `{"lint":false}`)
	if res.Refused || res.Path != filepath.Join(h.TicketStore, "demo", "project.json") {
		t.Fatalf("result = %+v", res)
	}
	data, err := os.ReadFile(res.Path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	var checks bytes.Buffer
	_ = json.Compact(&checks, m["checks"])
	if checks.String() != `{"lint":false}` || !strings.Contains(string(m["repo"]), repo) || string(m["trunk"]) != `"main"` {
		t.Errorf("project.json = %s", data)
	}
	if demoChecks(t, h).Missing {
		t.Error("project still missing after save")
	}
}

func TestSaveChecks_VCSNoneOnlyProjectJSON(t *testing.T) {
	t.Parallel()
	h, _ := checksFixture(t, `,"vcs":"none"`)
	res := saveChecks(t, h.Client, "repo", `{}`)
	if !res.Refused || res.Reason != server.ReasonVCSNone || res.Message == "" {
		t.Errorf("repo result = %+v", res)
	}
	if res := saveChecks(t, h.Client, "project.json", `{}`); res.Refused {
		t.Errorf("project.json result = %+v", res)
	}
}

func TestSaveChecks_RefusedCommitChangesNothing(t *testing.T) {
	t.Parallel()
	t.Run("uncommitted checks file", func(t *testing.T) {
		h, repo := checksFixture(t, "")
		writeNested(t, repo, check.RepoConfigPath, `{"lint":false}`)
		before := gitIn(t, repo, "rev-parse", "HEAD")
		res := saveChecks(t, h.Client, "repo", `{"test":false}`)
		if !res.Refused || res.Reason != server.ReasonDirtyChecks || res.Message == "" {
			t.Fatalf("result = %+v", res)
		}
		if gitIn(t, repo, "rev-parse", "HEAD") != before {
			t.Error("trunk moved")
		}
		if data, _ := os.ReadFile(filepath.Join(repo, check.RepoConfigPath)); string(data) != `{"lint":false}` {
			t.Errorf("file = %q", data)
		}
	})
	t.Run("other branch checked out", func(t *testing.T) {
		h, repo := checksFixture(t, "")
		trunk := gitIn(t, repo, "rev-parse", "--abbrev-ref", "HEAD")
		gitIn(t, repo, "checkout", "-q", "-b", "side")
		before := gitIn(t, repo, "rev-parse", trunk)
		res := saveChecks(t, h.Client, "repo", `{}`)
		if !res.Refused || res.Reason != server.ReasonWrongBranch {
			t.Fatalf("result = %+v", res)
		}
		if gitIn(t, repo, "rev-parse", trunk) != before {
			t.Error("trunk moved")
		}
		if _, err := os.Stat(filepath.Join(repo, check.RepoConfigPath)); err == nil {
			t.Error("file written")
		}
	})
	t.Run("git fails the commit", func(t *testing.T) {
		h, repo := checksFixture(t, "")
		hook := filepath.Join(repo, ".git", "hooks", "pre-commit")
		if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		before := gitIn(t, repo, "rev-parse", "HEAD")
		res := saveChecks(t, h.Client, "repo", `{}`)
		if !res.Refused || res.Reason != server.ReasonCommitFailed {
			t.Fatalf("result = %+v", res)
		}
		if gitIn(t, repo, "rev-parse", "HEAD") != before {
			t.Error("trunk moved")
		}
		if _, err := os.Stat(filepath.Join(repo, check.RepoConfigPath)); err == nil {
			t.Error("file left behind")
		}
		if st := gitIn(t, repo, "status", "--porcelain"); st != "" {
			t.Errorf("status = %q", st)
		}
	})
	t.Run("no trunk checkout", func(t *testing.T) {
		h, repo := checksFixture(t, "")
		gitIn(t, repo, "checkout", "-q", "--detach")
		res := saveChecks(t, h.Client, "repo", `{}`)
		if !res.Refused || res.Message == "" {
			t.Fatalf("result = %+v", res)
		}
	})
}

func TestSaveChecks_InvalidConfigIsRefused(t *testing.T) {
	t.Parallel()
	h, repo := checksFixture(t, "")
	before := gitIn(t, repo, "rev-parse", "HEAD")
	for _, target := range []string{"repo", "project.json"} {
		res := saveChecks(t, h.Client, target, `{"deploy":"x"}`)
		if !res.Refused || res.Reason != server.ReasonInvalidChecks || !strings.Contains(res.Message, "deploy") {
			t.Errorf("%s: result = %+v", target, res)
		}
	}
	if gitIn(t, repo, "rev-parse", "HEAD") != before {
		t.Error("trunk moved")
	}
	if !demoChecks(t, h).Missing {
		t.Error("something was saved")
	}
}

func TestWeb_ChecksSaveIsAWrite(t *testing.T) {
	t.Parallel()
	h, base := startWeb(t)
	headers := map[string]string{"Content-Type": "application/json", "Origin": base}
	if code := webSend(t, h, "POST", base+"/v1/checks/save", headers); code != 200 {
		t.Errorf("status = %d", code)
	}
}
