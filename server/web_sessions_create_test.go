package server_test

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/testutil/agentfake"
)

func (f designsFixture) webCreate(t *testing.T, body string) (int, server.SessionResult) {
	t.Helper()
	code, out := webPostJSON(t, f.base+"/v1/sessions/create", body, f.ck, "", jsonPost(f.base))
	var res server.SessionResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("body %q: %v", out, err)
	}
	return code, res
}

func (f designsFixture) projectRoot(t *testing.T) string {
	t.Helper()
	list, err := f.h.Client.ProjectWorktrees(f.ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("worktrees = %+v, %v", list, err)
	}
	return list[0].Root
}

func sameDir(t *testing.T, a, b string) bool {
	t.Helper()
	ra, _ := filepath.EvalSymlinks(a)
	rb, _ := filepath.EvalSymlinks(b)
	return ra == rb
}

func TestWebSessionCreate_StartsASessionInTheProjectRoot(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	root := f.projectRoot(t)
	body, _ := json.Marshal(server.WebCreateSessionRequest{Project: "proj", Dir: root, Name: "chat", Prompt: "hello"})
	code, res := f.webCreate(t, string(body))
	if code != http.StatusOK || res.Refused || res.Session == nil {
		t.Fatalf("create = %d %+v", code, res)
	}
	if !sameDir(t, res.Session.Dir, root) || res.Session.Name != "chat" || res.Session.Harness != string(agentfake.HarnessKind) {
		t.Errorf("session = %+v, want dir %s, name chat, the chat harness", res.Session, root)
	}
}

func TestWebSessionCreate_Refusals(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	root := f.projectRoot(t)
	for name, tc := range map[string]struct{ body, want string }{
		"ticket":          {`{"project":"proj","dir":"` + root + `","ticket":"proj:map"}`, server.ReasonSessionBadRequest},
		"harness":         {`{"project":"proj","dir":"` + root + `","harness":"claude"}`, server.ReasonSessionBadRequest},
		"made worktree":   {`{"project":"proj","dir":"` + root + `","made_worktree":true}`, server.ReasonSessionBadRequest},
		"not json":        {`nope`, server.ReasonSessionBadRequest},
		"slash prompt":    {`{"project":"proj","dir":"` + root + `","prompt":"/clear"}`, server.ReasonPromptPrefixNotAllowed},
		"bang prompt":     {`{"project":"proj","dir":"` + root + `","prompt":" !ls"}`, server.ReasonPromptPrefixNotAllowed},
		"long prompt":     {`{"project":"proj","dir":"` + root + `","prompt":"` + strings.Repeat("a", 32*1024+1) + `"}`, server.ReasonPromptTextTooLong},
		"unknown project": {`{"project":"nope","dir":"` + root + `"}`, server.ReasonSessionBadRequest},
		"foreign dir":     {`{"project":"proj","dir":"` + t.TempDir() + `"}`, server.ReasonSessionBadRequest},
		"bad worktree":    {`{"project":"proj","worktree":"../escape"}`, server.ReasonSessionBadRequest},
	} {
		code, res := f.webCreate(t, tc.body)
		if code != http.StatusOK || !res.Refused || res.Reason != tc.want {
			t.Errorf("%s: create = %d %+v, want refusal %q", name, code, res, tc.want)
		}
	}
	if live := f.live(t); len(live) != 0 {
		t.Errorf("refusals started %d sessions", len(live))
	}
}

func TestWebSessionCreate_NewWorktreeRunsTheSessionThere(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	code, res := f.webCreate(t, `{"project":"proj","worktree":"fresh"}`)
	if code != http.StatusOK || res.Refused || res.Session == nil {
		t.Fatalf("create = %d %+v", code, res)
	}
	if filepath.Base(res.Session.Dir) != "fresh" || sameDir(t, res.Session.Dir, f.projectRoot(t)) || res.Session.Name != "fresh" {
		t.Errorf("session = %+v, want a worktree dir named fresh", res.Session)
	}
	list, err := f.h.Client.ProjectWorktrees(f.ctx)
	if err != nil || len(list) != 1 || !strings.Contains(strings.Join(list[0].Worktrees, "\n"), "fresh") {
		t.Errorf("worktrees = %+v, %v; want fresh listed", list, err)
	}
}

func TestSessionCreate_CliRouteStillAcceptsASlashPrompt(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	res, err := f.h.Client.CreateSession(f.ctx, server.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Prompt: "/x"})
	if err != nil || res.Refused || res.Session == nil {
		t.Errorf("create = %+v, %v; want accepted", res, err)
	}
}
