package codexdoctor

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/elentok/gx/harness"
)

func runOne(t *testing.T, name string, rec Recording, env []string) Result {
	t.Helper()
	src := Source{
		Live:         true,
		CodexVersion: "0.166.0",
		Env:          env,
		Session:      func(string) (Recording, error) { return rec, nil },
	}
	return byName(Run(Table, src))[name]
}

func TestLaunchArgs_MatchProductionLaunch(t *testing.T) {
	t.Parallel()
	roots := []string{"/a", "/b"}
	want := codexHarness().Launch(harness.LaunchOptions{
		Args: harness.CodexArgs(harness.CodexArgsInput{WritableRoots: roots}),
	}).Args
	if got := LaunchArgs(roots); !reflect.DeepEqual(got, want) {
		t.Fatalf("LaunchArgs drifted from the production launch:\n%v\n%v", got, want)
	}
}

func TestLaunchFlags(t *testing.T) {
	t.Parallel()
	args := LaunchArgs([]string{"/w"})
	if r := runOne(t, "launch-flags", Recording{Launch: Launch{Args: args}}, nil); r.Status != Pass {
		t.Fatalf("good run: %+v", r)
	}
	bad := Recording{Launch: Launch{Args: args, Err: "exit 2", Stderr: "error: unknown field `sandbox_workspace_write.writable_roots`"}}
	r := runOne(t, "launch-flags", bad, nil)
	if r.Status != Fail || !strings.Contains(r.Detail, "sandbox_workspace_write.writable_roots") {
		t.Fatalf("bad run: %+v", r)
	}
}

func TestOSC7501Title(t *testing.T) {
	t.Parallel()
	good := Recording{Frames: []Frame{{Data: []byte("\x1b]0;⠋ gx-codex-rec\a\x1b]7501;state=working:app=codex\a")}}}
	if r := runOne(t, "osc7501-title", good, nil); r.Status != Pass {
		t.Fatalf("good run: %+v", r)
	}
	noOSC := Recording{Frames: []Frame{{Data: []byte("\x1b]0;gx-codex-rec\a")}}}
	if r := runOne(t, "osc7501-title", noOSC, nil); r.Status != Fail || !strings.Contains(r.Detail, "7501") {
		t.Fatalf("no OSC: %+v", r)
	}
	noTitle := Recording{Frames: []Frame{{Data: []byte("\x1b]7501;state=idle:app=codex\a")}}}
	if r := runOne(t, "osc7501-title", noTitle, nil); r.Status != Fail || !strings.Contains(r.Detail, "title") {
		t.Fatalf("no title: %+v", r)
	}
}

func TestHooksFire(t *testing.T) {
	t.Parallel()
	if r := runOne(t, "hooks-fire", Recording{Hooks: []string{"SessionStart", "PreToolUse"}}, nil); r.Status != Pass {
		t.Fatalf("good run: %+v", r)
	}
	r := runOne(t, "hooks-fire", Recording{Hooks: []string{"SessionStart"}}, nil)
	if r.Status != Fail || !strings.Contains(r.Detail, "PreToolUse") {
		t.Fatalf("silent hook: %+v", r)
	}
}

func TestPermissionDeny(t *testing.T) {
	t.Parallel()
	if r := runOne(t, "permission-deny", Recording{Hooks: []string{"PermissionRequest"}, DenyHonoured: true}, nil); r.Status != Pass {
		t.Fatalf("good run: %+v", r)
	}
	r := runOne(t, "permission-deny", Recording{Hooks: []string{"PermissionRequest"}}, nil)
	if r.Status != Fail || !strings.Contains(r.Detail, "ignored the deny") {
		t.Fatalf("deny ignored: %+v", r)
	}
	if r := runOne(t, "permission-deny", Recording{}, nil); r.Status != Fail {
		t.Fatalf("hook silent: %+v", r)
	}
}

func TestProbeCommit(t *testing.T) {
	t.Parallel()
	if r := runOne(t, "probe-commit", Recording{}, nil); r.Status != Pass {
		t.Fatalf("good run: %+v", r)
	}
	r := runOne(t, "probe-commit", Recording{Probe: Probe{Err: "index.lock: read-only"}}, nil)
	if r.Status != Fail || !strings.Contains(r.Detail, "index.lock") {
		t.Fatalf("refused: %+v", r)
	}
}

func TestRolloutReader(t *testing.T) {
	t.Parallel()
	good, err := os.ReadFile("testdata/rollouts/plain-turn.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if r := runOne(t, "rollout-reader", Recording{Rollouts: []Rollout{{Name: "x", Data: good}}}, nil); r.Status != Pass {
		t.Fatalf("good run: %+v", r)
	}
	if r := runOne(t, "rollout-reader", Recording{Rollouts: []Rollout{{Name: "x", Data: []byte("not json\n")}}}, nil); r.Status != Fail {
		t.Fatalf("bad rollout: %+v", r)
	}
	if r := runOne(t, "rollout-reader", Recording{}, nil); r.Status != Fail {
		t.Fatalf("no rollout: %+v", r)
	}
}

func TestLiveChecksDependOnCodexVersion(t *testing.T) {
	t.Parallel()
	for _, c := range Table {
		if c.Mode == Live && !c.Warn && c.Name != "codex-version" && len(c.DependsOn) != 1 {
			t.Errorf("%s must depend on codex-version", c.Name)
		}
	}
}

func codexHome(t *testing.T, config string, rules bool) []string {
	t.Helper()
	home := t.TempDir()
	if config != "" {
		if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(config), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if rules {
		if err := os.MkdirAll(filepath.Join(home, "rules"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(home, "rules", "gx.rules"), []byte("# gx\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return []string{"CODEX_HOME=" + home}
}

func TestWarnings(t *testing.T) {
	t.Parallel()
	hidden := "[features.multi_agent_v2]\nhide_spawn_agent_metadata = true\n"
	r := runOne(t, "spawn-model-fields", Recording{}, codexHome(t, hidden, true))
	if r.Status != Warn || !strings.Contains(r.Detail, "hide_spawn_agent_metadata") {
		t.Fatalf("hidden fields: %+v", r)
	}
	off := "[features.multi_agent_v2]\nexpose_spawn_agent_model_overrides = false\n"
	if r := runOne(t, "spawn-model-fields", Recording{}, codexHome(t, off, true)); r.Status != Warn {
		t.Fatalf("overrides off: %+v", r)
	}
	r = runOne(t, "managed-rules", Recording{}, codexHome(t, "", false))
	if r.Status != Warn || !strings.Contains(r.Detail, "gx codex setup") {
		t.Fatalf("no rules: %+v", r)
	}

	env := codexHome(t, "[features.multi_agent_v2]\nhide_spawn_agent_metadata = false\n", true)
	for _, name := range []string{"spawn-model-fields", "managed-rules"} {
		if r := runOne(t, name, Recording{}, env); r.Status != Pass {
			t.Fatalf("%s should be quiet: %+v", name, r)
		}
	}
}
