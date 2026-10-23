package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

func TestAgentsCreateAndList_JSON(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder())
	ctx := context.Background()
	dir := t.TempDir()

	var out bytes.Buffer
	req := server.CreateSessionRequest{Project: "proj", Dir: dir, Name: "spike", Harness: string(agentfake.HarnessKind)}
	if err := runAgentsCreate(ctx, h.Client, &out, true, req); err != nil {
		t.Fatal(err)
	}
	var created server.SessionResult
	if err := json.Unmarshal(out.Bytes(), &created); err != nil {
		t.Fatalf("create output %q: %v", out.String(), err)
	}
	if created.Refused || created.Session == nil || created.Session.Name != "spike" {
		t.Fatalf("create = %+v, want the session", created)
	}

	out.Reset()
	if err := runAgentsList(ctx, h.Client, &out, true); err != nil {
		t.Fatal(err)
	}
	var listed []server.SessionInfo
	if err := json.Unmarshal(out.Bytes(), &listed); err != nil {
		t.Fatalf("list output %q: %v", out.String(), err)
	}
	if len(listed) != 1 || listed[0].ID != created.Session.ID || listed[0].Dir != dir {
		t.Errorf("list = %s, want session %s", out.String(), created.Session.ID)
	}

	out.Reset()
	if err := runAgentsCreate(ctx, h.Client, &out, true, server.CreateSessionRequest{Dir: dir}); err != nil {
		t.Fatal(err)
	}
	var refused server.SessionResult
	if err := json.Unmarshal(out.Bytes(), &refused); err != nil || !refused.Refused || refused.Reason != server.ReasonSessionBadRequest {
		t.Errorf("create without project = %s, want refused %s", out.String(), server.ReasonSessionBadRequest)
	}
}

func TestAgentsChat_JSON(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder())
	ctx := context.Background()
	res, err := h.Client.CreateSession(ctx, server.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind)})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	var out bytes.Buffer
	if err := runAgentsChat(ctx, h.Client, &out, true, res.Session.ID); err != nil {
		t.Fatal(err)
	}
	var chat server.SessionChatResponse
	if err := json.Unmarshal(out.Bytes(), &chat); err != nil {
		t.Fatalf("chat output %q: %v", out.String(), err)
	}
	if chat.ConversationID == "" || !chat.Unsupported {
		t.Errorf("chat = %s, want the conversation id and unsupported (fake harness is not claude)", out.String())
	}
}

func TestAgentsScreen_JSON(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder())
	ctx := context.Background()
	res, err := h.Client.CreateSession(ctx, server.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind)})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	var out bytes.Buffer
	if err := runAgentsSessionScreen(ctx, h.Client, &out, true, res.Session.ID); err != nil {
		t.Fatal(err)
	}
	var screen server.SessionScreenResponse
	if err := json.Unmarshal(out.Bytes(), &screen); err != nil || screen.Rows == nil {
		t.Errorf("screen output %q: rows %v, %v; want a rows array", out.String(), screen.Rows, err)
	}
}

func TestAgentsCreateCmd_WorktreeAndPrompt(t *testing.T) {
	sock := filepath.Join(shortTempDir(t), "a.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	var got server.CreateSessionRequest
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/sessions/create" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		_ = json.NewEncoder(w).Encode(server.SessionResult{Session: &server.SessionInfo{ID: "s1", Name: got.Worktree}})
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	cmd := newAgentsCreateCmd(func() (*apiclient.Client, error) { return apiclient.New(sock), nil })
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--project", "proj", "--worktree", "feat", "--prompt", "fix the bug", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got.Worktree != "feat" || got.Prompt != "fix the bug" || got.Dir != "" {
		t.Errorf("request = %+v, want worktree and prompt with no dir", got)
	}
	var res server.SessionResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil || res.Session == nil || res.Session.ID != "s1" {
		t.Errorf("output = %q, %v; want the created session as JSON", out.String(), err)
	}
}

func TestAgentActionable(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder())
	ctx := context.Background()
	created, err := h.Client.CreateSession(ctx, server.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind)})
	if err != nil || created.Session == nil {
		t.Fatalf("create = %+v, %v", created, err)
	}
	client := func() (*apiclient.Client, error) { return h.Client, nil }

	var out bytes.Buffer
	req := server.MarkActionableRequest{ID: created.Session.ID, Reason: "review ready", Action: []string{"/review"}}
	if err := runAgentActionable(ctx, client, &out, true, req); err != nil {
		t.Fatal(err)
	}
	var res server.SessionResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("output %q: %v", out.String(), err)
	}
	if res.Refused || res.Session == nil || res.Session.Actionable.Reason != "review ready" || len(res.Session.Actionable.Action) != 1 {
		t.Errorf("actionable = %s, want the reason and action", out.String())
	}

	out.Reset()
	noServer := func() (*apiclient.Client, error) {
		t.Error("outside a session the server was called")
		return h.Client, nil
	}
	if err := runAgentActionable(ctx, noServer, &out, false, server.MarkActionableRequest{Reason: "x"}); err != nil || out.Len() == 0 {
		t.Errorf("outside a session = %q, %v; want a note and success", out.String(), err)
	}
}

func TestAgentActionable_NoServerInSession(t *testing.T) {
	client := func() (*apiclient.Client, error) { return apiclient.New(filepath.Join(shortTempDir(t), "a.sock")), nil }
	var out bytes.Buffer
	req := server.MarkActionableRequest{ID: "s1", Reason: "x"}
	if err := runAgentActionable(context.Background(), client, &out, false, req); err != nil {
		t.Errorf("no server = %q, %v; want success", out.String(), err)
	}
}

func TestAgentLink(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder())
	ctx := context.Background()
	created, err := h.Client.CreateSession(ctx, server.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind)})
	if err != nil || created.Session == nil {
		t.Fatalf("create = %+v, %v", created, err)
	}
	client := func() (*apiclient.Client, error) { return h.Client, nil }

	var out bytes.Buffer
	if err := runAgentLink(ctx, client, &out, true, server.LinkSessionRequest{ID: created.Session.ID, Ticket: "auth/03"}); err != nil {
		t.Fatal(err)
	}
	var res server.SessionResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("output %q: %v", out.String(), err)
	}
	if res.Refused || res.Session == nil || res.Session.Ticket != "proj:auth/03" {
		t.Errorf("link = %s, want the ticket qualified with the project", out.String())
	}

	out.Reset()
	noServer := func() (*apiclient.Client, error) {
		t.Error("outside a session the server was called")
		return h.Client, nil
	}
	if err := runAgentLink(ctx, noServer, &out, false, server.LinkSessionRequest{Ticket: "x"}); err != nil || out.Len() == 0 {
		t.Errorf("outside a session = %q, %v; want a note and success", out.String(), err)
	}
}

func TestAgentsStopAndRemove_JSON(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder())
	ctx := context.Background()
	created, err := h.Client.CreateSession(ctx, server.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind)})
	if err != nil || created.Session == nil {
		t.Fatalf("create = %+v, %v", created, err)
	}
	id := created.Session.ID

	run := func(write sessionWriteFunc) server.SessionResult {
		t.Helper()
		var out bytes.Buffer
		if err := runAgentsSessionWrite(ctx, h.Client, &out, true, write, id); err != nil {
			t.Fatal(err)
		}
		var res server.SessionResult
		if err := json.Unmarshal(out.Bytes(), &res); err != nil {
			t.Fatalf("output %q: %v", out.String(), err)
		}
		return res
	}
	if res := run((*apiclient.Client).RemoveSession); !res.Refused || res.Reason != server.ReasonSessionLive {
		t.Errorf("remove live = %+v, want refused %s", res, server.ReasonSessionLive)
	}
	if res := run((*apiclient.Client).StopSession); res.Refused || res.Session == nil || !res.Session.Stopped {
		t.Errorf("stop = %+v, want the stopped session", res)
	}
	if res := run((*apiclient.Client).RemoveSession); res.Refused {
		t.Errorf("remove = %+v, want removed", res)
	}
}
