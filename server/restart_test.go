package server_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

type restartFixture struct {
	h      *servertest.Harness
	base   string
	output string // the file the fake restart binary prints

	mu     sync.Mutex
	spawns []string
}

func (f *restartFixture) setOutput(t *testing.T, out string) {
	t.Helper()
	if err := os.WriteFile(f.output, []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (f *restartFixture) spawned() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.spawns...)
}

const shownCheck = `{"version":"new-2","config":{"ok":true},"projects":[{"name":"p","ok":true,"files":["a.md"]},{"name":"q","ok":false,"error":"boom","files":[]}]}`

func newRestartFixture(t *testing.T) *restartFixture {
	t.Helper()
	f := &restartFixture{output: filepath.Join(t.TempDir(), "out.json")}
	f.setOutput(t, shownCheck)
	bin := fakeRestartBinary(t, `[ "$1" = restart-check ] || exit 2
cat `+f.output)
	f.h = servertest.StartWithStore(t, t.TempDir(), func(c *server.Config) {
		c.WebAddr = "127.0.0.1:0"
		c.Build = "running-1"
		c.RestartBinary = func() string { return bin }
		c.SpawnRestart = func(b string) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.spawns = append(f.spawns, b)
			return nil
		}
	})
	hs, err := f.h.Client.Handshake(t.Context())
	if err != nil || hs.WebURL == "" {
		t.Fatalf("handshake: %+v, %v", hs, err)
	}
	f.base = hs.WebURL
	return f
}

const confirmedBody = `{"new_version":"new-2","config_ok":true,"failing_projects":["q"]}`

func (f *restartFixture) post(t *testing.T, body string) (int, wire.RestartResult) {
	t.Helper()
	code, out := servertest.WebPostJSON(t, f.base+"/v1/restart", body, servertest.Login(t, f.h.StateDir, f.base), "", servertest.JSONPost(f.base))
	var res wire.RestartResult
	if code == http.StatusOK {
		if err := json.Unmarshal([]byte(out), &res); err != nil {
			t.Fatalf("body %q: %v", out, err)
		}
	}
	return code, res
}

func TestRestart_MatchingValuesStartTheDetachedRestartOnce(t *testing.T) {
	t.Parallel()
	f := newRestartFixture(t)
	code, res := f.post(t, confirmedBody)
	if code != http.StatusOK || !res.Started || res.Refused || res.RestartLog == "" {
		t.Fatalf("code=%d res=%+v", code, res)
	}
	if got := f.spawned(); len(got) != 1 || got[0] == "" {
		t.Errorf("spawns = %v, want one", got)
	}
}

func TestRestart_ChangedValuesAreRefusedWithTheNewOnes(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"version":      `{"version":"new-3","config":{"ok":true},"projects":[{"name":"p","ok":true,"files":[]},{"name":"q","ok":false,"error":"boom","files":[]}]}`,
		"config flip":  `{"version":"new-2","config":{"ok":false,"error":"bad"},"projects":[{"name":"p","ok":true,"files":[]},{"name":"q","ok":false,"error":"boom","files":[]}]}`,
		"dry run flip": `{"version":"new-2","config":{"ok":true},"projects":[{"name":"p","ok":true,"files":[]},{"name":"q","ok":true,"files":[]}]}`,
		"failing set":  `{"version":"new-2","config":{"ok":true},"projects":[{"name":"p","ok":false,"error":"x","files":[]},{"name":"q","ok":false,"error":"boom","files":[]}]}`,
	}
	for name, changed := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newRestartFixture(t)
			f.setOutput(t, changed)
			code, res := f.post(t, confirmedBody)
			if code != http.StatusOK || !res.Refused || res.Reason != wire.ReasonRestartChanged || res.Report == nil {
				t.Fatalf("code=%d res=%+v", code, res)
			}
			if len(f.spawned()) != 0 {
				t.Errorf("spawned on a refusal: %v", f.spawned())
			}
			// The refusal does not block a retry.
			f.setOutput(t, shownCheck)
			if _, res := f.post(t, confirmedBody); !res.Started {
				t.Errorf("retry after refusal = %+v", res)
			}
		})
	}
}

func TestRestart_ErrorTextAndFileListChangesAreNotRefused(t *testing.T) {
	t.Parallel()
	f := newRestartFixture(t)
	f.setOutput(t, `{"version":"new-2","config":{"ok":true},"projects":[{"name":"p","ok":true,"files":["x.md","y.md"]},{"name":"q","ok":false,"error":"other text","files":["z.md"]}]}`)
	if _, res := f.post(t, confirmedBody); !res.Started {
		t.Fatalf("res = %+v", res)
	}
}

func TestRestart_ASecondRequestDuringARestartIsRefused(t *testing.T) {
	t.Parallel()
	f := newRestartFixture(t)
	if _, res := f.post(t, confirmedBody); !res.Started {
		t.Fatalf("first = %+v", res)
	}
	_, res := f.post(t, confirmedBody)
	if !res.Refused || res.Reason != wire.ReasonRestartInProgress || !res.Restarting {
		t.Errorf("second = %+v", res)
	}
	if len(f.spawned()) != 1 {
		t.Errorf("spawns = %v, want one", f.spawned())
	}
}

func TestRestart_UnreadableCheckIsRefusedAndSpawnsNothing(t *testing.T) {
	t.Parallel()
	f := newRestartFixture(t)
	f.setOutput(t, "not json")
	_, res := f.post(t, confirmedBody)
	if !res.Refused || res.Reason != wire.ReasonRestartCheck || len(f.spawned()) != 0 {
		t.Errorf("res = %+v, spawns = %v", res, f.spawned())
	}
}

func TestRestart_NeedsALogin(t *testing.T) {
	t.Parallel()
	f := newRestartFixture(t)
	code, _ := servertest.WebPostJSON(t, f.base+"/v1/restart", confirmedBody, nil, "", servertest.JSONPost(f.base))
	if code != http.StatusUnauthorized {
		t.Errorf("without login = %d, want 401", code)
	}
	if len(f.spawned()) != 0 {
		t.Errorf("spawned without a login: %v", f.spawned())
	}
}
