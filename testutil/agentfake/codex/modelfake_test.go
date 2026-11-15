package codex

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/testutil/agentfake"
)

func modelConfig(t *testing.T, models []agentfake.ProbeModel) {
	t.Helper()
	raw, err := json.Marshal(agentfake.ProbeConfig{Models: models})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "models.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(agentfake.CommandsFileEnv, path)
}

func modelArgs(model, effort string) []string {
	return []string{"--approve-for-me", "--model", model, "-c", `model_reasoning_effort="` + effort + `"`}
}

func TestCodex_WritesATurnContextWithTheLaunchModelAndEffort(t *testing.T) {
	home := t.TempDir()
	p := startCodex(t, home, "ctx-1", modelArgs("gpt-5.4", "high")...)
	p.turnAndExit("hello")

	got, ok, err := codexsession.ReadTurnModel(rolloutFiles(t, home)[0])
	if err != nil || !ok || got.Model != "gpt-5.4" || got.Effort != "high" || got.At.IsZero() {
		t.Fatalf("turn model = %+v, %v, %v", got, ok, err)
	}
}

func TestCodex_LaunchLogKeepsEveryLaunchOfAConversation(t *testing.T) {
	home := t.TempDir()
	first, second := modelArgs("gpt-5.4", "low"), modelArgs("gpt-5.5", "high")
	p := startCodex(t, home, "log-1", first...)
	p.waitOut("state=idle")
	_ = p.cmd.Process.Kill()
	<-p.done
	p = startCodex(t, home, "log-1", second...)
	p.waitOut("state=idle")

	got, err := ReadCodexLaunches(home, "log-1")
	if err != nil || len(got) != 2 || !slices.Equal(got[0], first) || !slices.Equal(got[1], second) {
		t.Fatalf("launches = %v, %v", got, err)
	}
	if latest, err := ReadCodexArgs(home, "log-1"); err != nil || !slices.Equal(latest, second) {
		t.Fatalf("latest args = %v, %v", latest, err)
	}
}

func TestCodex_ModelBehaviours(t *testing.T) {
	modelConfig(t, []agentfake.ProbeModel{
		{Value: "gone", Behaviour: agentfake.ModelExitAtStart},
		{Value: "held", Behaviour: agentfake.ModelHold},
		{Value: "missing", Behaviour: agentfake.ModelNotFound},
		{Value: "flaky", Behaviour: agentfake.ModelError, ErrorCode: "server_overloaded"},
	})

	t.Run("exit at start", func(t *testing.T) {
		p := startCodex(t, t.TempDir(), "gone-1", modelArgs("gone", "low")...)
		if err := <-p.done; err == nil {
			t.Fatal("want a non-zero exit")
		}
	})

	t.Run("hold until released", func(t *testing.T) {
		home := t.TempDir()
		p := startCodex(t, home, "held-1", modelArgs("held", "low")...)
		p.waitFor(func() bool { l, _ := ReadCodexLaunches(home, "held-1"); return len(l) == 1 }, "the launch log")
		if strings.Contains(p.out.String(), "state=idle") {
			t.Fatal("a held launch went idle before its release file")
		}
		if err := os.WriteFile(CodexReleasePath(home, "held-1"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		p.waitOut("state=idle")
	})

	for _, tc := range []struct{ model, code string }{
		{"missing", "model_not_found"},
		{"flaky", "server_overloaded"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			home := t.TempDir()
			p := startCodex(t, home, "err-"+tc.model, modelArgs(tc.model, "low")...)
			p.waitOut("state=idle")
			if _, err := io.WriteString(p.stdin, "hello\r"); err != nil {
				t.Fatal(err)
			}
			p.waitOut("state=error")
			raw := string(mustRead(t, rolloutFiles(t, home)[0]))
			if !strings.Contains(raw, `"codex_error_info":"`+tc.code+`"`) || !strings.Contains(raw, `"model":"`+tc.model+`"`) {
				t.Fatalf("rollout = %s", raw)
			}
		})
	}
}
