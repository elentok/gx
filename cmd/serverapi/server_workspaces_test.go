package serverapi

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

// runWorkspaces runs `gx server workspaces args...` against a live test server.
func runWorkspaces(t *testing.T, h *servertest.Harness, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	d := Deps{Env: cmdbase.Env{
		Stdout:    &out,
		NewClient: func() (*apiclient.Client, error) { return h.Client, nil },
	}}
	err := execute(t, append([]string{"server", "workspaces"}, args...), d)
	return out.String(), err
}

func TestServerWorkspaces_CreateThenClashAndDefaultDeleteRefuse(t *testing.T) {
	h := servertest.Start(t)

	if _, err := runWorkspaces(t, h, "create", "Bugs"); err != nil {
		t.Fatalf("create Bugs: %v", err)
	}
	_, err := runWorkspaces(t, h, "create", "BUGS")
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("create BUGS err = %v, want the name clash", err)
	}
	if _, err := runWorkspaces(t, h, "delete", "Default workspace"); err == nil {
		t.Error("delete of the Default workspace exited zero")
	}

	out, err := runWorkspaces(t, h, "list")
	if err != nil {
		t.Fatal(err)
	}
	if want := "Default workspace\t0\t0 needs you\tdefault\nBugs\t0\t0 needs you\n"; out != want {
		t.Errorf("list =%q, want %q", out, want)
	}
}

func TestServerWorkspaces_RenameAndDelete(t *testing.T) {
	h := servertest.Start(t)
	for _, args := range [][]string{{"create", "Bugs"}, {"rename", "Bugs", "Fixes"}} {
		if _, err := runWorkspaces(t, h, args...); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	out, err := runWorkspaces(t, h, "list")
	if err != nil || !strings.Contains(out, "Fixes\t0\t0 needs you\n") {
		t.Fatalf("list after rename = %q, %v", out, err)
	}
	if _, err := runWorkspaces(t, h, "delete", "Fixes"); err != nil {
		t.Fatal(err)
	}
	if out, _ = runWorkspaces(t, h, "list"); strings.Contains(out, "Fixes") {
		t.Errorf("list after delete = %q, want no Fixes", out)
	}
}

func TestServerWorkspaces_JSONMatchesTheRoute(t *testing.T) {
	h := servertest.Start(t)
	if _, err := runWorkspaces(t, h, "create", "Bugs"); err != nil {
		t.Fatal(err)
	}
	out, err := runWorkspaces(t, h, "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got wire.WorkspaceList
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	want, err := h.Client.Reads.Workspaces(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Workspaces) != len(want.Workspaces) || got.Workspaces[1] != want.Workspaces[1] {
		t.Errorf("--json = %+v, route = %+v", got, want)
	}

	out, err = runWorkspaces(t, h, "create", "bugs", "--json")
	if err != nil {
		t.Fatalf("a refusal under --json is the output, got %v", err)
	}
	var res wire.WorkspaceResult
	if err := json.Unmarshal([]byte(out), &res); err != nil || !res.Refused || res.Reason != wire.ReasonWorkspaceNameTaken {
		t.Errorf("--json refusal = %q (%v)", out, err)
	}
}

func TestServerWorkspaces_SortReadSetAndRefuse(t *testing.T) {
	h := servertest.Start(t)
	if _, err := runWorkspaces(t, h, "create", "Bugs"); err != nil {
		t.Fatal(err)
	}
	out, err := runWorkspaces(t, h, "sort", "Bugs")
	if err != nil || out != wire.SortNeedsYou+"\n" {
		t.Errorf("read = %q (%v), want the default", out, err)
	}
	out, err = runWorkspaces(t, h, "sort", "Bugs", "name")
	if err != nil || out != "name\n" {
		t.Errorf("set = %q (%v), want name", out, err)
	}
	if out, err = runWorkspaces(t, h, "sort", "bugs"); err != nil || out != "name\n" {
		t.Errorf("read back = %q (%v), want name", out, err)
	}

	out, err = runWorkspaces(t, h, "list", "--json")
	var l wire.WorkspaceList
	if err != nil || json.Unmarshal([]byte(out), &l) != nil || l.Workspaces[1].Sort != wire.SortName {
		t.Errorf("list --json = %q (%v), want Bugs sort name", out, err)
	}

	_, err = runWorkspaces(t, h, "sort", "Bugs", "random")
	if err == nil || !strings.Contains(err.Error(), "activity") {
		t.Errorf("unknown value err = %v, want the valid values", err)
	}
	if _, err = runWorkspaces(t, h, "sort", "Nope", "name"); err == nil {
		t.Error("an unknown workspace on set exited zero")
	}
	if _, err = runWorkspaces(t, h, "sort", "Nope"); err == nil {
		t.Error("an unknown workspace on read exited zero")
	}
	if out, _ = runWorkspaces(t, h, "sort", "Bugs"); out != "name\n" {
		t.Errorf("after refusals sort = %q, want name unchanged", out)
	}
}

func TestServerWorkspaces_ListShowsNeedsYouCount(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicketWith(t, store, "proj", "stuck", "01", "broken", servertest.TicketOpts{PauseReason: "iteration-error"})
	h := servertest.StartWithStore(t, store)

	out, err := runWorkspaces(t, h, "list")
	if want := "Default workspace\t0\t1 needs you\tdefault\n"; err != nil || out != want {
		t.Errorf("list = %q (%v), want %q", out, err, want)
	}
	out, err = runWorkspaces(t, h, "list", "--json")
	var got wire.WorkspaceList
	if err != nil || json.Unmarshal([]byte(out), &got) != nil || len(got.Workspaces) != 1 || got.Workspaces[0].NeedsYou != 1 {
		t.Errorf("--json = %q (%v), want needs_you 1", out, err)
	}
}

func TestServerWorkspaces_ReorderMoveRemove(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "plain", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "follow-ups", "01", "first", "")
	h := servertest.StartWithStore(t, store)
	for _, n := range []string{"Bugs", "Ideas"} {
		if _, err := runWorkspaces(t, h, "create", n); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := runWorkspaces(t, h, "reorder", "Ideas", "Default workspace", "Bugs"); err != nil {
		t.Fatalf("reorder: %v", err)
	}
	out, _ := runWorkspaces(t, h, "list")
	if want := "Ideas\t0\t0 needs you\nDefault workspace\t0\t0 needs you\tdefault\nBugs\t0\t0 needs you\n"; out != want {
		t.Errorf("list after reorder = %q, want %q", out, want)
	}
	if _, err := runWorkspaces(t, h, "reorder", "Ideas", "Bugs"); err == nil {
		t.Error("a partial reorder exited zero")
	}

	if _, err := runWorkspaces(t, h, "move", "proj:plain", "Bugs"); err != nil {
		t.Fatalf("move: %v", err)
	}
	out, _ = runWorkspaces(t, h, "list")
	if !strings.Contains(out, "Bugs\t1\t0 needs you\n") {
		t.Errorf("list after move = %q, want Bugs with one member", out)
	}
	for _, key := range []string{"proj:follow-ups", "proj:nope"} {
		if _, err := runWorkspaces(t, h, "move", key, "Bugs"); err == nil {
			t.Errorf("move %s exited zero", key)
		}
	}

	out, err := runWorkspaces(t, h, "move", "proj:follow-ups", "Bugs", "--json")
	var res wire.WorkspaceResult
	if err != nil || json.Unmarshal([]byte(out), &res) != nil || !res.Refused || res.Reason != wire.ReasonWorkspaceNotJoinable {
		t.Errorf("--json refusal = %q (%v)", out, err)
	}

	if _, err := runWorkspaces(t, h, "remove", "proj:plain"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := runWorkspaces(t, h, "remove", "proj:plain"); err != nil {
		t.Errorf("remove of a Default member = %v, want a no-op success", err)
	}
	if out, _ = runWorkspaces(t, h, "list"); !strings.Contains(out, "Bugs\t0\t0 needs you\n") {
		t.Errorf("list after remove = %q, want Bugs empty", out)
	}
}
