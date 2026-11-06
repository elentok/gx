package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdtest"
	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

func TestAgentsCreateAndList_JSON(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder())
	ctx := context.Background()
	dir := t.TempDir()

	var out bytes.Buffer
	req := wire.CreateSessionRequest{Project: "proj", Dir: dir, Name: "spike", Harness: string(agentfake.HarnessKind)}
	if err := runAgentsCreate(ctx, h.Client, &out, true, req); err != nil {
		t.Fatal(err)
	}
	var created wire.SessionResult
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
	var listed []wire.SessionInfo
	if err := json.Unmarshal(out.Bytes(), &listed); err != nil {
		t.Fatalf("list output %q: %v", out.String(), err)
	}
	if len(listed) != 1 || listed[0].ID != created.Session.ID || listed[0].Dir != dir {
		t.Errorf("list = %s, want session %s", out.String(), created.Session.ID)
	}

	out.Reset()
	if err := runAgentsCreate(ctx, h.Client, &out, true, wire.CreateSessionRequest{Dir: dir}); err != nil {
		t.Fatal(err)
	}
	var refused wire.SessionResult
	if err := json.Unmarshal(out.Bytes(), &refused); err != nil || !refused.Refused || refused.Reason != wire.ReasonSessionBadRequest {
		t.Errorf("create without project = %s, want refused %s", out.String(), wire.ReasonSessionBadRequest)
	}
}

func TestAgentsChat_JSON(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder())
	ctx := context.Background()
	res, err := h.Client.Sessions.Create(ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind)})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	var out bytes.Buffer
	if err := runAgentsChat(ctx, h.Client, &out, true, false, res.Session.ID); err != nil {
		t.Fatal(err)
	}
	var chat wire.SessionChatResponse
	if err := json.Unmarshal(out.Bytes(), &chat); err != nil {
		t.Fatalf("chat output %q: %v", out.String(), err)
	}
	if chat.ConversationID == "" || !chat.Unsupported {
		t.Errorf("chat = %s, want the conversation id and unsupported (fake harness is not claude)", out.String())
	}
}

func TestAgentsChat_ListsAnImagesOnlyMessage(t *testing.T) {
	t.Parallel()
	imgs := []wire.SessionChatImage{{ID: "aa11"}, {Placeholder: true}}
	for _, c := range []struct {
		msg  wire.SessionChatMessage
		want string
	}{
		{wire.SessionChatMessage{Role: "person", Kind: "prompt", Images: imgs}, "person\tprompt\t[2 images]"},
		{wire.SessionChatMessage{Role: "person", Kind: "prompt", Text: "look", Images: imgs[:1]}, "person\tprompt\tlook [1 image]"},
		{wire.SessionChatMessage{Role: "agent", Kind: "text", Text: "hi"}, "agent\ttext\thi"},
	} {
		if got := chatLine(c.msg); got != c.want {
			t.Errorf("chatLine(%+v) = %q, want %q", c.msg, got, c.want)
		}
	}
}

func TestAgentsScreen_JSON(t *testing.T) {
	t.Parallel()
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder())
	ctx := context.Background()
	res, err := h.Client.Sessions.Create(ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind)})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	var out bytes.Buffer
	if err := runAgentsSessionScreen(ctx, h.Client, &out, true, res.Session.ID); err != nil {
		t.Fatal(err)
	}
	var screen wire.SessionScreenResponse
	if err := json.Unmarshal(out.Bytes(), &screen); err != nil || screen.Rows == nil {
		t.Errorf("screen output %q: rows %v, %v; want a rows array", out.String(), screen.Rows, err)
	}
}

func TestAgentsCreateCmd_WorktreeAndPrompt(t *testing.T) {
	sock := filepath.Join(cmdtest.ShortTempDir(t), "a.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	var got wire.CreateSessionRequest
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/sessions/create" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		_ = json.NewEncoder(w).Encode(wire.SessionResult{Session: &wire.SessionInfo{ID: "s1", Name: got.Worktree}})
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
	var res wire.SessionResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil || res.Session == nil || res.Session.ID != "s1" {
		t.Errorf("output = %q, %v; want the created session as JSON", out.String(), err)
	}
}

func TestAgentsCreateCmd_Tier(t *testing.T) {
	sock := filepath.Join(cmdtest.ShortTempDir(t), "a.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	var got wire.CreateSessionRequest
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		_ = json.NewEncoder(w).Encode(wire.SessionResult{Session: &wire.SessionInfo{ID: "s1"}})
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	run := func(args ...string) string {
		cmd := newAgentsCreateCmd(func() (*apiclient.Client, error) { return apiclient.New(sock), nil })
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs(append([]string{"--project", "proj", "--harness", "codex", "--json"}, args...))
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		return string(got.Tier)
	}
	if tier := run(); tier != "worker" {
		t.Errorf("default tier = %q, want worker", tier)
	}
	if tier := run("--tier", "strongest"); tier != "strongest" {
		t.Errorf("tier = %q, want strongest", tier)
	}
	var help bytes.Buffer
	cmd := newAgentsCreateCmd(func() (*apiclient.Client, error) { return nil, nil })
	cmd.SetOut(&help)
	cmd.SetArgs([]string{"--help"})
	_ = cmd.Execute()
	if !strings.Contains(help.String(), "--tier") {
		t.Errorf("help = %q, want --tier", help.String())
	}
	if strings.Contains(help.String(), "default claude") || !strings.Contains(help.String(), "Default harness") {
		t.Errorf("help = %q, want --harness to name the Default harness", help.String())
	}
}

func TestAgentsCreateCmd_HarnessIsEmptyUnlessGiven(t *testing.T) {
	sock := filepath.Join(cmdtest.ShortTempDir(t), "a.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	var got wire.CreateSessionRequest
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		_ = json.NewEncoder(w).Encode(wire.SessionResult{Session: &wire.SessionInfo{ID: "s1"}})
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	run := func(args ...string) string {
		cmd := newAgentsCreateCmd(func() (*apiclient.Client, error) { return apiclient.New(sock), nil })
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetArgs(append([]string{"--project", "proj", "--json"}, args...))
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		return got.Harness
	}
	if h := run(); h != "" {
		t.Errorf("harness = %q, want empty so the server resolves the default", h)
	}
	if h := run("--harness", "codex"); h != "codex" {
		t.Errorf("harness = %q, want codex", h)
	}
}

func TestAgentActionable(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder())
	ctx := context.Background()
	created, err := h.Client.Sessions.Create(ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind)})
	if err != nil || created.Session == nil {
		t.Fatalf("create = %+v, %v", created, err)
	}
	client := func() (*apiclient.Client, error) { return h.Client, nil }

	var out bytes.Buffer
	req := wire.MarkActionableRequest{ID: created.Session.ID, Reason: "review ready", Action: []string{"/review"}}
	if err := runAgentActionable(ctx, client, &out, true, req); err != nil {
		t.Fatal(err)
	}
	var res wire.SessionResult
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
	if err := runAgentActionable(ctx, noServer, &out, false, wire.MarkActionableRequest{Reason: "x"}); err != nil || out.Len() == 0 {
		t.Errorf("outside a session = %q, %v; want a note and success", out.String(), err)
	}
}

func TestAgentActionable_NoServerInSession(t *testing.T) {
	client := func() (*apiclient.Client, error) {
		return apiclient.New(filepath.Join(cmdtest.ShortTempDir(t), "a.sock")), nil
	}
	var out bytes.Buffer
	req := wire.MarkActionableRequest{ID: "s1", Reason: "x"}
	if err := runAgentActionable(context.Background(), client, &out, false, req); err != nil {
		t.Errorf("no server = %q, %v; want success", out.String(), err)
	}
}

func TestAgentTranscript(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder())
	ctx := context.Background()
	created, err := h.Client.Sessions.Create(ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind)})
	if err != nil || created.Session == nil {
		t.Fatalf("create = %+v, %v", created, err)
	}
	client := func() (*apiclient.Client, error) { return h.Client, nil }
	noServer := func() (*apiclient.Client, error) {
		t.Error("outside a session the server was called")
		return h.Client, nil
	}
	decode := func(out *bytes.Buffer) wire.SessionTranscriptResponse {
		t.Helper()
		var got wire.SessionTranscriptResponse
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("output %q: %v", out.String(), err)
		}
		return got
	}

	t.Run("outside a session, text", func(t *testing.T) {
		var out bytes.Buffer
		if err := runAgentTranscript(ctx, noServer, &out, false, ""); err == nil || out.Len() != 0 {
			t.Errorf("err = %v, stdout = %q; want an error and empty stdout", err, out.String())
		}
	})
	t.Run("outside a session, json", func(t *testing.T) {
		var out bytes.Buffer
		if err := runAgentTranscript(ctx, noServer, &out, true, ""); err != nil {
			t.Fatal(err)
		}
		if got := decode(&out); got != (wire.SessionTranscriptResponse{}) {
			t.Errorf("got %+v, want found=false and no harness", got)
		}
	})
	t.Run("session without a file, text", func(t *testing.T) {
		var out bytes.Buffer
		if err := runAgentTranscript(ctx, client, &out, false, created.Session.ID); err == nil || out.Len() != 0 {
			t.Errorf("err = %v, stdout = %q; want an error and empty stdout", err, out.String())
		}
	})
	t.Run("session without a file, json", func(t *testing.T) {
		var out bytes.Buffer
		if err := runAgentTranscript(ctx, client, &out, true, created.Session.ID); err != nil {
			t.Fatal(err)
		}
		if got := decode(&out); got.Found || got.Path != "" {
			t.Errorf("got %+v, want found=false", got)
		}
	})
}

func TestAgentLink(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder())
	ctx := context.Background()
	created, err := h.Client.Sessions.Create(ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind)})
	if err != nil || created.Session == nil {
		t.Fatalf("create = %+v, %v", created, err)
	}
	client := func() (*apiclient.Client, error) { return h.Client, nil }

	var out bytes.Buffer
	if err := runAgentLink(ctx, client, &out, true, wire.LinkSessionRequest{ID: created.Session.ID, Ticket: "auth/03"}); err != nil {
		t.Fatal(err)
	}
	var res wire.SessionResult
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
	if err := runAgentLink(ctx, noServer, &out, false, wire.LinkSessionRequest{Ticket: "x"}); err != nil || out.Len() == 0 {
		t.Errorf("outside a session = %q, %v; want a note and success", out.String(), err)
	}
}

func TestPrintSessionResult_PrintsTheRebaseNoteOnlyWhenSet(t *testing.T) {
	var out bytes.Buffer
	res := wire.SessionResult{Session: &wire.SessionInfo{ID: "s1"}}
	if err := printSessionResult(&out, false, res, nil); err != nil || out.String() != "s1\n" {
		t.Errorf("no note = %q, %v; want just the id", out.String(), err)
	}

	out.Reset()
	res.Session.RebaseNote = "design branch not rebased: conflict"
	if err := printSessionResult(&out, false, res, nil); err != nil || out.String() != "s1\ndesign branch not rebased: conflict\n" {
		t.Errorf("with note = %q, %v; want the id then the note", out.String(), err)
	}

	out.Reset()
	if err := printSessionResult(&out, true, res, nil); err != nil || !strings.Contains(out.String(), `"rebase_note"`) {
		t.Errorf("json = %q, %v; want rebase_note", out.String(), err)
	}
}

func TestAgentLinkAndStep_IgnoreTheClaudeSessionVariable(t *testing.T) {
	t.Setenv("CLAUDE_CODE_SESSION_ID", "claude-1")
	t.Setenv(holder.AgentSessionEnv, "")
	for _, args := range [][]string{{"agent", "link", "auth/03"}, {"agent", "step", "spec"}} {
		var out bytes.Buffer
		d := agentTestDeps(&out, nil)
		d.NewClient = func() (*apiclient.Client, error) {
			t.Errorf("%v: a Claude variable alone reached the server", args)
			return nil, errors.New("unexpected dial")
		}
		if err := execute(t, args, d); err != nil || !strings.Contains(out.String(), "not in a gx session") {
			t.Errorf("%v = %q, %v; want the outside-a-session note", args, out.String(), err)
		}
	}
}

func TestAgentStep_OutsideASessionIsANote(t *testing.T) {
	noServer := func() (*apiclient.Client, error) {
		t.Error("outside a session the server was called")
		return nil, nil
	}
	var out bytes.Buffer
	if err := runAgentStep(context.Background(), noServer, &out, false, wire.ReportStepRequest{Step: "spec"}); err != nil || out.Len() == 0 {
		t.Errorf("outside a session = %q, %v; want a note and success", out.String(), err)
	}
}

func TestAgentsStopAndRemove_JSON(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder())
	ctx := context.Background()
	created, err := h.Client.Sessions.Create(ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind)})
	if err != nil || created.Session == nil {
		t.Fatalf("create = %+v, %v", created, err)
	}
	id := created.Session.ID

	run := func(write sessionWriteFunc) wire.SessionResult {
		t.Helper()
		var out bytes.Buffer
		if err := runAgentsSessionWrite(ctx, h.Client, &out, true, write, id); err != nil {
			t.Fatal(err)
		}
		var res wire.SessionResult
		if err := json.Unmarshal(out.Bytes(), &res); err != nil {
			t.Fatalf("output %q: %v", out.String(), err)
		}
		return res
	}
	if res := run((*apiclient.Sessions).Remove); !res.Refused || res.Reason != wire.ReasonSessionLive {
		t.Errorf("remove live = %+v, want refused %s", res, wire.ReasonSessionLive)
	}
	if res := run((*apiclient.Sessions).Stop); res.Refused || res.Session == nil || !res.Session.Stopped {
		t.Errorf("stop = %+v, want the stopped session", res)
	}
	if res := run((*apiclient.Sessions).Remove); res.Refused {
		t.Errorf("remove = %+v, want removed", res)
	}
}
