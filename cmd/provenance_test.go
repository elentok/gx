package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
)

func decodeJSON(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("output %q is not a JSON object: %v", b, err)
	}
	return m
}

func TestTicketsSetJSON_DirectHuman(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "04-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04\"\nstatus: open\ntype: implement\n---\nBody.\n")

	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil), getwd: nonAgentGetwd(t)}
	if err := execute([]string{"tickets", "set", path, "--type=implement", "--json"}, d); err != nil {
		t.Fatalf("execute: %v", err)
	}
	got := decodeJSON(t, stdout.Bytes())
	if got["via"] != "direct" || got["actor"] != "human" {
		t.Errorf("via/actor = %v/%v; want direct/human", got["via"], got["actor"])
	}
}

func TestTicketsSetJSON_IterationStatusFromAgent(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "04-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04\"\nstatus: claimed\ntype: implement\n---\nBody.\n")

	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil), getwd: agentGetwd(t, "ralph-loop/epic")}
	if err := execute([]string{"tickets", "set", path, "--iteration-status=finished", "--json"}, d); err != nil {
		t.Fatalf("execute: %v", err)
	}
	got := decodeJSON(t, stdout.Bytes())
	if got["via"] != "direct" || got["actor"] != "agent" {
		t.Errorf("via/actor = %v/%v; want direct/agent", got["via"], got["actor"])
	}
}

func TestRecoveryJSON_DirectRecoveryOnSuccessAndRefusal(t *testing.T) {
	t.Parallel()
	for name, runErr := range map[string]error{"success": nil, "refusal": &RefusalError{Reason: ReasonNotParked, Message: "no"}} {
		var out bytes.Buffer
		_ = finishRecovery(&out, &bytes.Buffer{}, true, struct{}{}, "", runErr)
		got := decodeJSON(t, out.Bytes())
		if got["via"] != "direct" || got["actor"] != "recovery" {
			t.Errorf("%s: via/actor = %v/%v; want direct/recovery", name, got["via"], got["actor"])
		}
	}
}

func TestServerRepairJSON_ReportsServerVia(t *testing.T) {
	cl := apiclient.New(filepath.Join(shortTempDir(t), "a.sock"))
	var out bytes.Buffer
	if err := runServerRepair(context.Background(), cl, &out, true, "unpark", server.RepairRequest{Address: "p:e/01"}); err != nil {
		t.Fatalf("runServerRepair: %v", err)
	}
	got := decodeJSON(t, out.Bytes())
	if got["via"] != "server" || got["actor"] != "recovery" {
		t.Errorf("via/actor = %v/%v; want server/recovery", got["via"], got["actor"])
	}
}

func TestServerQueueWriteJSON_ReportsServerVia(t *testing.T) {
	cl := apiclient.New(filepath.Join(shortTempDir(t), "a.sock"))
	var out bytes.Buffer
	err := runServerQueueWrite(context.Background(), cl, &out, true,
		func(ctx context.Context, cl *apiclient.Client) (server.QueueResult, error) {
			return cl.QueueAdd(ctx, "p:e/01", "")
		})
	if err != nil {
		t.Fatalf("runServerQueueWrite: %v", err)
	}
	got := decodeJSON(t, out.Bytes())
	if got["via"] != "server" || got["actor"] == nil {
		t.Errorf("via/actor = %v/%v; want server/<actor>", got["via"], got["actor"])
	}
}
