package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const logFixture = `{"time":"2026-01-02T03:04:05Z","level":"INFO","msg":"server started","pid":7}
{"time":"2026-01-02T03:04:06Z","level":"WARN","msg":"slow","ms":9}
{"time":"2026-01-02T03:04:07Z","level":"ERROR","msg":"boom"}
`

func writeLogFixture(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "server.log")
	if err := os.WriteFile(p, []byte(logFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestServerLogs_PrettyPrints(t *testing.T) {
	var out bytes.Buffer
	if err := runServerLogs(context.Background(), writeLogFixture(t), serverLogsOpts{}, &out); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"INFO  server started pid=7", "WARN  slow ms=9", "ERROR boom"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, `"msg"`) {
		t.Errorf("raw JSON leaked:\n%s", got)
	}
}

func TestServerLogs_FiltersByLevel(t *testing.T) {
	var out bytes.Buffer
	if err := runServerLogs(context.Background(), writeLogFixture(t), serverLogsOpts{Level: "warn"}, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "server started") || !strings.Contains(out.String(), "boom") {
		t.Errorf("bad filter:\n%s", out.String())
	}
}

func TestServerLogs_JSONPassesThrough(t *testing.T) {
	var out bytes.Buffer
	if err := runServerLogs(context.Background(), writeLogFixture(t), serverLogsOpts{JSON: true, Level: "error"}, &out); err != nil {
		t.Fatal(err)
	}
	if want := `{"time":"2026-01-02T03:04:07Z","level":"ERROR","msg":"boom"}` + "\n"; out.String() != want {
		t.Errorf("got %q, want %q", out.String(), want)
	}
}

func TestServerLogs_RejectsBadLevel(t *testing.T) {
	err := runServerLogs(context.Background(), writeLogFixture(t), serverLogsOpts{Level: "loud"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("want error")
	}
}

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.b.Write(p) }
func (s *syncBuf) String() string              { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

func TestServerLogs_FollowPicksUpNewLines(t *testing.T) {
	path := writeLogFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	var out syncBuf
	done := make(chan error, 1)
	go func() { done <- runServerLogs(ctx, path, serverLogsOpts{Follow: true}, &out) }()

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(`{"time":"2026-01-02T03:04:08Z","level":"INFO","msg":"later"}` + "\n"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(out.String(), "later") && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "later") {
		t.Errorf("follow missed new line:\n%s", out.String())
	}
}
