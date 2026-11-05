package serverapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/cmd/cmdtest"
	"github.com/elentok/gx/repair"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/ticketops"
	"github.com/elentok/gx/server/wire"
)

func TestServerRepair_IgnoreLiveTabHelpNamesTheAgentSession(t *testing.T) {
	parent := &cobra.Command{Use: "repair"}
	addServerRepairCmds(parent, Deps{})
	merge, _, err := parent.Find([]string{"merge"})
	if err != nil {
		t.Fatal(err)
	}
	f := merge.Flags().Lookup("ignore-live-tab")
	if f == nil {
		t.Fatal("merge has no --ignore-live-tab flag")
	}
	if !strings.Contains(f.Usage, "agent session") || strings.Contains(f.Usage, "herdr") {
		t.Errorf("--ignore-live-tab usage = %q; want it to say \"agent session\", not herdr", f.Usage)
	}
}

func TestServerRepair_ResumeTierFlagReachesTheRequest(t *testing.T) {
	sock := filepath.Join(cmdtest.ShortTempDir(t), "a.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	var got wire.RepairRequest
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/tickets/resume", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(wire.RepairResult{Data: json.RawMessage(`{}`)})
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	parent := &cobra.Command{Use: "tickets"}
	addServerRepairCmds(parent, Deps{Env: cmdbase.Env{Env: cmdtest.EnvFor(t), NewClient: func() (*apiclient.Client, error) { return apiclient.New(sock), nil }}})
	parent.SetArgs([]string{"resume", "proj:epic-a/01", "--tier", "worker"})
	parent.SetOut(&bytes.Buffer{})
	if err := parent.Execute(); err != nil {
		t.Fatal(err)
	}
	if got.Tier != "worker" || got.Address != "proj:epic-a/01" {
		t.Errorf("request = %+v; want tier worker for proj:epic-a/01", got)
	}
}

func TestServerRepairJSON_ReportsServerVia(t *testing.T) {
	cl := apiclient.New(filepath.Join(cmdtest.ShortTempDir(t), "a.sock"))
	var out bytes.Buffer
	if err := runServerRepair(context.Background(), cl, &out, true, "resume", wire.RepairRequest{Address: "p:e/01"},
		func(string, wire.RepairRequest) (wire.RepairResult, error) { return wire.RepairResult{}, nil }); err != nil {
		t.Fatalf("runServerRepair: %v", err)
	}
	got := cmdtest.DecodeJSON(t, out.Bytes())
	if got["via"] != "server" || got["actor"] != "recovery" {
		t.Errorf("via/actor = %v/%v; want server/recovery", got["via"], got["actor"])
	}
}

func TestServerRepair_RunsDirectlyWhenNoServerRunning(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	cl := apiclient.New(filepath.Join(cmdtest.ShortTempDir(t), "a.sock"))
	direct := func(verb string, req wire.RepairRequest) (wire.RepairResult, error) {
		return ticketops.RunRepairDirect(store, verb, req)
	}
	req := wire.RepairRequest{Address: "proj:epic-a/01"}

	var out bytes.Buffer
	err := runServerRepair(context.Background(), cl, &out, true, "resume", req, direct)
	var res wire.RepairResult
	if jerr := json.Unmarshal(out.Bytes(), &res); jerr != nil {
		t.Fatalf("json: %v (%s)", jerr, out.String())
	}
	if err != nil || res.Via != wire.ViaDirect || res.Reason != repair.ReasonNotPaused {
		t.Errorf("res = %+v, err = %v; want direct run refused %s", res, err, repair.ReasonNotPaused)
	}

	out.Reset()
	_ = runServerRepair(context.Background(), cl, &out, false, "verify", wire.RepairRequest{Address: "proj:epic-a", Cwd: t.TempDir()}, direct)
	if !strings.HasPrefix(out.String(), directNotice) {
		t.Errorf("output = %q; want the direct-run notice first", out.String())
	}
}
