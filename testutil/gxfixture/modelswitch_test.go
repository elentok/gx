package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/testutil/agentfake/codex"
	"github.com/elentok/gx/testutil/gxfixture/spec"
)

// waitIdle waits until the session is Idle and, when launches is positive,
// its launch log has that many entries.
func (r *running) waitIdle(t *testing.T, id string, launches int, read func() ([][]string, error)) [][]string {
	t.Helper()
	var got [][]string
	testutil.WaitFor(t, 30*time.Second, fmt.Sprintf("session %s idle after %d launches", id, launches), func() bool {
		got, _ = read()
		return len(got) == launches && slices.ContainsFunc(r.sessions(t), func(s wire.SessionInfo) bool {
			return s.ID == id && s.Status.State == wire.SessionIdle
		})
	})
	return got
}

// conversationID is the id the fakes key their launch logs by, which differs
// from the session's own id.
func (r *running) conversationID(t *testing.T, sessionID string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(r.Root(), "data", "gx", "sessions", sessionID, "meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	var meta struct {
		ConversationID string `json:"conversation_id"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil || meta.ConversationID == "" {
		t.Fatalf("meta of %s: conversation id %q, %v", sessionID, meta.ConversationID, err)
	}
	return meta.ConversationID
}

func flagValue(args []string, flag string) string {
	if i := slices.Index(args, flag); i >= 0 && i+1 < len(args) {
		return args[i+1]
	}
	return ""
}

// setBehaviour rewrites the fakes' config so the model named value behaves as
// behaviour says, over the scenario's lists.
func (r *running) setBehaviour(t *testing.T, value, behaviour string) {
	t.Helper()
	models := spec.ModelSwitchScenario().Commands.Models
	for i := range models {
		if models[i].Value == value {
			models[i].Behaviour = behaviour
		}
	}
	raw, err := json.Marshal(agentfake.ProbeConfig{Models: models})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.ModelFakes.FakeConfig, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestModelSwitchScenario_SeedsTheSessionsAndNamesTheFakeFiles(t *testing.T) {
	t.Parallel()
	r := start(t, "model-switch")
	if r.ModelFakes == nil {
		t.Fatal("output has no model_fakes")
	}
	for name, path := range map[string]string{"fake config": r.ModelFakes.FakeConfig, "claude dir": r.ModelFakes.ClaudeDir, "codex dir": r.ModelFakes.CodexDir} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	var claudeChats, codexChats, design int
	for _, s := range r.sessions(t) {
		switch {
		case s.Design:
			design++
		case s.Harness == "codex":
			codexChats++
		default:
			claudeChats++
		}
		if s.Status.State != wire.SessionIdle && !s.Design {
			t.Errorf("session %s is %s, want idle", s.ID, s.Status.State)
		}
	}
	if claudeChats != 2 || codexChats != 1 || design != 1 {
		t.Errorf("claude chats = %d, codex chats = %d, design sessions = %d; want 2, 1, 1", claudeChats, codexChats, design)
	}
	r.Stop(t)
}

// Story 35: an effort the new model lacks is replaced, so the launch is valid.
func TestModelSwitchScenario_ReplacesAnEffortTheModelLacks(t *testing.T) {
	t.Parallel()
	r := start(t, "model-switch")
	var claudeID, codexID string
	for _, s := range r.sessions(t) {
		switch {
		case s.Design:
		case s.Harness == "codex":
			codexID = s.ID
		case claudeID == "":
			claudeID = s.ID
		}
	}
	home := r.Home()
	claudeConv, codexConv := r.conversationID(t, claudeID), r.conversationID(t, codexID)
	claudeLaunches := func() ([][]string, error) { return agentfake.ReadTUILaunches(home, claudeConv) }
	codexLaunches := func() ([][]string, error) { return codex.ReadCodexLaunches(filepath.Join(home, ".codex"), codexConv) }

	// switchEffort is the effort the server says a switch to model would run
	// with, given the session's current effort.
	switchEffort := func(t *testing.T, id, model string) string {
		t.Helper()
		var res struct {
			Models []struct {
				ID           string `json:"id"`
				SwitchEffort string `json:"switch_effort"`
			} `json:"models"`
		}
		if err := json.Unmarshal([]byte(r.Gx(t, "agents", "models", id, "--json")), &res); err != nil {
			t.Fatal(err)
		}
		for _, m := range res.Models {
			if m.ID == model {
				return m.SwitchEffort
			}
		}
		t.Fatalf("models of %s has no row for %s", id, model)
		return ""
	}

	t.Run("claude takes the nearest lower level", func(t *testing.T) {
		r.waitIdle(t, claudeID, 1, claudeLaunches)
		// opus lists xhigh; claude-opus-4-6 lists low, medium and high only.
		r.Gx(t, "agents", "switch", claudeID, "opus", "--effort", "xhigh")
		r.waitIdle(t, claudeID, 2, claudeLaunches)
		want := switchEffort(t, claudeID, "claude-opus-4-6")
		if want != "high" {
			t.Fatalf("switch effort = %q, want high", want)
		}
		r.Gx(t, "agents", "switch", claudeID, "claude-opus-4-6", "--effort", want)
		launches := r.waitIdle(t, claudeID, 3, claudeLaunches)
		if got := flagValue(launches[2], "--effort"); got != "high" {
			t.Fatalf("switch launch = %v, want --effort high", launches[2])
		}
	})

	t.Run("codex takes the model default", func(t *testing.T) {
		r.waitIdle(t, codexID, 1, codexLaunches)
		// The CLI sends the effort as given, so xhigh puts the session on an
		// effort gpt-6-mini (low, medium, high; default low) does not list.
		r.Gx(t, "agents", "switch", codexID, "gpt-5.5-codex", "--effort", "xhigh")
		r.waitIdle(t, codexID, 2, codexLaunches)
		want := switchEffort(t, codexID, "gpt-6-mini")
		if want != "low" {
			t.Fatalf("switch effort = %q, want low", want)
		}
		r.Gx(t, "agents", "switch", codexID, "gpt-6-mini", "--effort", want)
		launches := r.waitIdle(t, codexID, 3, codexLaunches)
		if got := strings.Join(launches[2], " "); !strings.Contains(got, `model_reasoning_effort="low"`) {
			t.Fatalf("switch launch = %v, want effort low", launches[2])
		}
	})
	r.Stop(t)
}

func TestModelSwitchScenario_SwitchesAndRollsBackEachKind(t *testing.T) {
	t.Parallel()
	r := start(t, "model-switch")
	var claudeID, codexID string
	for _, s := range r.sessions(t) {
		switch {
		case s.Design:
		case s.Harness == "codex":
			codexID = s.ID
		case claudeID == "":
			// The sessions list in creation order: the first Claude chat has history.
			claudeID = s.ID
		}
	}
	if claudeID == "" || codexID == "" {
		t.Fatalf("claude = %q, codex = %q; want both", claudeID, codexID)
	}
	home := r.Home()
	claudeConv, codexConv := r.conversationID(t, claudeID), r.conversationID(t, codexID)
	claudeLaunches := func() ([][]string, error) { return agentfake.ReadTUILaunches(home, claudeConv) }
	codexLaunches := func() ([][]string, error) { return codex.ReadCodexLaunches(filepath.Join(home, ".codex"), codexConv) }

	t.Run("claude", func(t *testing.T) {
		launches := r.waitIdle(t, claudeID, 1, claudeLaunches)
		if got := flagValue(launches[0], "--model"); got != spec.LaggingClaudeAlias {
			t.Fatalf("first launch --model = %q, want %q", got, spec.LaggingClaudeAlias)
		}
		// A model the harness refuses on the first message rolls the switch back.
		r.setBehaviour(t, "opus", agentfake.ModelNotFound)
		r.Gx(t, "agents", "switch", claudeID, "opus", "--effort", "low")
		launches = r.waitIdle(t, claudeID, 2, claudeLaunches)
		if flagValue(launches[1], "--model") != "opus" || flagValue(launches[1], "--effort") != "low" {
			t.Fatalf("switch launch = %v, want --model opus --effort low", launches[1])
		}
		r.Gx(t, "agents", "prompt", claudeID, "first message")
		launches = r.waitIdle(t, claudeID, 3, claudeLaunches)
		if flagValue(launches[2], "--model") != spec.LaggingClaudeAlias || flagValue(launches[2], "--effort") != "medium" {
			t.Fatalf("rollback launch = %v, want the old --model %s --effort medium", launches[2], spec.LaggingClaudeAlias)
		}

		r.Gx(t, "agents", "switch", claudeID, "claude-fable-5-2", "--effort", "high")
		launches = r.waitIdle(t, claudeID, 4, claudeLaunches)
		if flagValue(launches[3], "--model") != "claude-fable-5-2" || flagValue(launches[3], "--effort") != "high" {
			t.Fatalf("switch launch = %v, want --model claude-fable-5-2 --effort high", launches[3])
		}
	})

	t.Run("codex", func(t *testing.T) {
		launches := r.waitIdle(t, codexID, 1, codexLaunches)
		if got := flagValue(launches[0], "--model"); got != spec.OutdatedCodexModel {
			t.Fatalf("first launch --model = %q, want %q", got, spec.OutdatedCodexModel)
		}
		r.setBehaviour(t, "gpt-6-mini", agentfake.ModelNotFound)
		r.Gx(t, "agents", "switch", codexID, "gpt-6-mini", "--effort", "low")
		launches = r.waitIdle(t, codexID, 2, codexLaunches)
		if flagValue(launches[1], "--model") != "gpt-6-mini" || !strings.Contains(strings.Join(launches[1], " "), `model_reasoning_effort="low"`) {
			t.Fatalf("switch launch = %v, want --model gpt-6-mini and effort low", launches[1])
		}
		r.Gx(t, "agents", "prompt", codexID, "first message")
		launches = r.waitIdle(t, codexID, 3, codexLaunches)
		if flagValue(launches[2], "--model") != spec.OutdatedCodexModel || !strings.Contains(strings.Join(launches[2], " "), `model_reasoning_effort="medium"`) {
			t.Fatalf("rollback launch = %v, want the old --model %s and effort medium", launches[2], spec.OutdatedCodexModel)
		}

		r.Gx(t, "agents", "switch", codexID, "gpt-5.5-codex", "--effort", "high")
		launches = r.waitIdle(t, codexID, 4, codexLaunches)
		if flagValue(launches[3], "--model") != "gpt-5.5-codex" || !strings.Contains(strings.Join(launches[3], " "), `model_reasoning_effort="high"`) {
			t.Fatalf("switch launch = %v, want --model gpt-5.5-codex and effort high", launches[3])
		}
	})
	r.Stop(t)
}
