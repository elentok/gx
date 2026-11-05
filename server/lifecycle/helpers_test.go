package lifecycle_test

import (
	"context"
	"github.com/elentok/gx/server/servertest/rigs"
	"os/exec"
	"strings"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

// Short names for the helpers that live in servertest because other packages'
// tests use them too.
const schedEpic = servertest.SchedEpic

var (
	commitWork = servertest.CommitWork
	runLogOf   = rigs.RunLogOf
	waitFor    = servertest.Await
)

type schedFixture struct {
	*servertest.SchedFixture
}

func newSchedFixture(t *testing.T, tickets map[string]servertest.TicketOpts, agentTurn func(f *schedFixture, p *servertest.Turn, id string), cfgOpts ...func(*server.Config)) *schedFixture {
	t.Helper()
	f := &schedFixture{}
	var inner func(*servertest.SchedFixture, *servertest.Turn, string)
	if agentTurn != nil {
		inner = func(_ *servertest.SchedFixture, p *servertest.Turn, id string) { agentTurn(f, p, id) }
	}
	f.SchedFixture = servertest.NewSchedFixture(t, tickets, inner, cfgOpts...)
	return f
}

// reload asks the running server to reload its config and returns the result.
func reload(t *testing.T, h *servertest.Harness) wire.ReloadResult {
	t.Helper()
	res, err := h.Client.Admin.ReloadConfig(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func storeLog(t *testing.T, store string) string {
	t.Helper()
	out, _ := exec.Command("git", "-C", store, "log", "--format=%s").Output()
	return strings.TrimSpace(string(out))
}
