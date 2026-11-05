package lifecycle_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

func fakeRestartBinary(t *testing.T, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gx")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// startRestartCheck starts a web server whose restart binary is bin.
func startRestartCheck(t *testing.T, bin func() string) (*servertest.Harness, string) {
	t.Helper()
	h := servertest.StartWithStore(t, t.TempDir(), func(c *server.Config) {
		c.WebAddr = "127.0.0.1:0"
		c.Build = "running-1"
		c.RestartBinary = bin
	})
	hs, err := h.Client.Handshake(t.Context())
	if err != nil || hs.WebURL == "" {
		t.Fatalf("handshake: %+v, %v", hs, err)
	}
	return h, hs.WebURL
}

func restartCheckOver(t *testing.T, h *servertest.Harness, base string) wire.RestartCheckReport {
	t.Helper()
	code, body := servertest.WebDo(t, h, http.MethodGet, base+"/v1/restart/check")
	if code != http.StatusOK {
		t.Fatalf("GET /v1/restart/check = %d: %s", code, body)
	}
	var rep wire.RestartCheckReport
	if err := json.Unmarshal([]byte(body), &rep); err != nil {
		t.Fatal(err)
	}
	return rep
}

const okCheckScript = `[ "$1" = restart-check ] || exit 2
cat <<'EOF'
{"version":"new-2","config":{"ok":false,"error":"bad key"},"projects":[{"name":"p","ok":true,"files":["a.md"],"pending":3},{"name":"q","ok":false,"error":"boom","files":[]}]}
EOF`

func TestRestartCheck_ReportsVersionsConfigDryRunAgentsInstanceAndLog(t *testing.T) {
	t.Parallel()
	bin := fakeRestartBinary(t, okCheckScript)
	h, base := startRestartCheck(t, func() string { return bin })
	rep := restartCheckOver(t, h, base)
	if rep.Status != wire.RestartCheckOK || rep.RunningVersion != "running-1" || rep.NewVersion != "new-2" {
		t.Errorf("status/versions = %+v", rep)
	}
	if rep.Config.OK || rep.Config.Error != "bad key" {
		t.Errorf("config = %+v", rep.Config)
	}
	if len(rep.Projects) != 2 || !rep.Projects[0].OK || rep.Projects[1].Error != "boom" || rep.Projects[0].Files[0] != "a.md" || rep.Projects[0].Pending != 3 {
		t.Errorf("projects = %+v", rep.Projects)
	}
	if rep.Agents != 0 || rep.Instance == "" || rep.Binary != bin {
		t.Errorf("agents/instance/binary = %d %q %q", rep.Agents, rep.Instance, rep.Binary)
	}
	if want := filepath.Join(h.StateDir, "restart.log"); !strings.HasSuffix(rep.RestartLog, "restart.log") || rep.RestartLog == "" {
		t.Errorf("restart_log = %q, want under state dir (%s)", rep.RestartLog, want)
	}
	if again := restartCheckOver(t, h, base); again.Instance != rep.Instance {
		t.Errorf("instance changed between calls: %q vs %q", rep.Instance, again.Instance)
	}
}

func TestRestartCheck_RunsTheRestartBinaryNotTheServersOwn(t *testing.T) {
	t.Parallel()
	agent := fakeRestartBinary(t, strings.Replace(okCheckScript, "new-2", "agent-9", 1))
	h, base := startRestartCheck(t, func() string { return agent })
	if rep := restartCheckOver(t, h, base); rep.NewVersion != "agent-9" || rep.Binary != agent {
		t.Errorf("report = %+v", rep)
	}
}

func TestRestartCheck_BadBinariesAreThreeDistinctResults(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, status string
		bin          func(t *testing.T) string
	}{
		{"missing", wire.RestartCheckUnreadable, func(t *testing.T) string { return filepath.Join(t.TempDir(), "gone") }},
		{"failing", wire.RestartCheckUnreadable, func(t *testing.T) string { return fakeRestartBinary(t, "echo kaput >&2; exit 1") }},
		{"too old", wire.RestartCheckTooOld, func(t *testing.T) string {
			return fakeRestartBinary(t, `echo 'Error: unknown command "restart-check" for "gx"' >&2; exit 1`)
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			bin := c.bin(t)
			h, base := startRestartCheck(t, func() string { return bin })
			rep := restartCheckOver(t, h, base)
			if rep.Status != c.status || rep.Error == "" || rep.RunningVersion != "running-1" || rep.Instance == "" {
				t.Errorf("report = %+v", rep)
			}
			if hasHint := strings.Contains(rep.Hint, "gx server restart"); hasHint != (c.status == wire.RestartCheckTooOld) {
				t.Errorf("hint = %q", rep.Hint)
			}
			// The old server still answers.
			if _, err := h.Client.Handshake(t.Context()); err != nil {
				t.Errorf("server stopped answering: %v", err)
			}
		})
	}
}

func TestRestartCheck_NeedsALogin(t *testing.T) {
	t.Parallel()
	bin := fakeRestartBinary(t, okCheckScript)
	h, base := startRestartCheck(t, func() string { return bin })
	if code, _ := servertest.WebDoCookie(t, http.MethodGet, base+"/v1/restart/check", nil); code != http.StatusUnauthorized {
		t.Errorf("without login = %d, want 401", code)
	}
	if code, _ := servertest.WebDo(t, h, http.MethodGet, base+"/v1/restart/check"); code != http.StatusOK {
		t.Errorf("with login = %d, want 200", code)
	}
}
