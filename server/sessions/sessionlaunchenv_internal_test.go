package sessions

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/settings"
)

// startCodexAndReadEnv starts a Codex holder through a stand-in that writes its
// environment to a file, and returns the GX_CODEX_SWITCHED_AT value it saw.
func startCodexAndReadEnv(t *testing.T, meta *Meta, override *launchOverride) string {
	t.Helper()
	dir := t.TempDir()
	// A FIFO makes the read below block until the stand-in has written.
	envFile := filepath.Join(dir, "env.fifo")
	if err := syscall.Mkfifo(envFile, 0o600); err != nil {
		t.Fatal(err)
	}
	set := &settings.Holder{}
	set.Store(&settings.Settings{})
	mg := &Manager{
		settings: set,
		projects: projects.NewRegistry(func() projects.Env { return projects.Env{} }, nil, nil, set, nil, nil),
		cfg:      SessionsConfig{Holder: []string{"/bin/sh", "-c", `env > "$1"`, "sh", envFile}},
	}
	meta.Harness = string(harness.Codex)
	meta.Dir = dir
	if err := mg.startSessionHolderAs(dir, meta, override, 80, 24, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.SplitSeq(string(raw), "\n") {
		if v, ok := strings.CutPrefix(line, harness.CodexSwitchedAtEnv+"="); ok {
			return v
		}
	}
	return ""
}

// A switch launch carries its own time: the saved record still holds the
// previous switch until the new agent is idle.
func TestStartSessionHolderAs_CodexSwitchLaunchGetsTheNewSwitchTime(t *testing.T) {
	t.Parallel()
	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	meta := &Meta{ID: "s", ConversationID: "c", SwitchedAt: old}
	got := startCodexAndReadEnv(t, meta, &launchOverride{Model: "m", SwitchedAt: now})
	if want := now.Format(time.RFC3339Nano); got != want {
		t.Errorf("%s = %q, want %q", harness.CodexSwitchedAtEnv, got, want)
	}
	if !meta.SwitchedAt.Equal(old) {
		t.Errorf("saved SwitchedAt = %v, want the old %v kept", meta.SwitchedAt, old)
	}
}

func TestStartSessionHolderAs_CodexPlainLaunchKeepsTheSavedSwitchTime(t *testing.T) {
	t.Parallel()
	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	meta := &Meta{ID: "s", ConversationID: "c", SwitchedAt: old}
	if got, want := startCodexAndReadEnv(t, meta, nil), old.Format(time.RFC3339Nano); got != want {
		t.Errorf("%s = %q, want %q", harness.CodexSwitchedAtEnv, got, want)
	}
}
