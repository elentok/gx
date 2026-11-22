package projects

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server/settings"
)

func projectFileFixture(t *testing.T) (*Registry, func(harness string, mtime time.Time), *atomic.Int32) {
	t.Helper()
	store := t.TempDir()
	projectDir := filepath.Join(store, "proj")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	pjson := filepath.Join(projectDir, config.ProjectFileName)
	write := func(harness string, mtime time.Time) {
		t.Helper()
		body := `{"agents":{"default-harness":"` + harness + `"}}`
		if err := os.WriteFile(pjson, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(pjson, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
	var reads atomic.Int32
	orig := readProjectFile
	readProjectFile = func(dir string) (config.ProjectFile, error) {
		reads.Add(1)
		return orig(dir)
	}
	t.Cleanup(func() { readProjectFile = orig })

	hold := &settings.Holder{}
	hold.Store(&settings.Settings{AgentsDefaultHarness: "global"})
	reg := &Registry{cfg: func() Env { return Env{TicketStore: store} }, settings: hold}
	return reg, write, &reads
}

func TestAgentsConfig_ParsesUnchangedProjectFileOnce(t *testing.T) {
	reg, write, reads := projectFileFixture(t)
	write("codex", time.Now())

	for range 5 {
		if got := reg.AgentsConfig("proj").DefaultHarness; got != "codex" {
			t.Fatalf("DefaultHarness = %q, want codex", got)
		}
		_ = reg.CIFixSkills("proj")
		_ = reg.PRTextSkill("proj")
	}
	if n := reads.Load(); n != 1 {
		t.Fatalf("project.json parsed %d times, want 1", n)
	}
}

func TestAgentsConfig_ReparsesAfterProjectFileChanges(t *testing.T) {
	reg, write, reads := projectFileFixture(t)
	base := time.Now()
	write("codex", base)
	if got := reg.AgentsConfig("proj").DefaultHarness; got != "codex" {
		t.Fatalf("DefaultHarness = %q, want codex", got)
	}
	write("claude", base.Add(time.Minute))
	if got := reg.AgentsConfig("proj").DefaultHarness; got != "claude" {
		t.Fatalf("after change DefaultHarness = %q, want claude", got)
	}
	if n := reads.Load(); n != 2 {
		t.Fatalf("project.json parsed %d times, want 2", n)
	}
}

func TestAgentsConfig_MissingOrInvalidProjectFileUsesGlobal(t *testing.T) {
	reg, write, _ := projectFileFixture(t)
	if got := reg.AgentsConfig("proj").DefaultHarness; got != "global" {
		t.Fatalf("missing file: DefaultHarness = %q, want global", got)
	}
	write("codex", time.Now())
	pjson := filepath.Join(reg.cfg().TicketStore, "proj", config.ProjectFileName)
	if err := os.WriteFile(pjson, []byte(`{not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := reg.AgentsConfig("proj").DefaultHarness; got != "global" {
		t.Fatalf("invalid file: DefaultHarness = %q, want global", got)
	}
}
