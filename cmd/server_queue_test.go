package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
)

func TestServerQueueAdd_RefusesWhenNoServerRunning(t *testing.T) {
	cl := apiclient.New(filepath.Join(shortTempDir(t), "a.sock"))
	var out bytes.Buffer
	err := runServerQueueWrite(context.Background(), cl, &out, false,
		func(ctx context.Context, cl *apiclient.Client) (server.QueueResult, error) {
			return cl.QueueAdd(ctx, "proj:epic/01", "")
		})
	if err == nil || !strings.Contains(err.Error(), server.ReasonServerNotRunning) || !strings.Contains(err.Error(), "gx server start") {
		t.Errorf("err = %v; want server-not-running with the start hint", err)
	}
}

// shortTempDir keeps the socket path under the unix sun_path limit, which
// t.TempDir() exceeds on macOS.
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "gx")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}
