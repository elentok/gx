package ralphloop

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/tickets/schema"
)

// TestConflictResolutionHandoff covers what gx does with the conflict-resolution
// child once its agent exits, for both a fresh resolver (resolveCherryPickConflict)
// and one reattached after a restart (reattachLiveConflictResolver).
func TestConflictResolutionHandoff(t *testing.T) {
	t.Parallel()

	const childFile = "03a-conflict-resolution.md"
	const childFrontmatter = "---\nid: \"03a\"\nstatus: claimed\ntype: conflict-resolution\nparent: \"03\"\n---\n# Conflict resolution for 03\n"

	cases := []struct {
		name string
		// edit is the agent's own change to the child ticket while it works.
		edit            func(t *testing.T, path string)
		stillConflicted bool
		wantChild       schema.Status
		wantFreshErr    func(error) bool
	}{
		{
			name: "invalid child status with a clean pick ends done",
			edit: func(t *testing.T, path string) {
				raw := mustRead(t, path)
				if err := os.WriteFile(path, []byte(strings.Replace(raw, "status: claimed", "status: resolved", 1)), 0644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
			},
			wantChild: schema.StatusDone,
		},
		{
			name:      "valid claimed child with a clean pick closes done",
			edit:      func(*testing.T, string) {},
			wantChild: schema.StatusDone,
		},
		{
			name:            "pick still conflicted is not closed done",
			edit:            func(*testing.T, string) {},
			stillConflicted: true,
			// A fresh resolver pauses its child paused (repair); a reattached one
			// leaves the child claimed and just fails the iteration.
			wantChild:    schema.StatusClaimed,
			wantFreshErr: func(err error) bool { return errors.Is(err, errConflictResolutionUnresolved) },
		},
	}

	for _, tc := range cases {
		for _, path := range []string{"fresh", "reattached"} {
			t.Run(path+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				d, _, _ := fakeDeps()
				p := conflictParams(t, t.TempDir())
				label := conflictLabel(p.Ticket.Identifier)
				issuesDir := filepath.Join(p.ScratchDir, "main", "issues")
				childPath := filepath.Join(issuesDir, childFile)
				if err := os.MkdirAll(issuesDir, 0755); err != nil {
					t.Fatalf("MkdirAll: %v", err)
				}
				parent := "---\nid: \"03\"\nstatus: claimed\ntype: implement\n---\n# C\n"
				if err := os.WriteFile(filepath.Join(issuesDir, "03-c.md"), []byte(parent), 0644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}

				inProgress := true
				d.CherryPickInProgress = func(string) (bool, error) { return inProgress, nil }
				worked := false
				agentWorks := func() {
					if worked {
						return
					}
					worked = true
					child, err := findConflictResolutionChildTicket(p)
					if err != nil {
						t.Errorf("finding child: %v", err)
						return
					}
					tc.edit(t, child)
					inProgress = tc.stillConflicted
				}

				var err error
				if path == "fresh" {
					onRunnerPrompt(d, func(agentrunner.Session, string) error {
						agentWorks()
						return nil
					})
					_, err = resolveCherryPickConflict(d, p)
					var loadErr error
					if childPath, loadErr = loadChild(p); loadErr != nil {
						t.Fatal(loadErr)
					}
				} else {
					if err := os.WriteFile(childPath, []byte(childFrontmatter), 0644); err != nil {
						t.Fatalf("WriteFile: %v", err)
					}
					hostLiveAgent(t, d, label)
					fakeRunner(d).SetState(label, agentrunner.StateWorking, "")
					onRunnerWait(d, func(s agentrunner.Session) {
						if s.Label == label {
							agentWorks()
							fakeRunner(d).SetState(label, agentrunner.StateIdle, "")
						}
					})
					live, found, findErr := d.Runner.Find(label)
					if findErr != nil || !found {
						t.Fatalf("Find(%s) = found %v, err %v", label, found, findErr)
					}
					_, err = reattachLiveConflictResolver(d, p, live)
				}

				if tc.stillConflicted {
					if err == nil {
						t.Fatal("handoff succeeded with the cherry-pick still conflicted")
					}
					if path == "fresh" && !tc.wantFreshErr(err) {
						t.Errorf("err = %v, want the unresolved sentinel", err)
					}
				} else if err != nil {
					t.Fatalf("handoff: %v", err)
				}

				child := mustParse(t, childPath)
				if tc.stillConflicted && path == "fresh" {
					assertPaused(t, child, schema.PauseClassRepair, "")
				} else if child.Status != tc.wantChild {
					t.Errorf("child status = %q, want %q", child.Status, tc.wantChild)
				}
				if tc.wantChild == schema.StatusDone && len(child.SessionIDs) != 1 {
					t.Errorf("child session_ids = %v, want the resolver's session", child.SessionIDs)
				}
			})
		}
	}
}

// loadChild returns the path of the single conflict-resolution child file under
// the test epic, whatever its status.
func loadChild(p iterationParams) (string, error) {
	matches, err := filepath.Glob(filepath.Join(p.ScratchDir, p.FeatureBranch, "issues", "*-conflict-resolution.md"))
	if err != nil || len(matches) != 1 {
		return "", errors.New("want exactly one conflict-resolution child ticket on disk")
	}
	return matches[0], nil
}

// TestRun_CommitlessFork_WithExplicitBase_IsAdoptedDoneWithoutPausing covers a
// fork's placeholder authored with `base:` that the agent then closes as
// `--iteration-status finished --commitless true`: the CLI drops base on that
// transition (cmd's tickets set tests), so adoption must read it as a valid
// commitless ticket and write done rather than pausing on validation.
func TestRun_CommitlessFork_WithExplicitBase_IsAdoptedDoneWithoutPausing(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\nbase: main\n---\n# A\n",
	})
	ticketPath := filepath.Join(scratchDir, "epic", "issues", "01-a.md")

	d, _, _ := fakeDeps()
	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) { return 0, nil }
	onRunnerPrompt(d, func(agentrunner.Session, string) error {
		return updateTicket(ticketPath, func(tk *schema.Ticket) {
			tk.IterationStatus = schema.IterationStatusFinished
			tk.Commitless = true
			tk.Base = ""
		})
	})

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got := mustParse(t, ticketPath)
	if got.Status != schema.StatusDone || !got.Commitless {
		t.Errorf("ticket status = %q commitless = %v, want done and commitless", got.Status, got.Commitless)
	}
	if got.PauseReason != "" {
		t.Errorf("pause reason = %q, want none", got.PauseReason)
	}
}
