package ralphloop

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/herdr"
)

// writeEpic builds a fixture epic directory under a fresh t.TempDir()'s
// .scratch/{name}/issues/, with one ticket file per entry in tickets (each a
// "NN-slug.md" -> content pair), and returns the scratch dir.
func writeEpic(t *testing.T, epicName string, tickets map[string]string) string {
	t.Helper()
	scratchDir := t.TempDir()
	issuesDir := filepath.Join(scratchDir, epicName, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	for name, content := range tickets {
		if err := os.WriteFile(filepath.Join(issuesDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}
	return scratchDir
}

// fakeDeps returns a Deps wired to in-memory fakes plus a record of prompt
// texts sent (in call order) and worktree branches removed, for assertions.
func fakeDeps() (d Deps, prompts *[]string, removedBranches *[]string) {
	prompts = &[]string{}
	removedBranches = &[]string{}
	branchByWorkspace := map[string]string{}

	d = Deps{
		FindOrCreateWorkspace: func(label, cwd string) (string, error) {
			return "ws1", nil
		},
		WorktreeCreate: func(opts herdr.WorktreeCreateOptions) (herdr.Worktree, error) {
			wsID := "ws-" + opts.Branch
			branchByWorkspace[wsID] = opts.Branch
			return herdr.Worktree{
				WorkspaceID: wsID,
				PaneID:      "pane-" + opts.Branch,
				Path:        "/fake/" + opts.Branch,
				Branch:      opts.Branch,
			}, nil
		},
		WorktreeRemove: func(workspaceID string, force bool) error {
			*removedBranches = append(*removedBranches, branchByWorkspace[workspaceID])
			return nil
		},
		AgentStart: func(opts herdr.AgentStartOptions) (herdr.Agent, error) {
			return herdr.Agent{PaneID: opts.Pane, AgentStatus: "idle"}, nil
		},
		AgentPrompt: func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
			*prompts = append(*prompts, opts.Text)
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "working"}, nil
		},
		AgentWait: func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
		},
		RevParse: func(dir, ref string) (string, error) {
			return "deadbeef", nil
		},
		CherryPickRange: func(dir, fromExclusive, toInclusive string) error {
			return nil
		},
	}
	return d, prompts, removedBranches
}

func TestRun_LinearChain_RunsTicketsInOrderAndLandsAll(t *testing.T) {
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md":  "# First\n\n**Status:** open\n",
		"02-second.md": "# Second\n\n**Blocked by:** 01\n\n**Status:** open\n",
	})
	d, prompts, removed := fakeDeps()

	var out bytes.Buffer
	err := Run(RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &out)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	wantPrompts := []string{
		"/implement " + filepath.Join(scratchDir, "my-epic", "issues", "01-first.md"),
		"/implement " + filepath.Join(scratchDir, "my-epic", "issues", "02-second.md"),
	}
	if len(*prompts) != 2 || (*prompts)[0] != wantPrompts[0] || (*prompts)[1] != wantPrompts[1] {
		t.Fatalf("prompts = %v, want %v", *prompts, wantPrompts)
	}

	wantBranches := []string{"ralph-loop/iter-01", "ralph-loop/iter-02"}
	if len(*removed) != 2 || (*removed)[0] != wantBranches[0] || (*removed)[1] != wantBranches[1] {
		t.Fatalf("removed worktree branches = %v, want %v", *removed, wantBranches)
	}

	for _, name := range []string{"01-first.md", "02-second.md"} {
		raw, err := os.ReadFile(filepath.Join(scratchDir, "my-epic", "issues", name))
		if err != nil {
			t.Fatalf("ReadFile %s: %v", name, err)
		}
		if !strings.Contains(string(raw), "Status:** done") {
			t.Errorf("%s not marked done:\n%s", name, raw)
		}
	}

	if !strings.Contains(out.String(), "complete: 2 ticket(s)") {
		t.Errorf("summary output = %q, want a completion summary mentioning 2 tickets", out.String())
	}
}

func TestRun_SkillFlag_OverridesPromptSkill(t *testing.T) {
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md": "# First\n\n**Status:** open\n",
	})
	d, prompts, _ := fakeDeps()

	var out bytes.Buffer
	if err := Run(RunOptions{EpicName: "my-epic", Skill: "tdd", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &out); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(*prompts) != 1 || !strings.HasPrefix((*prompts)[0], "/tdd ") {
		t.Fatalf("prompts = %v, want a single /tdd-prefixed prompt", *prompts)
	}
}

func TestRun_ZeroOpenTickets_NoOpSummary(t *testing.T) {
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md": "# First\n\n**Status:** done\n",
	})
	d, prompts, removed := fakeDeps()

	var out bytes.Buffer
	if err := Run(RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &out); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(*prompts) != 0 || len(*removed) != 0 {
		t.Fatalf("expected no iterations to run, got prompts=%v removed=%v", *prompts, *removed)
	}
	if !strings.Contains(out.String(), "already complete") {
		t.Errorf("summary output = %q, want a no-op/already-complete message", out.String())
	}
}

func TestRun_NoEpicFound_NoOpSummary(t *testing.T) {
	scratchDir := t.TempDir()
	d, _, _ := fakeDeps()

	var out bytes.Buffer
	if err := Run(RunOptions{EpicName: "missing-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &out); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(out.String(), "nothing to do") {
		t.Errorf("summary output = %q, want a nothing-to-do message", out.String())
	}
}
