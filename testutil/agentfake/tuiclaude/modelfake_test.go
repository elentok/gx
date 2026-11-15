package tuiclaude

import . "github.com/elentok/gx/testutil/agentfake"

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/elentok/gx/commandlist"
	"github.com/elentok/gx/transcript"
)

// modelConfigEnv writes cfg to a commands file and returns the env entry that
// points the fake at it; rewriting the file changes the next query or launch.
func modelConfigEnv(t *testing.T, cfg ProbeConfig) (path, env string) {
	t.Helper()
	path = writeProbeConfig(t, filepath.Join(t.TempDir(), "models.json"), cfg)
	return path, CommandsFileEnv + "=" + path
}

func modelByValue(models []commandlist.Model, value string) (commandlist.Model, bool) {
	for _, m := range models {
		if m.Value == value {
			return m, true
		}
	}
	return commandlist.Model{}, false
}

func TestClaudeProbe_AnswersTheModelQueryAndLogsIt(t *testing.T) {
	cfgPath, _ := modelConfigEnv(t, ProbeConfig{Models: DefaultClaudeModels()})
	env, cwd, _ := probeEnv(t, nil)
	logPath := filepath.Join(t.TempDir(), "queries.log")
	env = append(env, CommandsFileEnv+"="+cfgPath, ModelQueryLogEnv+"="+logPath)

	got, err := commandlist.ProbeClaudeModels(context.Background(), cwd, env)
	if err != nil {
		t.Fatal(err)
	}
	if fable, ok := modelByValue(got, "fable"); !ok || fable.ID != "claude-fable-5-1" {
		t.Errorf("fable = %+v, %v", fable, ok)
	}
	if haiku, ok := modelByValue(got, "haiku"); !ok || len(haiku.Efforts) != 0 {
		t.Errorf("haiku = %+v, %v; want no efforts", haiku, ok)
	}
	if opus, ok := modelByValue(got, "claude-opus-4-6"); !ok || slices.Contains(opus.Efforts, "xhigh") {
		t.Errorf("opus 4.6 = %+v, %v; want no xhigh", opus, ok)
	}

	// A mid-run edit makes the next query fail.
	writeProbeConfig(t, cfgPath, ProbeConfig{Models: DefaultClaudeModels(), ModelsFail: true})
	if _, err := commandlist.ProbeClaudeModels(context.Background(), cwd, env); err == nil {
		t.Fatal("want the query to fail after the edit")
	}
	if q, err := ReadModelQueries(logPath); err != nil || !slices.Equal(q, []string{"claude", "claude"}) {
		t.Fatalf("query log = %v, %v", q, err)
	}
}

func TestCodexProbe_AnswersTheModelQueryOverPagesAndLogsIt(t *testing.T) {
	cfgPath, _ := modelConfigEnv(t, ProbeConfig{Models: DefaultCodexModels()})
	env, cwd, _ := probeEnv(t, nil)
	logPath := filepath.Join(t.TempDir(), "queries.log")
	env = append(env, CodexEnv+"=1", CommandsFileEnv+"="+cfgPath, ModelQueryLogEnv+"="+logPath)

	got, err := commandlist.ProbeCodexModels(context.Background(), cwd, env)
	if err != nil {
		t.Fatal(err)
	}
	if old, ok := modelByValue(got, "gpt-5.4-codex"); !ok || old.Upgrade != "gpt-5.5-codex" {
		t.Errorf("gpt-5.4-codex = %+v, %v; want upgrade gpt-5.5-codex", old, ok)
	}
	if hidden, ok := modelByValue(got, "gpt-5.3-hidden"); !ok || !hidden.Hidden {
		t.Errorf("hidden = %+v, %v", hidden, ok)
	}
	if _, ok := modelByValue(got, "gpt-5.6-codex"); !ok {
		t.Errorf("second page missing from %v", got)
	}

	writeProbeConfig(t, cfgPath, ProbeConfig{Models: DefaultCodexModels(), ModelsFail: true})
	if _, err := commandlist.ProbeCodexModels(context.Background(), cwd, env); err == nil {
		t.Fatal("want the query to fail after the edit")
	}
	q, err := ReadModelQueries(logPath)
	if err != nil || len(q) != 3 || q[0] != "codex" {
		t.Fatalf("query log = %v, %v; want 3 codex queries (two pages, one failure)", q, err)
	}
}

func TestTUIClaude_ReportsTheResolvedIDOfAnAliasAndLogsEveryLaunch(t *testing.T) {
	_, cfgEnv := modelConfigEnv(t, ProbeConfig{Models: DefaultClaudeModels()})
	home := t.TempDir()
	first := append([]string{"--model", "fable"}, tuiLaunchArgs...)
	f := startTUIFakeIn(t, home, first, cfgEnv)
	path := f.transcriptPath()
	f.writeWorkerConfig(WorkerConfig{Chat: true})
	f.typeLine("hello there")
	f.waitStatus(StatusBusy, "")
	f.waitStatus(StatusIdle, "")
	usage, ok, err := transcript.LastAssistantUsage(path)
	if err != nil || !ok || usage.Model != "claude-fable-5-1" {
		t.Fatalf("reported model = %q, %v, %v; want the resolved id", usage.Model, ok, err)
	}
	f.control(Control{Action: ActionExit})
	f.waitExit()

	second := append([]string{"--model", "claude-fable-5-2"}, tuiLaunchArgs...)
	f = startTUIFakeIn(t, home, second, cfgEnv)
	f.waitStatus(StatusIdle, "")

	launches, err := ReadTUILaunches(home, "sess-1")
	if err != nil || len(launches) != 2 || !slices.Equal(launches[0], first) || !slices.Equal(launches[1], second) {
		t.Fatalf("launches = %v, %v", launches, err)
	}
	if latest, err := ReadTUIArgs(home, "sess-1"); err != nil || !slices.Equal(latest, second) {
		t.Fatalf("latest args = %v, %v", latest, err)
	}
}

func TestTUIClaude_ModelBehaviours(t *testing.T) {
	models := []ProbeModel{
		{Value: "gone", Behaviour: ModelExitAtStart},
		{Value: "held", Behaviour: ModelHold},
		{Value: "missing", Behaviour: ModelNotFound},
		{Value: "flaky", Behaviour: ModelError, ErrorCode: "overloaded"},
	}
	_, cfgEnv := modelConfigEnv(t, ProbeConfig{Models: models})
	launch := func(model string) []string { return append([]string{"--model", model}, tuiLaunchArgs...) }

	t.Run("exit at start", func(t *testing.T) {
		f := startTUIFake(t, launch("gone"), cfgEnv)
		if code := f.waitExit(); code != 1 {
			t.Fatalf("exit code = %d, want 1", code)
		}
	})

	t.Run("hold until released", func(t *testing.T) {
		f := startTUIFake(t, launch("held"), cfgEnv)
		f.waitFor("the launch log", func() bool { l, _ := ReadTUILaunches(f.home, "sess-1"); return len(l) == 1 })
		if f.pidFile() != nil {
			t.Fatal("a held launch went idle before its release file")
		}
		if err := os.WriteFile(TUIReleasePath(f.home, "sess-1"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		f.waitStatus(StatusIdle, "")
	})

	for _, tc := range []struct{ model, code string }{
		{"missing", "model_not_found"},
		{"flaky", "overloaded"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			f := startTUIFake(t, launch(tc.model), cfgEnv)
			path := f.transcriptPath()
			f.typeLine("hello there")
			f.waitFor("the error line", func() bool { _, ok, _ := transcript.LastAPIError(path); return ok })
			e, _, err := transcript.LastAPIError(path)
			if err != nil || e.Error != tc.code {
				t.Fatalf("error = %+v, %v; want %s", e, err, tc.code)
			}
			if model := lastAssistantModel(t, path); model != "<synthetic>" {
				t.Fatalf("error line model = %q", model)
			}
			f.waitStatus(StatusIdle, "")
		})
	}
}

func lastAssistantModel(t *testing.T, path string) string {
	t.Helper()
	usage, ok, err := transcript.LastAssistantUsage(path)
	if err != nil || !ok {
		t.Fatalf("LastAssistantUsage = %v, %v", ok, err)
	}
	return usage.Model
}
