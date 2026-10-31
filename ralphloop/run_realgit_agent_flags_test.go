package ralphloop

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/harness"
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
	t.Parallel()
	const epicName = "epic"
	for _, tc := range []struct {
		name          string
		agent         AgentKind
		agents        config.AgentsConfig
		wantAgentArgs func(scratchDir string) []string
		// wantRoots: the launch carries writable_roots from the real worktree.
		wantRoots bool
	}{
		{
			name:  "claude",
			agent: AgentClaude,
			agents: config.AgentsConfig{
				Claude: config.AgentConfig{Model: "sonnet", Effort: "medium"},
			},
			wantAgentArgs: func(string) []string {
				return []string{"--permission-mode", "auto", "--settings", claudeGuardSettings, "--disallowedTools", agentrunner.LoopDisallowedTools, "--append-system-prompt-file", harness.GXSystemPromptFile(), "--model", "sonnet", "--effort", "medium"}
			},
		},
		{
			name:  "codex",
			agent: AgentCodex,
			agents: config.AgentsConfig{
				Codex: config.AgentConfig{Model: "gpt-6.1-sol", Effort: "medium"},
			},
			wantAgentArgs: func(string) []string {
				return harness.CodexArgs(harness.CodexArgsInput{Model: "gpt-6.1-sol", Effort: "medium"})
			},
			wantRoots: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			realGitTimeoutWatchdog(t, realGitTestTimeout)
			repoDir := testutil.TempRepo(t)
			scratchDir := writeEpic(t, epicName, map[string]string{
				"01-first.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# First\n",
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
			_, fakeEnv := herdrfake.StartStateEnv(t, s)

			deps := testDepsWithOverrides(DepsOverrides{Home: home, Env: withProcessEnv(fakeEnv)})
			deps.PreflightAgent = func(AgentKind) error { return nil }
			deps.VerifySkill = func(AgentKind, string) error { return nil }
			deps.Sleep = func(time.Duration) {}

			sink := newRecordingEventSink()
			runUntilPaused(t, RunOptions{
				EpicName: epicName, Agent: tc.agent, Agents: tc.agents, Skill: "implement",
				ScratchDir: scratchDir, RepoDir: repoDir,
			}, deps, sink)

			idx := slices.Index(startArgv, "--")
			if idx == -1 {
				t.Fatalf("herdr agent start argv = %v, want a -- separator before the agent's own flags", startArgv)
			}
			got := startArgv[idx+1:]
			if tc.wantRoots {
				i := slices.IndexFunc(got, func(a string) bool { return strings.HasPrefix(a, "sandbox_workspace_write.writable_roots=") })
				if i < 1 {
					t.Fatalf("argv = %v, want a writable_roots flag", got)
				}
				common, err := filepath.EvalSymlinks(filepath.Join(repoDir, ".git"))
				if err != nil {
					t.Fatal(err)
				}
				for _, root := range []string{filepath.Join(common, "worktrees"), common, scratchDir, filepath.Join(scratchDir, epicName)} {
					if !strings.Contains(got[i], root) {
						t.Errorf("writable_roots %s lacks %s", got[i], root)
					}
				}
				got = slices.Delete(slices.Clone(got), i-1, i+1)
			}
			want := tc.wantAgentArgs(scratchDir)
			if !slices.Equal(got, want) {
				t.Errorf("herdr agent start argv after -- = %v, want %v", got, want)
			}
		})
	}
}
