package ralphloop

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/herdrfake"
)

// authenticatedCodexLoginScript is a fake `codex` binary that answers `codex
// login status` as already logged in and fails any other invocation —
// reused by every launch-failure scenario whose codex step must pass so the
// failure under test (a missing/incompatible Herdr integration) is reached.
const authenticatedCodexLoginScript = `if [ "$1 $2" = "login status" ]; then
  echo 'Logged in using ChatGPT'
  exit 0
fi
exit 1
`

// TestRun_ProductionRealGit_CodexLaunchPreflightFailures drives ticket 24's
// preflight through the real process boundary Run uses for a successful
// Codex launch: real exec.LookPath/exec.Command against a controlled PATH
// (a hand-rolled fake `codex` executable, and — only for the case that
// actually reaches it — a fake `herdr` via testutil/herdrfake), rather than
// loop_agent_test.go's function-injected preflightAgentWith. Each case
// asserts ticket 32's shared launch-failure outcomes via assertNoLaunchTrace
// on top of its own distinct, actionable error message.
func TestRun_ProductionRealGit_CodexLaunchPreflightFailures(t *testing.T) {
	// not parallel-safe: the "incompatible herdr integration" case's subtest
	// calls herdrfake.Start, which calls t.Setenv — and Setenv panics if the
	// parent test has called t.Parallel, so this outer test must stay
	// sequential too.
	for _, tc := range []struct {
		name          string
		codexScript   string // "" leaves `codex` absent from PATH entirely
		withHerdrFake bool
		herdrHelp     string // "agent start --help" response, when withHerdrFake
		wantErr       string
	}{
		{
			name:    "missing codex executable",
			wantErr: "codex executable not found in PATH; install Codex or add it to PATH",
		},
		{
			name: "codex not authenticated",
			codexScript: `if [ "$1 $2" = "login status" ]; then
  echo 'Not logged in'
  exit 0
fi
exit 1
`,
			wantErr: "codex is not authenticated; run `codex login`",
		},
		{
			name:        "missing herdr executable",
			codexScript: authenticatedCodexLoginScript,
			wantErr:     "Herdr executable not found in PATH; install or upgrade Herdr with Codex integration",
		},
		{
			name:          "incompatible herdr integration",
			codexScript:   authenticatedCodexLoginScript,
			withHerdrFake: true,
			herdrHelp:     "[possible values: claude, gemini]",
			wantErr:       "installed Herdr does not support Codex agents; upgrade Herdr",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			realGitTimeoutWatchdog(t, realGitTestTimeout)
			const epicName = "epic"
			repoDir := testutil.TempRepo(t)
			scratchDir := writeEpic(t, epicName, map[string]string{
				"01-first.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# First\n",
			})
			home := t.TempDir()
			pathEnv := pathExcluding("codex", "herdr")
			if tc.codexScript != "" {
				codexDir := writeFakeExecutable(t, "codex", tc.codexScript)
				pathEnv = codexDir + string(os.PathListSeparator) + pathExcluding("codex", "herdr")
			}

			herdrCalls := 0
			if tc.withHerdrFake {
				// herdrfake.Start prepends its fake herdr's bin dir to the real
				// process PATH via its own t.Setenv (outside this ticket's
				// scope to change) — recover that bin dir from the PATH delta
				// so it can be folded into pathEnv, which is what deps below
				// actually resolves lookups against.
				origPath := os.Getenv("PATH")
				herdrfake.Start(t, func(argv []string) ([]byte, int) {
					herdrCalls++
					if len(argv) == 3 && argv[0] == "agent" && argv[1] == "start" && argv[2] == "--help" {
						return []byte(tc.herdrHelp), 0
					}
					return herdrfake.CommandError("unexpected herdr command in launch-preflight test: " + strings.Join(argv, " "))
				})
				herdrDir := strings.TrimSuffix(os.Getenv("PATH"), string(os.PathListSeparator)+origPath)
				pathEnv = herdrDir + string(os.PathListSeparator) + pathEnv
			}

			deps := testDepsWithOverrides(DepsOverrides{Home: home, Path: pathEnv})
			deps.Sleep = func(time.Duration) {}

			sink := newRecordingEventSink()
			err := Run(RunOptions{
				EpicName: epicName, Agent: AgentCodex, Skill: "implement",
				ScratchDir: scratchDir, RepoDir: repoDir,
			}, deps, sink)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Run() error = %v, want containing %q", err, tc.wantErr)
			}

			assertNoLaunchTrace(t, repoDir, epicName, scratchDir, "01-first.md", sink)

			// Tab outcome for the one case that actually reaches Herdr: exactly
			// the "agent start --help" probe, no tab/workspace command.
			if tc.withHerdrFake && herdrCalls != 1 {
				t.Errorf("herdr calls = %d, want exactly 1 (the agent start --help probe, no tab ever opened)", herdrCalls)
			}
		})
	}
}

// TestRun_ProductionRealGit_MissingSkillFailsBeforeClaim drives ticket 25's
// missing-implementation-skill check through the real process boundary: a
// real HOME with no ~/.claude/skills/implement/SKILL.md, exercised via
// DefaultDeps().VerifySkill (verifySkill in deps.go) rather than a stubbed
// function, asserting the same shared launch-failure outcomes plus the
// skill's own specific, actionable reason.
func TestRun_ProductionRealGit_MissingSkillFailsBeforeClaim(t *testing.T) {
	t.Parallel()
	realGitTimeoutWatchdog(t, realGitTestTimeout)
	const epicName = "epic"
	repoDir := testutil.TempRepo(t)
	scratchDir := writeEpic(t, epicName, map[string]string{
		"01-first.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# First\n",
	})
	home := t.TempDir()

	deps := testDepsWithOverrides(DepsOverrides{Home: home})
	deps.Sleep = func(time.Duration) {}

	sink := newRecordingEventSink()
	err := Run(RunOptions{
		EpicName: epicName, Skill: "implement", ScratchDir: scratchDir, RepoDir: repoDir,
	}, deps, sink)
	wantErr := fmt.Sprintf(`skill "implement" not found at %s`, filepath.Join(home, ".claude", "skills", "implement", "SKILL.md"))
	if err == nil || !strings.Contains(err.Error(), wantErr) {
		t.Fatalf("Run() error = %v, want containing %q", err, wantErr)
	}

	assertNoLaunchTrace(t, repoDir, epicName, scratchDir, "01-first.md", sink)
}
