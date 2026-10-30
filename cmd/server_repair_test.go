package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/repair"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

func TestServerRepair_IgnoreLiveTabHelpNamesTheAgentSession(t *testing.T) {
	parent := &cobra.Command{Use: "repair"}
	addServerRepairCmds(parent, defaultDeps())
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

func TestServerRepair_RunsDirectlyWhenNoServerRunning(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	cl := apiclient.New(filepath.Join(shortTempDir(t), "a.sock"))
	direct := func(verb string, req server.RepairRequest) (server.RepairResult, error) {
		return server.RunRepairDirect(store, verb, req)
	}
	req := server.RepairRequest{Address: "proj:epic-a/01"}

	var out bytes.Buffer
	err := runServerRepair(context.Background(), cl, &out, true, "resume", req, direct)
	var res server.RepairResult
	if jerr := json.Unmarshal(out.Bytes(), &res); jerr != nil {
		t.Fatalf("json: %v (%s)", jerr, out.String())
	}
	if err != nil || res.Via != server.ViaDirect || res.Reason != repair.ReasonNotPaused {
		t.Errorf("res = %+v, err = %v; want direct run refused %s", res, err, repair.ReasonNotPaused)
	}

	out.Reset()
	_ = runServerRepair(context.Background(), cl, &out, false, "verify", server.RepairRequest{Address: "proj:epic-a", Cwd: t.TempDir()}, direct)
	if !strings.HasPrefix(out.String(), directNotice) {
		t.Errorf("output = %q; want the direct-run notice first", out.String())
	}
}
