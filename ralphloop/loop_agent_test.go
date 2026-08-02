package ralphloop

import (
	"bytes"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/herdr"
)

func TestRun_SkillFlag_OverridesPromptSkill(t *testing.T) {
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# First\n",
	})
	d, prompts, _ := fakeDeps()

	var out bytes.Buffer
	if err := Run(RunOptions{EpicName: "my-epic", Skill: "tdd", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, NewTextEventSink(&out)); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(*prompts) != 1 || !strings.HasPrefix((*prompts)[0], "/tdd ") {
		t.Fatalf("prompts = %v, want a single /tdd-prefixed prompt", *prompts)
	}
}

func TestRun_AgentSelection_ConfiguresLaunchAndPrompt(t *testing.T) {
	for _, tc := range []struct {
		name       string
		agent      AgentKind
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			scratchDir := writeEpic(t, "my-epic", map[string]string{
				"01-first.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# First\n",
			})
			d, prompts, _ := fakeDeps()
			var start herdr.AgentStartOptions
			d.AgentStart = func(opts herdr.AgentStartOptions) (herdr.Agent, error) {
				start = opts
				return herdr.Agent{PaneID: opts.Pane, AgentStatus: "idle"}, nil
			}

			var out bytes.Buffer
			err := Run(RunOptions{EpicName: "my-epic", Agent: tc.agent, Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, NewTextEventSink(&out))
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
			}
			wantPrompt := tc.wantPrefix + filepath.Join(scratchDir, "my-epic", "issues", "01-first.md")
			if len(*prompts) != 1 || (*prompts)[0] != wantPrompt {
				t.Errorf("prompts = %v, want %q", *prompts, wantPrompt)
			}
			events, ok, readErr := readEvents(scratchDir, "my-epic")
			if readErr != nil || !ok || len(events) == 0 {
				t.Fatalf("readEvents: events=%+v ok=%v err=%v", events, ok, readErr)
			}
			for _, event := range events {
				if event.Agent != AgentKind(tc.wantKind) {
					t.Errorf("event %q agent = %q, want %q", event.Type, event.Agent, tc.wantKind)
				}
			}
		})
	}
}

func TestRun_InvalidAgent_ReturnsError(t *testing.T) {
	var out bytes.Buffer
	err := Run(RunOptions{Agent: "other"}, Deps{}, NewTextEventSink(&out))
	if err == nil || !strings.Contains(err.Error(), "must be claude or codex") {
		t.Fatalf("Run() error = %v, want invalid-agent error", err)
	}
}

func TestRun_MaxParallelOne_RunsSerially(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
		"02-b.md": "---\nid: \"02\"\nstatus: open\ntype: task\n---\n# B\n",
		"03-c.md": "---\nid: \"03\"\nstatus: open\ntype: task\n---\n# C\n",
	})
	d, prompts, _ := fakeDeps()

	var out bytes.Buffer
	err := Run(RunOptions{
		EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo",
		MaxParallel: 1,
	}, d, NewTextEventSink(&out))
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
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
		"02-b.md": "---\nid: \"02\"\nstatus: open\ntype: task\n---\n# B\n",
		"03-c.md": "---\nid: \"03\"\nstatus: open\ntype: task\n---\n# C\n",
	})
	d, _, removed := fakeDeps()
	wait, started, release := gatedAgentWait(d.AgentWait)
	d.AgentWait = wait

	var out bytes.Buffer
	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(RunOptions{
			EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo",
			MaxParallel: 2,
		}, d, NewTextEventSink(&out))
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
