package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/server/chatdrafts"
	"github.com/elentok/gx/server/drafthistory"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/ops"
	"github.com/elentok/gx/server/workspaces"
)

// draftsCmdDeps dials the fixture server and reads stdin from stdin.
func draftsCmdDeps(f rigs.DesignsFixture, stdin string, out *bytes.Buffer) Deps {
	d := agentTestDeps(out, nil)
	d.Env.Stdin = strings.NewReader(stdin)
	d.NewClient = func() (*apiclient.Client, error) { return f.H.Client, nil }
	return d
}

func chatIn(t *testing.T, f rigs.DesignsFixture, ws string) string {
	t.Helper()
	if _, err := f.H.Client.Admin.CreateWorkspace(f.Ctx, ws); err != nil {
		t.Fatal(err)
	}
	res, err := f.H.Client.Sessions.Create(f.Ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir()})
	if err != nil || res.Session == nil {
		t.Fatalf("create chat = %+v, %v", res, err)
	}
	if mv, err := f.H.Client.Admin.MoveToWorkspace(f.Ctx, ws, res.Session.ID); err != nil || mv.Results[0].Refused() {
		t.Fatalf("move chat = %+v, %v", mv, err)
	}
	return res.Session.ID
}

func TestAgentsDrafts_CreateReadsBriefFromStdinAndPrintsID(t *testing.T) {
	f := rigs.StartDesigns(t)
	var out bytes.Buffer
	t.Setenv(holder.AgentSessionEnv, "")
	err := execute(t, []string{"agents", "drafts", "create", "--project", "proj", "--brief", "-", "--folder", "trunk"}, draftsCmdDeps(f, "the brief\n", &out))
	if err != nil {
		t.Fatal(err)
	}
	id := strings.TrimSpace(out.String())
	got, err := chatdrafts.New(f.H.StateDir).Get(id)
	if err != nil || got == nil || got.Brief != "the brief" || got.Folder != ops.ChatDraftFolderTrunk {
		t.Fatalf("read draft %q = %+v, %v", id, got, err)
	}
	if got := f.H.Server.Workspaces().Workspace(chatdrafts.MemberKey(id)); got != workspaces.DefaultName {
		t.Errorf("no env var: workspace = %q, want Default", got)
	}
}

func TestAgentsDrafts_CreateFromChatLandsInItsWorkspace(t *testing.T) {
	f := rigs.StartDesigns(t)
	chat := chatIn(t, f, "A")
	t.Setenv(holder.AgentSessionEnv, chat)
	var out bytes.Buffer
	if err := execute(t, []string{"agents", "drafts", "create", "--project", "proj", "--brief", "-"}, draftsCmdDeps(f, "brief", &out)); err != nil {
		t.Fatal(err)
	}
	id := strings.TrimSpace(out.String())
	if got := f.H.Server.Workspaces().Workspace(chatdrafts.MemberKey(id)); got != "A" {
		t.Errorf("draft workspace = %q, want A", got)
	}
}

func TestAgentsDrafts_CreateJSONCarriesID(t *testing.T) {
	f := rigs.StartDesigns(t)
	var out bytes.Buffer
	err := runAgentsDraftsCreate(context.Background(), f.H.Client, &out, true, ops.ChatDraftCreateRequest{Project: "proj", Brief: "b", Folder: ops.ChatDraftFolderTrunk})
	if err != nil {
		t.Fatal(err)
	}
	var res ops.ChatDraftResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil || res.Draft == nil || res.Draft.ID == "" {
		t.Fatalf("--json output = %q, %v", out.String(), err)
	}
}

func TestAgentsDrafts_DeleteSavesAHistoryEntryAndUnknownIDIsRefused(t *testing.T) {
	f := rigs.StartDesigns(t)
	t.Setenv(holder.AgentSessionEnv, "")
	var out bytes.Buffer
	if err := execute(t, []string{"agents", "drafts", "create", "--project", "proj", "--brief", "-", "--folder", "trunk"}, draftsCmdDeps(f, "the brief", &out)); err != nil {
		t.Fatal(err)
	}
	id := strings.TrimSpace(out.String())
	out.Reset()
	if err := execute(t, []string{"agents", "drafts", "delete", id, "--json"}, draftsCmdDeps(f, "", &out)); err != nil {
		t.Fatal(err)
	}
	var res ops.ChatDraftResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil || !res.Deleted {
		t.Fatalf("--json output = %q, %v", out.String(), err)
	}
	items, err := drafthistory.New(f.H.StateDir).List(drafthistory.ChatDrafts)
	if err != nil || len(items) != 1 || items[0].Text != "the brief" {
		t.Fatalf("history = %+v, %v", items, err)
	}
	err = execute(t, []string{"agents", "drafts", "delete", id}, draftsCmdDeps(f, "", &out))
	if err == nil || !strings.Contains(err.Error(), ops.ReasonDraftGone) {
		t.Fatalf("unknown id err = %v, want %s", err, ops.ReasonDraftGone)
	}
	if items, _ := drafthistory.New(f.H.StateDir).List(drafthistory.ChatDrafts); len(items) != 1 {
		t.Errorf("entries after refused delete = %d, want 1", len(items))
	}
}

func TestAgentsDrafts_OversizeBriefIsRefusedAndStoresNothing(t *testing.T) {
	f := rigs.StartDesigns(t)
	var out bytes.Buffer
	t.Setenv(holder.AgentSessionEnv, "")
	big := strings.Repeat("a", wire.MaxSessionPromptBytes+1)
	err := execute(t, []string{"agents", "drafts", "create", "--project", "proj", "--brief", "-", "--folder", "trunk"}, draftsCmdDeps(f, big, &out))
	if err == nil || !strings.Contains(err.Error(), wire.ReasonPromptTextTooLong) {
		t.Fatalf("err = %v, want %s", err, wire.ReasonPromptTextTooLong)
	}
	if out.Len() != 0 {
		t.Errorf("printed %q on a refusal", out.String())
	}
}

func TestAgentsDrafts_EmptyBriefIsRefusedBeforeAnyCall(t *testing.T) {
	d := agentTestDeps(&bytes.Buffer{}, nil)
	d.Env.Stdin = strings.NewReader("  \n")
	d.NewClient = func() (*apiclient.Client, error) { return nil, errors.New("dialed") }
	err := execute(t, []string{"agents", "drafts", "create", "--project", "proj", "--brief", "-"}, d)
	if err == nil || strings.Contains(err.Error(), "dialed") || !strings.Contains(err.Error(), "brief") {
		t.Fatalf("err = %v, want a brief error before dialing", err)
	}
}

func TestAgentsDrafts_MissingProjectIsRefusedByCobra(t *testing.T) {
	d := agentTestDeps(&bytes.Buffer{}, nil)
	err := execute(t, []string{"agents", "drafts", "create", "--brief", "x"}, d)
	if err == nil || !strings.Contains(err.Error(), "project") {
		t.Fatalf("err = %v, want a required-flag error", err)
	}
}

func TestAgentsDrafts_NoServerPrintsRefusal(t *testing.T) {
	cl := apiclient.New("none.sock")
	var out bytes.Buffer
	err := runAgentsDraftsCreate(context.Background(), cl, &out, false, ops.ChatDraftCreateRequest{Project: "p", Brief: "b"})
	if err == nil || err.Error() != "refused (server-not-running)" {
		t.Fatalf("err = %v", err)
	}
	err = runAgentsDraftsCreate(context.Background(), cl, &out, true, ops.ChatDraftCreateRequest{Project: "p", Brief: "b"})
	var exit *cmdbase.ExitError
	if !errors.As(err, &exit) || exit.Code != 1 {
		t.Errorf("--json err = %v, want the JSON refusal exit", err)
	}
}
