package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
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

// Story 18: --agent overrides for one launch; no flag sends an empty agent.
func TestServerQueueAdd_AgentFlagReachesTheServerRequest(t *testing.T) {
	sock := filepath.Join(shortTempDir(t), "a.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	var got server.QueueRequest
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/queue/add", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(server.QueueResult{})
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	for _, tc := range []struct {
		args []string
		want string
	}{{[]string{"add", "proj:epic/01", "--agent", "codex"}, "codex"}, {[]string{"add", "proj:epic/01"}, ""}} {
		got = server.QueueRequest{}
		c := newServerQueueCmd(deps{env: envFor(t), serverClient: func() (*apiclient.Client, error) { return apiclient.New(sock), nil }})
		c.SetArgs(tc.args)
		c.SetOut(&bytes.Buffer{})
		if err := c.Execute(); err != nil {
			t.Fatal(err)
		}
		if got.Agent != tc.want {
			t.Errorf("%v sent agent %q, want %q", tc.args, got.Agent, tc.want)
		}
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
