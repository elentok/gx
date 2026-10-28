package server_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/server"
)

func TestOneOff_RemovedFileKeysAreRefusedAndNothingIsWritten(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"commits", "base"} {
		t.Run(key, func(t *testing.T) {
			h, repo := startOneOffHarness(t)
			ctx := context.Background()
			file := filepath.Join(t.TempDir(), "payload.md")
			if err := os.WriteFile(file, []byte("---\n"+key+": main\n---\nbuild it\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			res, err := h.Client.OneOff(ctx, server.OneOffRequest{File: file, Cwd: repo, Name: "refused-job"})
			if err != nil || !res.Refused || res.Reason != server.ReasonOptionRemoved {
				t.Fatalf("got %+v, %v; want refusal %s", res, err, server.ReasonOptionRemoved)
			}
			if !strings.Contains(res.Message, key) || !strings.Contains(res.Message, "never commits") {
				t.Errorf("message %q should name %q and say one-offs never commit", res.Message, key)
			}
			if q, _ := h.Client.QueueItems(ctx); len(q) != 0 {
				t.Errorf("queue = %+v; want empty", q)
			}
			if _, err := os.Stat(filepath.Join(h.TicketStore, "proj", "refused-job")); err == nil {
				t.Error("a refused submit left an epic behind")
			}
		})
	}
}
