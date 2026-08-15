package ralphloop

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/herdr"
)

func TestRun_CodexLaunchPreflight(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name           string
		executables    map[string]string
		loginStatus    string
		loginStatusErr error
		herdrHelp      string
		herdrHelpErr   error
		wantErr        string
		wantWorkspaces int
	}{
		{
			name:           "available",
			executables:    map[string]string{"codex": "/bin/codex", "herdr": "/bin/herdr"},
			loginStatus:    "Logged in using ChatGPT",
			herdrHelp:      "[possible values: claude, codex]",
			wantWorkspaces: 1,
		},
		{
			name:        "missing codex",
			executables: map[string]string{"herdr": "/bin/herdr"},
			wantErr:     "codex executable not found in PATH; install Codex or add it to PATH",
		},
		{
			name:        "codex not authenticated",
			executables: map[string]string{"codex": "/bin/codex", "herdr": "/bin/herdr"},
			loginStatus: "Not logged in",
			wantErr:     "codex is not authenticated; run `codex login`",
		},
		{
			name:           "unverifiable codex auth",
			executables:    map[string]string{"codex": "/bin/codex", "herdr": "/bin/herdr"},
			loginStatusErr: errors.New("unknown command login status"),
			wantErr:        "could not verify Codex authentication",
		},
		{
			name:        "missing herdr",
			executables: map[string]string{"codex": "/bin/codex"},
			loginStatus: "Logged in using ChatGPT",
			wantErr:     "Herdr executable not found in PATH; install or upgrade Herdr with Codex integration",
		},
		{
			name:        "incompatible herdr",
			executables: map[string]string{"codex": "/bin/codex", "herdr": "/bin/herdr"},
			loginStatus: "Logged in using ChatGPT",
			herdrHelp:   "[possible values: claude, gemini]",
			wantErr:     "installed Herdr does not support Codex agents; upgrade Herdr",
		},
		{
			name:         "unverifiable herdr",
			executables:  map[string]string{"codex": "/bin/codex", "herdr": "/bin/herdr"},
			loginStatus:  "Logged in using ChatGPT",
			herdrHelpErr: errors.New("unknown command agent start"),
			wantErr:      "could not verify Herdr Codex integration; upgrade Herdr",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			scratchDir := writeEpic(t, "my-epic", map[string]string{
				"01-first.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# First\n",
			})
			d, _, _ := fakeDeps()
			preflightCalls := 0
			d.PreflightAgent = func(agent AgentKind) error {
				preflightCalls++
				return preflightAgentWith(agent,
					func(name string) (string, error) {
						path, ok := tc.executables[name]
						if !ok {
							return "", errors.New("not found")
						}
						return path, nil
					},
					func(name string, args ...string) ([]byte, error) {
						if slices.Equal(args, []string{"login", "status"}) {
							if name != tc.executables["codex"] {
								t.Fatalf("command name = %q, want codex path %q", name, tc.executables["codex"])
							}
							return []byte(tc.loginStatus), tc.loginStatusErr
						}
						if name != tc.executables["herdr"] {
							t.Fatalf("command name = %q, want Herdr path %q", name, tc.executables["herdr"])
						}
						if !slices.Equal(args, []string{"agent", "start", "--help"}) {
							t.Fatalf("command args = %v, want agent start --help", args)
						}
						return []byte(tc.herdrHelp), tc.herdrHelpErr
					},
				)
			}
			findOrCreateWorkspace := d.FindOrCreateWorkspace
			workspaceCalls := 0
			d.FindOrCreateWorkspace = func(label, cwd string) (string, error) {
				workspaceCalls++
				return findOrCreateWorkspace(label, cwd)
			}

			err := Run(RunOptions{
				EpicName: "my-epic", Agent: AgentCodex, Skill: "implement",
				ScratchDir: scratchDir, RepoDir: "/fake/repo",
			}, d, noopEventSink{})
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Run() error = %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Run() error = %v, want containing %q", err, tc.wantErr)
			}
			if preflightCalls != 1 {
				t.Errorf("PreflightAgent() calls = %d, want 1", preflightCalls)
			}
			if workspaceCalls != tc.wantWorkspaces {
				t.Errorf("FindOrCreateWorkspace() calls = %d, want %d", workspaceCalls, tc.wantWorkspaces)
			}

			if tc.wantErr != "" {
				ticketPath := filepath.Join(scratchDir, "my-epic", "issues", "01-first.md")
				contents, readErr := os.ReadFile(ticketPath)
				if readErr != nil {
					t.Fatalf("ReadFile(%q): %v", ticketPath, readErr)
				}
				if !strings.Contains(string(contents), "status: open") {
					t.Errorf("ticket after preflight failure =\n%s\nwant status to remain open", contents)
				}
			}
		})
	}
}

func TestRun_MissingSkill_FailsBeforeClaimingAnyTicket(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# First\n",
	})
	d, _, _ := fakeDeps()
	verifySkillCalls := 0
	d.VerifySkill = func(agent AgentKind, skill string) error {
		verifySkillCalls++
		if agent != AgentClaude || skill != "implement" {
			t.Fatalf("VerifySkill(%q, %q), want (claude, implement)", agent, skill)
		}
		return errors.New(`skill "implement" not found at /home/x/.claude/skills/implement/SKILL.md; install it or pass a different --skill`)
	}
	findOrCreateWorkspace := d.FindOrCreateWorkspace
	workspaceCalls := 0
	d.FindOrCreateWorkspace = func(label, cwd string) (string, error) {
		workspaceCalls++
		return findOrCreateWorkspace(label, cwd)
	}

	err := Run(RunOptions{
		EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo",
	}, d, noopEventSink{})
	if err == nil || !strings.Contains(err.Error(), `skill "implement" not found`) {
		t.Fatalf("Run() error = %v, want missing-skill error", err)
	}
	if verifySkillCalls != 1 {
		t.Errorf("VerifySkill() calls = %d, want 1", verifySkillCalls)
	}
	if workspaceCalls != 0 {
		t.Errorf("FindOrCreateWorkspace() calls = %d, want 0", workspaceCalls)
	}

	ticketPath := filepath.Join(scratchDir, "my-epic", "issues", "01-first.md")
	contents, readErr := os.ReadFile(ticketPath)
	if readErr != nil {
		t.Fatalf("ReadFile(%q): %v", ticketPath, readErr)
	}
	if !strings.Contains(string(contents), "status: open") {
		t.Errorf("ticket after missing-skill failure =\n%s\nwant status to remain open", contents)
	}
}

func TestRun_ClaudeDoesNotRunCodexLaunchPreflight(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# First\n",
	})
	d, _, _ := fakeDeps()
	preflightCalls := 0
	d.PreflightAgent = func(AgentKind) error {
		preflightCalls++
		return errors.New("Codex preflight should not run")
	}

	if err := Run(RunOptions{
		EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo",
	}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if preflightCalls != 0 {
		t.Errorf("PreflightAgent() calls = %d, want 0", preflightCalls)
	}
}

func TestRun_CodexLaunchFailureAfterClaimNeedsRepair(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# First\n",
	})
	d, _, _ := fakeDeps()
	d.PreflightAgent = func(AgentKind) error { return nil }
	d.AgentStart = func(herdr.AgentStartOptions) (herdr.Agent, error) {
		return herdr.Agent{}, errors.New("Herdr rejected Codex integration")
	}
	sink := newRecordingEventSink()
	// The failed launch leaves the epic's only ticket needs-repair, so the
	// run parks on it rather than returning.
	runUntilParked(t, RunOptions{
		EpicName: "my-epic", Agent: AgentCodex, Skill: "implement",
		ScratchDir: scratchDir, RepoDir: "/fake/repo",
	}, d, sink)

	ticketPath := filepath.Join(scratchDir, "my-epic", "issues", "01-first.md")
	contents, err := os.ReadFile(ticketPath)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", ticketPath, err)
	}
	for _, unwanted := range []string{"status: done", "status: needs-answer"} {
		if strings.Contains(string(contents), unwanted) {
			t.Errorf("ticket after launch failure =\n%s\nmust not contain %q", contents, unwanted)
		}
	}
	if !strings.Contains(string(contents), "status: needs-repair") ||
		!strings.Contains(string(contents), "launching codex: Herdr rejected Codex integration") {
		t.Errorf("ticket after launch failure =\n%s\nwant durable needs-repair status and launch reason", contents)
	}

	if hasEvent(sink, LiveEventIterationFinished, func(LiveEvent) bool { return true }) ||
		hasEvent(sink, LiveEventTicketNeedsHuman, func(ev LiveEvent) bool { return ev.Status == "needs-answer" }) {
		t.Errorf("events = %+v, must not report successful/generic completion", sink.Events())
	}
}

func TestRun_SkillFlag_OverridesPromptSkill(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# First\n",
	})
	d, prompts, _ := fakeDeps()

	if err := Run(RunOptions{EpicName: "my-epic", Skill: "tdd", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(*prompts) != 1 || !strings.HasPrefix((*prompts)[0], "/tdd ") {
		t.Fatalf("prompts = %v, want a single /tdd-prefixed prompt", *prompts)
	}
}

func TestRun_AgentSelection_ConfiguresLaunchAndPrompt(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		agent      AgentKind
		agents     config.AgentsConfig
		wantKind   string
		wantArgs   []string
		wantPrefix string
	}{
		{
			name:       "claude default",
			wantKind:   "claude",
			wantArgs:   []string{"--permission-mode", "auto"},
			wantPrefix: "/implement ",
		},
		{
			name:       "codex",
			agent:      AgentCodex,
			wantKind:   "codex",
			wantArgs:   []string{"--sandbox", "workspace-write", "--ask-for-approval", "on-request"},
			wantPrefix: "$implement ",
		},
		{
			name:  "claude with model and effort",
			agent: AgentClaude,
			agents: config.AgentsConfig{
				Claude: config.AgentConfig{Model: "sonnet", Effort: "medium"},
			},
			wantKind:   "claude",
			wantArgs:   []string{"--permission-mode", "auto", "--model", "sonnet", "--effort", "medium"},
			wantPrefix: "/implement ",
		},
		{
			name:  "codex with model and effort",
			agent: AgentCodex,
			agents: config.AgentsConfig{
				Codex: config.AgentConfig{Model: "gpt-5.6-sol", Effort: "medium"},
			},
			wantKind: "codex",
			wantArgs: []string{
				"--sandbox", "workspace-write", "--ask-for-approval", "on-request",
			},
			wantPrefix: "$implement ",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			scratchDir := writeEpic(t, "my-epic", map[string]string{
				"01-first.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# First\n",
			})
			d, prompts, _ := fakeDeps()
			var start herdr.AgentStartOptions
			d.AgentStart = func(opts herdr.AgentStartOptions) (herdr.Agent, error) {
				start = opts
				return herdr.Agent{PaneID: opts.Pane, AgentStatus: "idle"}, nil
			}

			err := Run(RunOptions{EpicName: "my-epic", Agent: tc.agent, Agents: tc.agents, Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{})
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if start.Kind != tc.wantKind {
				t.Errorf("AgentStart Kind = %q, want %q", start.Kind, tc.wantKind)
			}
			if len(start.AgentArgs) < len(tc.wantArgs) || !slices.Equal(start.AgentArgs[:len(tc.wantArgs)], tc.wantArgs) {
				t.Errorf("AgentStart AgentArgs = %v, want prefix %v", start.AgentArgs, tc.wantArgs)
			}
			if tc.agent == AgentCodex {
				wantScratch := filepath.Join(scratchDir, "my-epic")
				if !slices.Contains(start.AgentArgs, wantScratch) {
					t.Errorf("AgentStart AgentArgs = %v, want epic scratch directory %q", start.AgentArgs, wantScratch)
				}
				codexCfg := tc.agents.Codex
				if codexCfg.Model != "" && !slices.Contains(start.AgentArgs, "--model") {
					t.Errorf("AgentStart AgentArgs = %v, want --model flag", start.AgentArgs)
				}
				if codexCfg.Effort != "" {
					want := `model_reasoning_effort="` + codexCfg.Effort + `"`
					if !slices.Contains(start.AgentArgs, want) {
						t.Errorf("AgentStart AgentArgs = %v, want %q", start.AgentArgs, want)
					}
				}
			}
			wantPrompt := tc.wantPrefix + filepath.Join(scratchDir, "my-epic", "issues", "01-first.md")
			if len(*prompts) != 1 || (*prompts)[0] != wantPrompt {
				t.Errorf("prompts = %v, want %q", *prompts, wantPrompt)
			}
			events, ok, readErr := ReadEvents(scratchDir, "my-epic")
			if readErr != nil || !ok || len(events) == 0 {
				t.Fatalf("ReadEvents: events=%+v ok=%v err=%v", events, ok, readErr)
			}
			for _, event := range events {
				if event.Agent != AgentKind(tc.wantKind) {
					t.Errorf("event %q agent = %q, want %q", event.Type, event.Agent, tc.wantKind)
				}
			}
		})
	}
}

func TestAgentArgs_ModelAndEffort(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		agent  AgentKind
		model  string
		effort string
		want   []string
	}{
		{
			name:  "claude both empty unchanged",
			agent: AgentClaude,
			want:  []string{"--permission-mode", "auto"},
		},
		{
			name:   "claude both set",
			agent:  AgentClaude,
			model:  "sonnet",
			effort: "medium",
			want:   []string{"--permission-mode", "auto", "--model", "sonnet", "--effort", "medium"},
		},
		{
			name:  "claude model only",
			agent: AgentClaude,
			model: "sonnet",
			want:  []string{"--permission-mode", "auto", "--model", "sonnet"},
		},
		{
			name:   "claude effort only",
			agent:  AgentClaude,
			effort: "medium",
			want:   []string{"--permission-mode", "auto", "--effort", "medium"},
		},
		{
			name:  "codex both empty unchanged",
			agent: AgentCodex,
			want: []string{
				"--sandbox", "workspace-write",
				"--ask-for-approval", "on-request",
				"--add-dir", filepath.Join("scratch", "epic"),
			},
		},
		{
			name:   "codex both set",
			agent:  AgentCodex,
			model:  "gpt-5.6-sol",
			effort: "medium",
			want: []string{
				"--sandbox", "workspace-write",
				"--ask-for-approval", "on-request",
				"--add-dir", filepath.Join("scratch", "epic"),
				"--model", "gpt-5.6-sol",
				"-c", `model_reasoning_effort="medium"`,
			},
		},
		{
			name:  "codex model only",
			agent: AgentCodex,
			model: "gpt-5.6-sol",
			want: []string{
				"--sandbox", "workspace-write",
				"--ask-for-approval", "on-request",
				"--add-dir", filepath.Join("scratch", "epic"),
				"--model", "gpt-5.6-sol",
			},
		},
		{
			name:   "codex effort only",
			agent:  AgentCodex,
			effort: "medium",
			want: []string{
				"--sandbox", "workspace-write",
				"--ask-for-approval", "on-request",
				"--add-dir", filepath.Join("scratch", "epic"),
				"-c", `model_reasoning_effort="medium"`,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := agentArgs(tc.agent, "scratch", "epic", tc.model, tc.effort)
			if !slices.Equal(got, tc.want) {
				t.Errorf("agentArgs() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRun_InvalidAgent_ReturnsError(t *testing.T) {
	t.Parallel()
	err := Run(RunOptions{Agent: "other"}, Deps{}, noopEventSink{})
	if err == nil || !strings.Contains(err.Error(), "must be claude or codex") {
		t.Fatalf("Run() error = %v, want invalid-agent error", err)
	}
}

func TestRun_MaxParallelOne_RunsSerially(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
		"02-b.md": "---\nid: \"02\"\nstatus: open\ntype: task\n---\n# B\n",
		"03-c.md": "---\nid: \"03\"\nstatus: open\ntype: task\n---\n# C\n",
	})
	d, prompts, _ := fakeDeps()

	err := Run(RunOptions{
		EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo",
		MaxParallel: 1,
	}, d, noopEventSink{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	wantOrder := []string{"01-a.md", "02-b.md", "03-c.md"}
	if len(*prompts) != len(wantOrder) {
		t.Fatalf("prompts = %v, want %d prompts", *prompts, len(wantOrder))
	}
	for i, name := range wantOrder {
		if !strings.HasSuffix((*prompts)[i], name) {
			t.Errorf("prompts[%d] = %q, want suffix %q (serial, ticket-number order)", i, (*prompts)[i], name)
		}
	}
}

func TestRun_MaxParallelTwo_RunsExactlyTwoConcurrentlyAndBackfills(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
		"02-b.md": "---\nid: \"02\"\nstatus: open\ntype: task\n---\n# B\n",
		"03-c.md": "---\nid: \"03\"\nstatus: open\ntype: task\n---\n# C\n",
	})
	d, _, removed := fakeDeps()
	wait, started, release := gatedAgentWait(d.AgentWait)
	d.AgentWait = wait

	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(RunOptions{
			EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo",
			MaxParallel: 2,
		}, d, noopEventSink{})
	}()

	pane1 := <-started
	pane2 := <-started

	select {
	case pane3 := <-started:
		t.Fatalf("a third iteration started with only 2 slots and both full: %s", pane3)
	case <-time.After(100 * time.Millisecond):
	}

	release(pane1)
	pane3 := <-started // backfilled without waiting for pane2

	release(pane2)
	release(pane3)

	if err := <-errCh; err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(*removed) != 3 {
		t.Errorf("removed worktree branches = %v, want 3 entries", *removed)
	}
}

func TestRun_PauseLetsInFlightFinishAndResumesScheduling(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
		"02-b.md": "---\nid: \"02\"\nstatus: open\ntype: task\n---\n# B\n",
	})
	d, _, _ := fakeDeps()
	wait, started, release := gatedAgentWait(d.AgentWait)
	d.AgentWait = wait
	d.Sleep = func(time.Duration) { time.Sleep(time.Millisecond) }
	gate := NewGate()

	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(RunOptions{
			EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo",
			MaxParallel: 1, Gate: gate,
		}, d, noopEventSink{})
	}()

	first := <-started
	gate.Pause(QueuePauseLabel, "queue paused")
	release(first)

	select {
	case pane := <-started:
		t.Fatalf("iteration %q started while the queue was paused", pane)
	case err := <-errCh:
		t.Fatalf("Run() exited while paused: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	if !gate.ForceResume(QueuePauseLabel) {
		t.Fatal("expected queue pause to be active")
	}
	second := <-started
	release(second)

	if err := <-errCh; err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}
