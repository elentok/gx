package webhttp

import (
	"bytes"
	"log/slog"
	"testing"
)

// Serving HTTPS makes a wide bind safe to do quietly, so no bind logs a warning.
func TestBind_NeverWarnsAboutAllInterfaces(t *testing.T) {
	t.Parallel()
	for _, addr := range []string{":0", "0.0.0.0:0", "[::]:0", "127.0.0.1:0"} {
		t.Run(addr, func(t *testing.T) {
			var buf bytes.Buffer
			w := New(Config{Addr: addr, StateDir: t.TempDir()}, slog.New(slog.NewTextHandler(&buf, nil)), fakeHost{})
			w.Bind()
			if w.Listener() == nil {
				t.Fatalf("listener did not bind: %q", w.Err())
			}
			t.Cleanup(func() { w.Listener().Close() })
			if bytes.Contains(buf.Bytes(), []byte("all interfaces")) {
				t.Errorf("unexpected warning; log: %s", buf.String())
			}
		})
	}
}
