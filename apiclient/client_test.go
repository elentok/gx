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

// followServer serves /v1/snapshot with an incrementing seq and replays
// scripted event streams, one per /v1/events call.
func followServer(t *testing.T, streams [][]server.Event) (*apiclient.Client, *int) {
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
	snaps, evCalls := 0, 0
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/snapshot", func(w http.ResponseWriter, _ *http.Request) {
		snaps++
		_ = json.NewEncoder(w).Encode(server.Snapshot{Seq: uint64(snaps * 10)})
	})
	mux.HandleFunc("GET /v1/events", func(w http.ResponseWriter, r *http.Request) {
		i := evCalls
		evCalls++
		if i >= len(streams) {
			<-r.Context().Done()
			return
		}
		for _, ev := range streams[i] {
			b, _ := json.Marshal(ev)
			_, _ = w.Write([]byte("data: " + string(b) + "\n\n"))
		}
	})
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return apiclient.New(sock), &snaps
}

func TestFollow_ResnapshotsOnStreamEndAndOnSeqGap(t *testing.T) {
	c, snaps := followServer(t, [][]server.Event{
		{{Seq: 11, Type: "ticket-changed", Address: "a"}}, // then the stream drops
		{{Seq: 25, Type: "ticket-changed", Address: "b"}}, // gap: snapshot was at 20
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var got []string
	err := c.Follow(ctx, func(s *server.Snapshot, ev *server.Event) {
		if s != nil {
			got = append(got, "snap")
		} else {
			got = append(got, ev.Address)
		}
		if len(got) == 4 {
			cancel()
		}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Follow = %v, want context.Canceled", err)
	}
	// gap event "b" is never delivered; it forces a third snapshot.
	if want := "snap a snap snap"; strings.Join(got, " ") != want {
		t.Fatalf("got %v, want %q", got, want)
	}
	if *snaps != 3 {
		t.Fatalf("snapshots = %d, want 3", *snaps)
	}
}
