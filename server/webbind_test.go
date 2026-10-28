package server

import (
	"bytes"
	"log/slog"
	"testing"
)

func TestBindWeb_WarnsOnlyWhenBoundToAllInterfaces(t *testing.T) {
	t.Parallel()
	cases := []struct {
		addr string
		warn bool
	}{
		{":0", true},
		{"0.0.0.0:0", true},
		{"[::]:0", true},
		{"127.0.0.1:0", false},
	}
	for _, c := range cases {
		t.Run(c.addr, func(t *testing.T) {
			var buf bytes.Buffer
			s := &Server{
				cfg: Config{WebAddr: c.addr, StateDir: t.TempDir()},
				log: slog.New(slog.NewTextHandler(&buf, nil)),
			}
			s.bindWeb()
			if s.web == nil {
				t.Fatalf("listener did not bind: %q", s.webErr)
			}
			t.Cleanup(func() { s.web.Close() })
			if got := bytes.Contains(buf.Bytes(), []byte("all interfaces")); got != c.warn {
				t.Errorf("warning logged = %v, want %v; log: %s", got, c.warn, buf.String())
			}
		})
	}
}
