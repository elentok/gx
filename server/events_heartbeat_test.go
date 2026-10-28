package server_test

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

func TestEventsStream_IdleStreamGetsHeartbeatComment(t *testing.T) {
	t.Parallel()
	h := servertest.StartWithStore(t, t.TempDir(), func(c *server.Config) { c.SSEHeartbeat = 20 * time.Millisecond })
	sock := server.SocketPath(h.StateDir)
	hc := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", sock)
		},
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://gx/v1/events?since="+strconv.FormatUint(snap.Seq, 10), nil)
	resp, err := hc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, ":") {
			return
		}
		if strings.HasPrefix(line, "event:") || strings.HasPrefix(line, "data:") {
			t.Fatalf("idle stream carried an event line %q", line)
		}
	}
	t.Fatalf("stream ended without a comment line: %v", sc.Err())
}

func TestFollow_IgnoresHeartbeatComments(t *testing.T) {
	t.Parallel()
	h := servertest.StartWithStore(t, t.TempDir(), func(c *server.Config) { c.SSEHeartbeat = 10 * time.Millisecond })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	evs, err := h.Client.Events(ctx, snap.Seq)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case ev, ok := <-evs:
		if ok {
			t.Fatalf("heartbeat surfaced as event %+v", ev)
		}
		t.Fatal("stream closed on heartbeat")
	case <-time.After(100 * time.Millisecond):
	}
}
