package ralphloop

import (
	"fmt"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/herdrfake"
)

// TestRun_ProductionRealGit_AgentStartCarriesModelAndEffort drives ticket
// 04b's process-boundary seam: unlike loop_agent_test.go's
// TestRun_AgentSelection_ConfiguresLaunchAndPrompt, which only asserts the
// in-process Deps.AgentStart function fake receives the model/effort flags,
// this test asserts they survive all the way to the real `herdr agent start`
// argv the fake `herdr` executable (testutil/herdrfake) receives — the
// actual subprocess boundary herdr.AgentStart (herdr/agent.go) shells out
// across. `herdr agent start` fails immediately after capturing argv, which
// is enough to observe the flags without driving a full iteration to
// completion.
func TestRun_ProductionRealGit_AgentStartCarriesModelAndEffort(t *testing.T) {
	// not parallel-safe: herdrfake.StartState calls t.Setenv for the helper
	// socket path and PATH.
	const epicName = "epic"
	for _, tc := range []struct {
		name          string
		agent         AgentKind
		agents        config.AgentsConfig
		wantAgentArgs func(scratchDir string) []string
	}{
		{
			name:  "claude",
			agent: AgentClaude,
			agents: config.AgentsConfig{
				Claude: config.AgentConfig{Model: "sonnet", Effort: "medium"},
			},
			wantAgentArgs: func(string) []string {
				return []string{"--permission-mode", "auto", "--model", "sonnet", "--effort", "medium"}
			},
		},
		{
			name:  "codex",
			agent: AgentCodex,
			agents: config.AgentsConfig{
				Codex: config.AgentConfig{Model: "gpt-5.6-sol", Effort: "medium"},
			},
			wantAgentArgs: func(scratchDir string) []string {
				return []string{
					"--sandbox", "workspace-write", "--ask-for-approval", "on-request",
					"--add-dir", filepath.Join(scratchDir, epicName),
					"--model", "gpt-5.6-sol",
					"-c", `model_reasoning_effort="medium"`,
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			realGitTimeoutWatchdog(t, realGitTestTimeout)
			repoDir := testutil.TempRepo(t)
			scratchDir := writeEpic(t, epicName, map[string]string{
				"01-first.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# First\n",
			})
			home := t.TempDir()

			var startArgv []string
			s := herdrfake.NewState(t)
			s.Register("workspace", "list", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
				return map[string]any{"workspaces": []any{}}, herdrfake.Identities{}, nil
			})
			s.Register("workspace", "create", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
				return map[string]any{"workspace": map[string]any{"workspace_id": "ws1"}}, herdrfake.Identities{WorkspaceID: "ws1"}, nil
			})
			s.Register("tab", "create", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
				return map[string]any{
					"tab":       map[string]any{"tab_id": "tab-01", "label": iterLabel(epicName, "01"), "workspace_id": "ws1"},
					"root_pane": map[string]any{"pane_id": "pane-01"},
				}, herdrfake.Identities{WorkspaceID: "ws1", TabID: "tab-01", PaneID: "pane-01"}, nil
			})
			s.Register("tab", "close", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
				return nil, herdrfake.Identities{TabID: "tab-01"}, nil
			})
			s.Register("tab", "list", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
				return map[string]any{"tabs": []any{}}, herdrfake.Identities{}, nil
			})
			s.Register("agent", "start", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
				startArgv = argv
				return nil, herdrfake.Identities{}, fmt.Errorf("herdrfake: agent start rejected for argv capture")
			})
			herdrfake.StartState(t, s)

			deps := testDepsWithOverrides(DepsOverrides{Home: home})
			deps.PreflightAgent = func(AgentKind) error { return nil }
			deps.VerifySkill = func(AgentKind, string) error { return nil }
			deps.Sleep = func(time.Duration) {}

			sink := newRecordingEventSink()
			runUntilParked(t, RunOptions{
				EpicName: epicName, Agent: tc.agent, Agents: tc.agents, Skill: "implement",
				ScratchDir: scratchDir, RepoDir: repoDir,
			}, deps, sink)

			idx := slices.Index(startArgv, "--")
			if idx == -1 {
				t.Fatalf("herdr agent start argv = %v, want a -- separator before the agent's own flags", startArgv)
			}
			got := startArgv[idx+1:]
			want := tc.wantAgentArgs(scratchDir)
			if !slices.Equal(got, want) {
				t.Errorf("herdr agent start argv after -- = %v, want %v", got, want)
			}
		})
	}
}
