package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
)

func TestDesignsOpen_JSON(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "map", "01", "first", "")
	if err := os.WriteFile(filepath.Join(store, "proj", "map", "ticket.md"), []byte("---\nkind: map\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	servertest.SetProjectRepoNoAutoMerge(t, store, "proj", testutil.TempRepo(t))
	h := servertest.StartWithStore(t, store, servertest.WithTestHolder(), servertest.WithChatHarness(string(agentfake.HarnessKind)))

	var first, second server.SessionResult
	for _, res := range []*server.SessionResult{&first, &second} {
		var out bytes.Buffer
		if err := runDesignsOpen(context.Background(), h.Client, &out, true, server.DesignOpenRequest{Epic: "proj:map"}); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(out.Bytes(), res); err != nil {
			t.Fatalf("output %q: %v", out.String(), err)
		}
	}
	if first.Refused || first.Session == nil || second.Session == nil || first.Session.ID != second.Session.ID {
		t.Errorf("open twice = %+v, %+v, want one session", first, second)
	}
}
