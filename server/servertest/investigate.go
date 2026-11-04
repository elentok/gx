package servertest

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/wire"
)

// InvestigateAddr is the investigate ticket forked off ticket 01 of epic-a.
const InvestigateAddr = "proj:epic-a/02"

// RetryAs retries proj:epic-a/01 over the socket as the given investigator.
func RetryAs(t *testing.T, h *Harness, investigator string) wire.QueueResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := apiclient.New(server.SocketPath(h.StateDir)).AsInvestigator(investigator).TicketRetry(ctx, "proj:epic-a/01", false)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// ReadMeta reads a session's meta.json.
func ReadMeta(t *testing.T, h *Harness, id string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(h.SessionsDir, id, "meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	var meta map[string]any
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatal(err)
	}
	return meta
}
