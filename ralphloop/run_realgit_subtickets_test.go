package ralphloop

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/herdrfake"
)

// fullTicketIDFromImplementPrompt is ticketIDFromImplementPrompt's variant for
// this file's tests: it returns the implement prompt's full "NN[letters]"
// filename prefix (e.g. "01a") rather than just the first two characters, so
// a parent (e.g. "01") and its lettered children (e.g. "01a"/"01b") - which
// share those first two characters - are told apart.
func fullTicketIDFromImplementPrompt(text string) (id string, ok bool) {
	base, found := strings.CutPrefix(text, "/implement ")
	if !found {
		base, found = strings.CutPrefix(text, "$implement ")
	}
	if !found {
		return "", false
	}
	base = filepath.Base(base)
	idx := strings.Index(base, "-")
	if idx <= 0 {
		return "", false
	}
	return base[:idx], true
}

// addChildrenToTicket inserts a "children: [...]" frontmatter line into the
// ticket file at path, leaving every other frontmatter field (notably
// status:, which the loop may have already rewritten to "claimed" by the
// time a mid-run split happens) and the body untouched.
func addChildrenToTicket(path string, childIDs []string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	parts := strings.SplitN(string(raw), "---\n", 3)
	if len(parts) != 3 {
		return fmt.Errorf("malformed ticket frontmatter in %s", path)
	}
	quoted := make([]string, len(childIDs))
	for i, id := range childIDs {
		quoted[i] = fmt.Sprintf("%q", id)
	}
	newFrontmatter := parts[1] + "children: [" + strings.Join(quoted, ", ") + "]\n"
	return os.WriteFile(path, []byte("---\n"+newFrontmatter+"---\n"+parts[2]), 0644)
}

// writeChildTicket writes a fresh open child ticket file (filename
// "{id}-child.md") under issuesDir, with parent recorded as parentID - the
// fake agent's stand-in for a real implement skill turn that decides to split
// its ticket into subtickets mid-run.
func writeChildTicket(issuesDir, id, parentID string) error {
	content := fmt.Sprintf("---\nid: %q\nstatus: open\ntype: task\nparent: %q\n---\n# Child %s\n", id, parentID, id)
	return os.WriteFile(filepath.Join(issuesDir, id+"-child.md"), []byte(content), 0644)
}

// linkedWorktreeDir resolves repoDir's linked-worktree directory (e.g.
// "<repoDir>/.worktrees" for a standard clone) the same way Run resolves it
// via Deps.WorktreeDir, so tests that need to predict an iteration worktree's
// path agree with the loop's own path.
func linkedWorktreeDir(t *testing.T, repoDir string) string {
	t.Helper()
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo(%s): %v", repoDir, err)
	}
	return repo.LinkedWorktreeDir()
}

// subticketsFakeHerdr builds the herdr fake handler shared by both tests in
// this file: every ticket's implement turn commits one file to its own
// iteration worktree (via commitIterationWork, keyed by the full ticket
// identifier so lettered children never collide with their parent or each
// other), and onImplement runs once per ticket right after that commit - the
// hook each test uses to write subticket files for a specific parent.
func subticketsFakeHerdr(t *testing.T, worktreeDir, epicName string, onImplement func(id string)) (
	handler func([]string) ([]byte, int),
	commitOrder func() []string,
	openTabCount func() int,
	closedTabCount func() int,
) {
	t.Helper()

	var mu sync.Mutex
	status := map[string]string{}
	session := map[string]string{}
	openTabs := map[string]bool{}
	closedTabs := map[string]bool{}
	var order []string

	handler = func(argv []string) ([]byte, int) {
		if len(argv) < 2 {
			return herdrfake.CommandError(fmt.Sprintf("command too short: %v", argv))
		}
		switch argv[0] + " " + argv[1] {
		case "workspace list":
			return herdrfake.Result(map[string]any{"workspaces": []any{}})
		case "workspace create":
			return herdrfake.Result(map[string]any{"workspace": map[string]any{"workspace_id": "ws1"}})

		case "tab create":
			label := flagValue(argv, "--label")
			tabID, pane := "tab-"+label, "pane-"+label
			mu.Lock()
			openTabs[tabID] = true
			mu.Unlock()
			return herdrfake.Result(map[string]any{
				"tab":       map[string]any{"tab_id": tabID, "label": label, "workspace_id": flagValue(argv, "--workspace")},
				"root_pane": map[string]any{"pane_id": pane},
			})
		case "tab close":
			tabID := argv[2]
			mu.Lock()
			closedTabs[tabID] = true
			delete(openTabs, tabID)
			mu.Unlock()
			return []byte(`{"result":null}`), 0
		case "tab list":
			return herdrfake.Result(map[string]any{"tabs": []any{}})

		case "agent start":
			pane := flagValue(argv, "--pane")
			sess := "sess-" + argv[2]
			mu.Lock()
			status[pane] = "idle"
			session[pane] = sess
			mu.Unlock()
			return agentJSON(pane, "idle", sess)

		case "agent prompt":
			pane, text := argv[2], argv[3]
			mu.Lock()
			sess := session[pane]
			mu.Unlock()
			if id, ok := fullTicketIDFromImplementPrompt(text); ok {
				dir := iterationWorktreePath(worktreeDir, epicName, id)
				if err := commitIterationWork(dir, id); err != nil {
					t.Errorf("commitIterationWork(%s): %v", id, err)
					return herdrfake.CommandError(err.Error())
				}
				if onImplement != nil {
					onImplement(id)
				}
				mu.Lock()
				status[pane] = "idle"
				order = append(order, id)
				mu.Unlock()
			}
			return agentJSON(pane, "working", sess)

		case "agent wait":
			pane := argv[2]
			until := parseUntil(argv[3:])
			mu.Lock()
			cur, sess := status[pane], session[pane]
			mu.Unlock()
			if len(until) == 0 || slices.Contains(until, cur) {
				return agentJSON(pane, cur, sess)
			}
			return herdrfake.CommandError("timed out waiting for agent status")

		case "agent send-keys":
			pane := argv[2]
			mu.Lock()
			cur, sess := status[pane], session[pane]
			mu.Unlock()
			return agentJSON(pane, cur, sess)

		case "agent read":
			return []byte(""), 0

		default:
			return herdrfake.CommandError("unimplemented command: " + argv[0] + " " + argv[1])
		}
	}

	commitOrder = func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string{}, order...)
	}
	openTabCount = func() int {
		mu.Lock()
		defer mu.Unlock()
		return len(openTabs)
	}
	closedTabCount = func() int {
		mu.Lock()
		defer mu.Unlock()
		return len(closedTabs)
	}
	return handler, commitOrder, openTabCount, closedTabCount
}

// assertSubticketRunCompleted checks the shared postconditions both tests in
// this file want: every ticket in wantIDs landed a done commit, its iteration
// worktree/branch was cleaned up, and no herdr tab was left open.
func assertSubticketRunCompleted(t *testing.T, repoDir, epicName, scratchDir string, wantIDs []string, ticketFilenames map[string]string, openTabCount, closedTabCount func() int) {
	t.Helper()

	worktreeDir := linkedWorktreeDir(t, repoDir)
	featurePath := filepath.Join(worktreeDir, epicName)
	issuesDir := filepath.Join(scratchDir, epicName, "issues")

	for _, id := range wantIDs {
		raw, err := os.ReadFile(filepath.Join(issuesDir, ticketFilenames[id]))
		if err != nil {
			t.Fatalf("ReadFile %s: %v", ticketFilenames[id], err)
		}
		if !strings.Contains(string(raw), "status: done") {
			t.Errorf("ticket %s not marked done:\n%s", id, raw)
		}
	}

	for _, id := range wantIDs {
		path := iterationWorktreePath(worktreeDir, epicName, id)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("iteration worktree %s for ticket %s still exists, want removed", path, id)
		}
		branch := iterBranch(epicName, id)
		if _, err := git.RevParse(featurePath, branch); err == nil {
			t.Errorf("iteration branch %s for ticket %s still exists, want deleted", branch, id)
		}
	}

	if got := openTabCount(); got != 0 {
		t.Errorf("open tab count = %d, want all iteration tabs closed", got)
	}
	if got := closedTabCount(); got != len(wantIDs) {
		t.Errorf("closed tab count = %d, want exactly %d (one per ticket)", got, len(wantIDs))
	}
}

// TestRun_ProductionRealGit_TicketCreatesSubtickets drives a single regular
// task ticket (01) whose implement turn splits mid-run: right after landing
// its own commit, the fake agent writes two lettered child ticket files
// (01a, 01b, parent: "01") into the epic's issues directory and records
// Children on 01 itself - mirroring a real gx-implement turn that decides the
// work is bigger than one iteration and splits it. Nothing in the run request
// names 01a/01b upfront (RunOptions.TicketIDs is unset, i.e. whole-epic
// scope): Run must discover them from disk on its next frontier scan, start
// them, and only then consider the epic done.
func TestRun_ProductionRealGit_TicketCreatesSubtickets(t *testing.T) {
	const epicName = "epic"
	repoDir := testutil.TempRepo(t)

	scratchDir := writeEpic(t, epicName, map[string]string{
		"01-parent.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# Parent\n",
	})
	issuesDir := filepath.Join(scratchDir, epicName, "issues")
	parentPath := filepath.Join(issuesDir, "01-parent.md")

	t.Setenv("HOME", t.TempDir())

	var childrenOnce sync.Once
	onImplement := func(id string) {
		if id != "01" {
			return
		}
		childrenOnce.Do(func() {
			for _, childID := range []string{"01a", "01b"} {
				if err := writeChildTicket(issuesDir, childID, "01"); err != nil {
					t.Errorf("writeChildTicket(%s): %v", childID, err)
				}
			}
			if err := addChildrenToTicket(parentPath, []string{"01a", "01b"}); err != nil {
				t.Errorf("addChildrenToTicket: %v", err)
			}
		})
	}

	handler, commitOrder, openTabCount, closedTabCount := subticketsFakeHerdr(t, linkedWorktreeDir(t, repoDir), epicName, onImplement)
	herdrfake.Start(t, handler)

	deps := DefaultDeps()
	deps.Sleep = func(time.Duration) {}
	deps.VerifySkill = func(AgentKind, string) error { return nil }

	var out bytes.Buffer
	if err := Run(RunOptions{EpicName: epicName, Skill: "implement", ScratchDir: scratchDir, RepoDir: repoDir}, deps, NewTextEventSink(&out)); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	wantIDs := []string{"01", "01a", "01b"}
	order := commitOrder()
	if len(order) != len(wantIDs) {
		t.Fatalf("commitOrder = %v, want exactly one implement turn per %v", order, wantIDs)
	}
	if order[0] != "01" {
		t.Errorf("commitOrder = %v, want parent 01 implemented before its children start", order)
	}
	seen := map[string]bool{}
	for _, id := range order {
		seen[id] = true
	}
	for _, id := range wantIDs {
		if !seen[id] {
			t.Errorf("commitOrder = %v, missing child %s: children were never started", order, id)
		}
	}

	assertSubticketRunCompleted(t, repoDir, epicName, scratchDir, wantIDs, map[string]string{
		"01":  "01-parent.md",
		"01a": "01a-child.md",
		"01b": "01b-child.md",
	}, openTabCount, closedTabCount)
}

// TestRun_ProductionRealGit_CodeReviewTicketCreatesSubtickets is ticket
// creates-subtickets's sibling scenario: 02 is type: code-review (eligible
// only once every other ticket in the epic is done, see
// Epic.effectiveBlockedBy) rather than a plain task. It stays blocked while
// 01 runs, becomes frontier once 01 lands, and then splits mid-run exactly
// like a regular ticket - writing 02a/02b (parent: "02") after its own
// commit. Run must still start those findings tickets and only then consider
// the epic done, proving the code-review frontier rule and the mid-run split
// discovery compose correctly.
func TestRun_ProductionRealGit_CodeReviewTicketCreatesSubtickets(t *testing.T) {
	const epicName = "epic"
	repoDir := testutil.TempRepo(t)

	scratchDir := writeEpic(t, epicName, map[string]string{
		"01-implement.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# Implement\n",
		"02-review.md":    "---\nid: \"02\"\nstatus: open\ntype: code-review\n---\n# Review\n",
	})
	issuesDir := filepath.Join(scratchDir, epicName, "issues")
	reviewPath := filepath.Join(issuesDir, "02-review.md")

	t.Setenv("HOME", t.TempDir())

	var childrenOnce sync.Once
	onImplement := func(id string) {
		if id != "02" {
			return
		}
		childrenOnce.Do(func() {
			for _, childID := range []string{"02a", "02b"} {
				if err := writeChildTicket(issuesDir, childID, "02"); err != nil {
					t.Errorf("writeChildTicket(%s): %v", childID, err)
				}
			}
			if err := addChildrenToTicket(reviewPath, []string{"02a", "02b"}); err != nil {
				t.Errorf("addChildrenToTicket: %v", err)
			}
		})
	}

	handler, commitOrder, openTabCount, closedTabCount := subticketsFakeHerdr(t, linkedWorktreeDir(t, repoDir), epicName, onImplement)
	herdrfake.Start(t, handler)

	deps := DefaultDeps()
	deps.Sleep = func(time.Duration) {}
	deps.VerifySkill = func(AgentKind, string) error { return nil }

	var out bytes.Buffer
	if err := Run(RunOptions{EpicName: epicName, Skill: "implement", ScratchDir: scratchDir, RepoDir: repoDir}, deps, NewTextEventSink(&out)); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	wantIDs := []string{"01", "02", "02a", "02b"}
	order := commitOrder()
	if len(order) != len(wantIDs) {
		t.Fatalf("commitOrder = %v, want exactly one implement turn per %v", order, wantIDs)
	}
	idx01, idx02 := slices.Index(order, "01"), slices.Index(order, "02")
	if idx01 == -1 || idx02 == -1 || idx01 > idx02 {
		t.Errorf("commitOrder = %v, want the code-review ticket (02) to start only after 01 is done", order)
	}
	seen := map[string]bool{}
	for _, id := range order {
		seen[id] = true
	}
	for _, id := range []string{"02a", "02b"} {
		if !seen[id] {
			t.Errorf("commitOrder = %v, missing code-review finding %s: children were never started", order, id)
		}
	}

	assertSubticketRunCompleted(t, repoDir, epicName, scratchDir, wantIDs, map[string]string{
		"01":  "01-implement.md",
		"02":  "02-review.md",
		"02a": "02a-child.md",
		"02b": "02b-child.md",
	}, openTabCount, closedTabCount)
}
