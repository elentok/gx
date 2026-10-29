package server

import (
	"bytes"
	"log/slog"
	"testing"
)

// Serving HTTPS makes a wide bind safe to do quietly, so no bind logs a warning.
func TestBindWeb_NeverWarnsAboutAllInterfaces(t *testing.T) {
	t.Parallel()
	for _, addr := range []string{":0", "0.0.0.0:0", "[::]:0", "127.0.0.1:0"} {
		t.Run(addr, func(t *testing.T) {
			var buf bytes.Buffer
			s := &Server{
				cfg: Config{WebAddr: addr, StateDir: t.TempDir()},
				log: slog.New(slog.NewTextHandler(&buf, nil)),
			}
			s.bindWeb()
			if s.web == nil {
				t.Fatalf("listener did not bind: %q", s.webErr)
			}
			t.Cleanup(func() { s.web.Close() })
			if bytes.Contains(buf.Bytes(), []byte("all interfaces")) {
				t.Errorf("unexpected warning; log: %s", buf.String())
			}
		})
	}
}
