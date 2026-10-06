package apiclient_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
)

// fakeServer answers /v1/handshake with h on a temp unix socket.
func fakeServer(t *testing.T, h server.Handshake) *apiclient.Client {
	t.Helper()
	dir, err := os.MkdirTemp("", "gxc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "s.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/handshake", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(h)
	})
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return apiclient.New(sock)
}

func TestNegotiate_MatchingBuildHasNoHintAndAllowsWrites(t *testing.T) {
	c := fakeServer(t, server.Handshake{APIVersion: server.APIVersion, Build: "b1"})

	n, err := c.Negotiate(context.Background(), "b1")
	if err != nil {
		t.Fatal(err)
	}
	if n.Hint != "" || n.ReadOnly {
		t.Errorf("negotiation = %+v", n)
	}
	if err := c.CheckWrite(); err != nil {
		t.Errorf("CheckWrite = %v", err)
	}
}

func TestNegotiate_BuildMismatchOnlyHints(t *testing.T) {
	c := fakeServer(t, server.Handshake{APIVersion: server.APIVersion, Build: "old"})

	n, err := c.Negotiate(context.Background(), "new")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(n.Hint, "older build") || n.ReadOnly {
		t.Errorf("negotiation = %+v", n)
	}
	if err := c.CheckWrite(); err != nil {
		t.Errorf("CheckWrite = %v", err)
	}
}

func TestNegotiate_APIVersionMismatchGoesReadOnly(t *testing.T) {
	c := fakeServer(t, server.Handshake{APIVersion: server.APIVersion + 1, Build: "x"})

	n, err := c.Negotiate(context.Background(), "x")
	if err != nil {
		t.Fatal(err)
	}
	if !n.ReadOnly || !strings.Contains(n.Hint, "gx server restart") {
		t.Errorf("negotiation = %+v", n)
	}
	if err := c.CheckWrite(); !errors.Is(err, apiclient.ErrReadOnly) {
		t.Errorf("CheckWrite = %v, want ErrReadOnly", err)
	}
}
