package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/repair"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

func TestServerRepair_RunsDirectlyWhenNoServerRunning(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	cl := apiclient.New(filepath.Join(shortTempDir(t), "a.sock"))
	direct := func(verb string, req server.RepairRequest) (server.RepairResult, error) {
		return server.RunRepairDirect(store, verb, req)
	}
	req := server.RepairRequest{Address: "proj:epic-a/01"}

	var out bytes.Buffer
	err := runServerRepair(context.Background(), cl, &out, true, "unpark", req, direct)
	var res server.RepairResult
	if jerr := json.Unmarshal(out.Bytes(), &res); jerr != nil {
		t.Fatalf("json: %v (%s)", jerr, out.String())
	}
	if err != nil || res.Via != server.ViaDirect || res.Reason != repair.ReasonNotParked {
		t.Errorf("res = %+v, err = %v; want direct run refused %s", res, err, repair.ReasonNotParked)
	}

	out.Reset()
	_ = runServerRepair(context.Background(), cl, &out, false, "verify", server.RepairRequest{Address: "proj:epic-a", Cwd: t.TempDir()}, direct)
	if !strings.HasPrefix(out.String(), directNotice) {
		t.Errorf("output = %q; want the direct-run notice first", out.String())
	}
}
