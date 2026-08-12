package ralphloop

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/herdrfake"
	"github.com/elentok/gx/tickets/schema"
)

// TestRun_ProductionRealGit_AThenBAndCConcurrently drives ticket 06's A ->
// {B, C} scenario through Run with production Git and herdr dependency
// wiring: DefaultDeps with only Sleep swapped for a no-op (no smart-zone or
// compaction path is exercised here, so Now is left real), every git
// operation (worktrees, branches, cherry-picks, trailers) hitting a real
// repo via the real git package, and every herdr call (workspace/tab/agent)
// going out via the real herdr client functions to a fake `herdr` executable
// reached through PATH (testutil/herdrfake). The fake agent's only
// deterministic behavior is: answering an "/implement <ticket>" prompt by
// committing one real file to that ticket's real iteration worktree, and
// (for ticket C only) blocking until ticket B's commit has already landed —
// the scenario's fake-agent evidence that B's iteration commit is created
// before C's, even though B and C are scheduled and run concurrently.
func TestRun_ProductionRealGit_AThenBAndCConcurrently(t *testing.T) {
	realGitTimeoutWatchdog(t, realGitTestTimeout)
	const epicName = "epic"
	repoDir := testutil.TempRepo(t)
	wtDir := testWorktreeDir(t, repoDir)

	scratchDir := writeEpic(t, epicName, map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
		"02-b.md": "---\nid: \"02\"\nstatus: open\ntype: task\nblocked_by: [\"01\"]\n---\n# B\n",
		"03-c.md": "---\nid: \"03\"\nstatus: open\ntype: task\nblocked_by: [\"01\"]\n---\n# C\n",
	})

	home := t.TempDir()

	var mu sync.Mutex
	status := map[string]string{}  // pane -> agent_status
	session := map[string]string{} // pane -> agent session id
	openTabs := map[string]bool{}
	closedTabs := map[string]bool{}
	var commitOrder []string
	bLanded := make(chan struct{})
	var bLandedOnce sync.Once

	handler := func(argv []string) ([]byte, int) {
		if len(argv) < 2 {
			return herdrfake.CommandError(fmt.Sprintf("command too short: %v", argv))
		}
		switch argv[0] + " " + argv[1] {
		case "workspace list":
			return herdrfake.Result(map[string]any{"workspaces": []any{}})
		case "workspace create":
			return herdrfake.Result(map[string]any{"workspace": map[string]any{"workspace_id": "ws1"}})

		case "tab create":
			label := realGitFlagValue(argv, "--label")
			tabID, pane := "tab-"+label, "pane-"+label
			mu.Lock()
			openTabs[tabID] = true
			mu.Unlock()
			return herdrfake.Result(map[string]any{
				"tab":       map[string]any{"tab_id": tabID, "label": label, "workspace_id": realGitFlagValue(argv, "--workspace")},
				"root_pane": map[string]any{"pane_id": pane},
			})
		case "tab close":
			tabID := argv[2]
			mu.Lock()
			closedTabs[tabID] = true
			delete(openTabs, tabID)
			mu.Unlock()
			if tabID == "tab-"+iterLabel(epicName, "02") {
				// B's tab only closes once finishCleanup runs, i.e. strictly
				// after B's commit has already landed on the feature branch
				// (finishIteration cherry-picks before finishCleanup ever
				// runs) — the point C's own work must wait behind to make
				// B's landing-before-C's guaranteed, not just racily likely.
				bLandedOnce.Do(func() { close(bLanded) })
			}
			return []byte(`{"result":null}`), 0
		case "tab list":
			return herdrfake.Result(map[string]any{"tabs": []any{}})

		case "agent start":
			pane := realGitFlagValue(argv, "--pane")
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
			if id, ok := ticketIDFromImplementPrompt(text); ok {
				if id == "03" {
					<-bLanded
				}
				dir := iterationWorktreePath(wtDir, epicName, id)
				if err := commitIterationWork(dir, id); err != nil {
					t.Errorf("commitIterationWork(%s): %v", id, err)
					return herdrfake.CommandError(err.Error())
				}
				mu.Lock()
				status[pane] = "idle"
				commitOrder = append(commitOrder, id)
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

	herdrfake.Start(t, handler)

	deps := testDepsWithOverrides(DepsOverrides{Home: home})
	deps.Sleep = func(time.Duration) {}
	deps.VerifySkill = func(AgentKind, string) error { return nil }

	if err := Run(RunOptions{EpicName: epicName, Skill: "implement", ScratchDir: scratchDir, RepoDir: repoDir}, deps, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(commitOrder) != 3 || commitOrder[0] != "01" {
		t.Fatalf("commitOrder = %v, want A (01) created first", commitOrder)
	}
	idxB, idxC := slices.Index(commitOrder, "02"), slices.Index(commitOrder, "03")
	if idxB == -1 || idxC == -1 || idxB > idxC {
		t.Errorf("commitOrder = %v, want B (02) created before C (03)", commitOrder)
	}

	featurePath := filepath.Join(wtDir, epicName)
	trailers, err := git.TrailerMap(featurePath, "HEAD", ticketTrailerKey)
	if err != nil {
		t.Fatalf("TrailerMap: %v", err)
	}
	shaA := trailers[ticketTrailerValue(epicName, "01")]
	shaB := trailers[ticketTrailerValue(epicName, "02")]
	shaC := trailers[ticketTrailerValue(epicName, "03")]
	if shaA == "" || shaB == "" || shaC == "" {
		t.Fatalf("landed trailers = %v, want a landed commit for each of A, B, C", trailers)
	}
	if ok, err := git.IsAncestor(featurePath, shaA, shaB); err != nil || !ok {
		t.Errorf("A's landed commit is not an ancestor of B's (ok=%v err=%v): feature history doesn't have A before B", ok, err)
	}
	if ok, err := git.IsAncestor(featurePath, shaB, shaC); err != nil || !ok {
		t.Errorf("B's landed commit is not an ancestor of C's (ok=%v err=%v): feature history doesn't have B before C", ok, err)
	}

	for _, name := range []string{"01-a.md", "02-b.md", "03-c.md"} {
		raw, err := os.ReadFile(filepath.Join(scratchDir, epicName, "issues", name))
		if err != nil {
			t.Fatalf("ReadFile %s: %v", name, err)
		}
		if !strings.Contains(string(raw), "status: done") {
			t.Errorf("%s not marked done:\n%s", name, raw)
		}
	}

	for _, id := range []string{"01", "02", "03"} {
		path := iterationWorktreePath(wtDir, epicName, id)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("iteration worktree %s for ticket %s still exists, want removed", path, id)
		}
		branch := iterBranch(epicName, id)
		if _, err := git.RevParse(featurePath, branch); err == nil {
			t.Errorf("iteration branch %s for ticket %s still exists, want deleted", branch, id)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(openTabs) != 0 {
		t.Errorf("openTabs = %v, want all iteration tabs closed", openTabs)
	}
	if len(closedTabs) != 3 {
		t.Errorf("closedTabs = %v, want exactly 3 (one per ticket)", closedTabs)
	}
}

// TestRun_ProductionRealGit_DiamondThroughFullEpic extends ticket 06's A ->
// {B, C} scenario through a second parallel frontier and a final join:
// A -> {B, C} -> {D, E} -> F. It reuses ticket 06's landing-order trick (an
// agent blocks on a sibling's tab closing, which only happens once that
// sibling's commit has already landed) to force B before C and, symmetrically,
// E before D — even though each pair is scheduled and runs concurrently. F is
// left to the scheduler alone: it only becomes frontier once both D and E are
// done, so no extra fake-agent blocking is needed to prove F starts last.
func TestRun_ProductionRealGit_DiamondThroughFullEpic(t *testing.T) {
	realGitTimeoutWatchdog(t, realGitTestTimeout)
	const epicName = "epic"
	repoDir := testutil.TempRepo(t)
	wtDir := testWorktreeDir(t, repoDir)
	featurePath := filepath.Join(wtDir, epicName)
	type ticketContract struct {
		filename      string
		body          string
		blockedBy     []string
		tokens        int
		trailerTokens int
		elapsed       int
		compactions   int
	}
	contracts := map[string]ticketContract{
		"01": {filename: "01-a.md", body: "# A\n", tokens: 1100, elapsed: 1},
		"02": {filename: "02-b.md", body: "# B\n", blockedBy: []string{"01"}, tokens: 1200, elapsed: 1},
		"03": {filename: "03-c.md", body: "# C\n", blockedBy: []string{"01"}, tokens: 50, trailerTokens: 600, elapsed: 180, compactions: 1},
		"04": {filename: "04-d.md", body: "# D\n", blockedBy: []string{"02", "03"}, tokens: 1400, elapsed: 1},
		"05": {filename: "05-e.md", body: "# E\n", blockedBy: []string{"02", "03"}, tokens: 1500, elapsed: 1},
		"06": {filename: "06-f.md", body: "# F\n", blockedBy: []string{"04", "05"}, tokens: 1600, elapsed: 1},
	}

	scratchDir := writeEpic(t, epicName, map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
		"02-b.md": "---\nid: \"02\"\nstatus: open\ntype: task\nblocked_by: [\"01\"]\n---\n# B\n",
		"03-c.md": "---\nid: \"03\"\nstatus: open\ntype: task\nblocked_by: [\"01\"]\n---\n# C\n",
		"04-d.md": "---\nid: \"04\"\nstatus: open\ntype: task\nblocked_by: [\"02\", \"03\"]\n---\n# D\n",
		"05-e.md": "---\nid: \"05\"\nstatus: open\ntype: task\nblocked_by: [\"02\", \"03\"]\n---\n# E\n",
		"06-f.md": "---\nid: \"06\"\nstatus: open\ntype: task\nblocked_by: [\"04\", \"05\"]\n---\n# F\n",
	})

	// NewClaudeCompact below always writes its transcript via the real $HOME
	// (transcript.Path has no override hook), so this test keeps $HOME itself
	// pointed at the fixture via setHomeEnv rather than DepsOverrides.Home —
	// that way deps' default HOME resolution and NewClaudeCompact's agree on
	// the same directory.
	setHomeEnv(t, t.TempDir())
	const smartZone = 100
	var mu sync.Mutex
	var virtualTime time.Duration
	transcriptStart := time.Date(2026, 8, 4, 9, 0, 0, 0, time.UTC)
	for id, contract := range contracts {
		if id == "03" {
			continue
		}
		writeFakeTranscript(t, "", iterationWorktreePath(wtDir, epicName, id), "sess-"+iterLabel(epicName, id), transcriptStart,
			[3]any{"claude-sonnet-5", contract.tokens - 200, 0},
			[3]any{"claude-sonnet-5", contract.tokens - 100, 100},
		)
	}
	cCompact := herdrfake.NewClaudeCompact(t, iterationWorktreePath(wtDir, epicName, "03"), "sess-"+iterLabel(epicName, "03"), func() time.Duration {
		mu.Lock()
		defer mu.Unlock()
		return virtualTime
	}, smartZone, herdrfake.WithPairedPostCompactionTurn())

	status := map[string]string{}  // pane -> agent_status
	session := map[string]string{} // pane -> agent session id
	paneCwd := map[string]string{}
	openTabs := map[string]bool{}
	closedTabs := map[string]int{}
	var commitOrder []string
	var conflictResolutionSessionID string
	var ctrlCCalls, compactCalls, finishUpCalls int
	var bLandedDuringCCompact bool
	bLanded := make(chan struct{})
	dReady := make(chan struct{})
	eLanded := make(chan struct{})
	var bLandedOnce, dReadyOnce, eLandedOnce sync.Once

	handler := func(argv []string) ([]byte, int) {
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
			paneCwd[pane] = flagValue(argv, "--cwd")
			if label != conflictLabel("04") {
				openTabs[tabID] = true
			}
			mu.Unlock()
			return herdrfake.Result(map[string]any{
				"tab":       map[string]any{"tab_id": tabID, "label": label, "workspace_id": flagValue(argv, "--workspace")},
				"root_pane": map[string]any{"pane_id": pane},
			})
		case "tab close":
			tabID := argv[2]
			mu.Lock()
			closedTabs[tabID]++
			delete(openTabs, tabID)
			mu.Unlock()
			// B's and E's tabs only close once finishCleanup runs, i.e.
			// strictly after their commits have already landed on the
			// feature branch (finishIteration cherry-picks before
			// finishCleanup ever runs) — the point C's/D's own work must
			// wait behind to make B-before-C and E-before-D guaranteed, not
			// just racily likely.
			if tabID == "tab-"+iterLabel(epicName, "02") {
				bLandedOnce.Do(func() { close(bLanded) })
			}
			if tabID == "tab-"+iterLabel(epicName, "05") {
				eLandedOnce.Do(func() { close(eLanded) })
			}
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
			cwd := paneCwd[pane]
			mu.Unlock()
			if text == "/gx-resolving-merge-conflicts" {
				mu.Lock()
				conflictResolutionSessionID = sess
				mu.Unlock()
				if cwd != featurePath {
					return herdrfake.CommandError(fmt.Sprintf("conflict resolver cwd = %q, want %q", cwd, featurePath))
				}
				inProgress, err := git.CherryPickInProgress(cwd)
				if err != nil || !inProgress {
					return herdrfake.CommandError(fmt.Sprintf("conflict resolver cherry-pick state = %v, %v", inProgress, err))
				}
				conflicted, err := conflictedFiles(cwd)
				if err != nil || !slices.Equal(conflicted, []string{"shared.txt"}) {
					return herdrfake.CommandError(fmt.Sprintf("conflicted files = %v, %v", conflicted, err))
				}
				if err := os.WriteFile(filepath.Join(cwd, "shared.txt"), []byte(resolvedSharedContent), 0644); err != nil {
					return herdrfake.CommandError(err.Error())
				}
				if err := gitRun(cwd, "add", "shared.txt"); err != nil {
					return herdrfake.CommandError(err.Error())
				}
				if err := gitContinueCherryPick(cwd); err != nil {
					return herdrfake.CommandError(err.Error())
				}
				mu.Lock()
				status[pane] = "done"
				mu.Unlock()
				return agentJSON(pane, "working", sess)
			}
			if pane == "pane-"+iterLabel(epicName, "03") && text == "/compact" {
				mu.Lock()
				compactCalls++
				mu.Unlock()
				if err := cCompact.StartCompact(); err != nil {
					return herdrfake.CommandError(err.Error())
				}
				if compactStatus, err := cCompact.Status(); err != nil || compactStatus != "working" {
					return herdrfake.CommandError(fmt.Sprintf("C compact initial status = %q, %v", compactStatus, err))
				}
				<-bLanded
				if err := commitIterationWork(cwd, "03"); err != nil {
					t.Errorf("commitIterationWork(03): %v", err)
					return herdrfake.CommandError(err.Error())
				}
				mu.Lock()
				commitOrder = append(commitOrder, "03")
				bLandedDuringCCompact = true
				virtualTime += herdrfake.CompactDurationMs * time.Millisecond
				mu.Unlock()
				compactStatus, err := cCompact.Status()
				if err != nil || compactStatus != "idle" {
					return herdrfake.CommandError(fmt.Sprintf("C compact completed status = %q, %v", compactStatus, err))
				}
				return agentJSON(pane, "idle", sess)
			}
			if pane == "pane-"+iterLabel(epicName, "03") && strings.HasPrefix(text, "I stopped you because") {
				mu.Lock()
				finishUpCalls++
				mu.Unlock()
				if err := cCompact.AcceptFinishUp(); err != nil {
					return herdrfake.CommandError(err.Error())
				}
				mu.Lock()
				status[pane] = "idle"
				mu.Unlock()
				return agentJSON(pane, "working", sess)
			}
			if id, ok := ticketIDFromImplementPrompt(text); ok {
				if id == "03" {
					mu.Lock()
					status[pane] = "working"
					mu.Unlock()
					return agentJSON(pane, "working", sess)
				}
				if id == "04" {
					dReadyOnce.Do(func() { close(dReady) })
					<-eLanded
				}
				if id == "05" {
					<-dReady
				}
				dir := iterationWorktreePath(wtDir, epicName, id)
				if err := commitIterationWork(dir, id); err != nil {
					t.Errorf("commitIterationWork(%s): %v", id, err)
					return herdrfake.CommandError(err.Error())
				}
				mu.Lock()
				status[pane] = "idle"
				commitOrder = append(commitOrder, id)
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
			pane, key := argv[2], argv[3]
			if pane == "pane-"+iterLabel(epicName, "03") {
				mu.Lock()
				ctrlCCalls++
				mu.Unlock()
				if err := cCompact.SendKey(key); err != nil {
					return herdrfake.CommandError(err.Error())
				}
			}
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

	herdrfake.Start(t, handler)

	deps := testDeps()
	deps.Sleep = func(time.Duration) {}
	deps.VerifySkill = func(AgentKind, string) error { return nil }
	baseSHA, err := git.RevParse(repoDir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse base: %v", err)
	}

	if err := Run(RunOptions{EpicName: epicName, Skill: "implement", ScratchDir: scratchDir, RepoDir: repoDir, SmartZone: smartZone}, deps, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(commitOrder) != 6 || commitOrder[0] != "01" {
		t.Fatalf("commitOrder = %v, want A (01) created first", commitOrder)
	}
	if commitOrder[len(commitOrder)-1] != "06" {
		t.Fatalf("commitOrder = %v, want F (06) created last", commitOrder)
	}
	idxB, idxC := slices.Index(commitOrder, "02"), slices.Index(commitOrder, "03")
	if idxB == -1 || idxC == -1 || idxB > idxC {
		t.Errorf("commitOrder = %v, want B (02) created before C (03)", commitOrder)
	}
	idxD, idxE := slices.Index(commitOrder, "04"), slices.Index(commitOrder, "05")
	if idxD == -1 || idxE == -1 || idxE > idxD {
		t.Errorf("commitOrder = %v, want E (05) created before D (04)", commitOrder)
	}

	trailers, err := git.TrailerMap(featurePath, "HEAD", ticketTrailerKey)
	if err != nil {
		t.Fatalf("TrailerMap: %v", err)
	}
	shas := map[string]string{}
	for _, id := range []string{"01", "02", "03", "04", "05", "06"} {
		sha := trailers[ticketTrailerValue(epicName, id)]
		if sha == "" {
			t.Fatalf("landed trailers = %v, want a landed commit for ticket %s", trailers, id)
		}
		shas[id] = sha
	}
	// Clean feature history is exactly A, B, C, E, D, F: each ticket's
	// landed commit is an ancestor of the next in that order.
	landingOrder := []string{"01", "02", "03", "05", "04", "06"}
	for i := 1; i < len(landingOrder); i++ {
		prev, cur := landingOrder[i-1], landingOrder[i]
		if ok, err := git.IsAncestor(featurePath, shas[prev], shas[cur]); err != nil || !ok {
			t.Errorf("%s's landed commit is not an ancestor of %s's (ok=%v err=%v): feature history isn't A, B, C, E, D, F", prev, cur, ok, err)
		}
	}
	wantLandedSHAs := make([]string, 0, len(landingOrder))
	for _, id := range landingOrder {
		wantLandedSHAs = append(wantLandedSHAs, shas[id])
	}
	revList := exec.Command("git", "rev-list", "--reverse", baseSHA+"..HEAD")
	revList.Dir = featurePath
	rawLandedSHAs, err := revList.Output()
	if err != nil {
		t.Fatalf("git rev-list landed history: %v", err)
	}
	if got := strings.Fields(string(rawLandedSHAs)); !slices.Equal(got, wantLandedSHAs) {
		t.Errorf("landed history = %v, want exactly %v", got, wantLandedSHAs)
	}
	if got := gitSubject(t, featurePath, shas["04"]); got != "iteration 04" {
		t.Errorf("D subject = %q, want %q", got, "iteration 04")
	}
	shared, err := os.ReadFile(filepath.Join(featurePath, "shared.txt"))
	if err != nil {
		t.Fatalf("ReadFile shared.txt: %v", err)
	}
	if string(shared) != resolvedSharedContent {
		t.Errorf("shared.txt = %q, want %q", shared, resolvedSharedContent)
	}
	for id, sha := range shas {
		show := exec.Command("git", "show", "-s", "--format=%B", sha)
		show.Dir = featurePath
		message, err := show.Output()
		if err != nil {
			t.Fatalf("git show %s: %v", sha, err)
		}
		trailerTokens := contracts[id].tokens
		if contracts[id].trailerTokens != 0 {
			trailerTokens = contracts[id].trailerTokens
		}
		wantTrailers := map[string]string{
			ticketTrailerKey:  ticketTrailerValue(epicName, id),
			tokensTrailerKey:  strconv.Itoa(trailerTokens),
			elapsedTrailerKey: strconv.Itoa(contracts[id].elapsed) + "s",
		}
		for key, wantValue := range wantTrailers {
			var values []string
			for _, line := range strings.Split(string(message), "\n") {
				if value, ok := strings.CutPrefix(line, key+": "); ok {
					values = append(values, value)
				}
			}
			if len(values) != 1 || values[0] != wantValue {
				t.Errorf("commit %s trailer %s = %v, want exactly [%s]", sha, key, values, wantValue)
			}
		}
	}

	for id, contract := range contracts {
		path := filepath.Join(scratchDir, epicName, "issues", contract.filename)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile %s: %v", contract.filename, err)
		}
		frontmatter, err := schema.ParseTicketFromRaw(string(raw), path)
		if err != nil {
			t.Fatalf("ParseTicketFromRaw(%s): %v", contract.filename, err)
		}
		if err := schema.Validate(frontmatter); err != nil {
			t.Errorf("Validate(%s): %v", contract.filename, err)
		}
		blockedBy := make([]string, len(frontmatter.BlockedBy))
		for i, blocker := range frontmatter.BlockedBy {
			blockedBy[i] = fmt.Sprint(blocker)
		}
		parts := strings.SplitN(string(raw), "---\n", 3)
		if len(parts) != 3 {
			t.Fatalf("ticket %s lost its frontmatter boundaries", id)
		}
		if fmt.Sprint(frontmatter.ID) != id || fmt.Sprint(frontmatter.Type) != "task" || fmt.Sprint(frontmatter.Status) != "done" || parts[2] != contract.body || !slices.Equal(blockedBy, contract.blockedBy) {
			t.Errorf("ticket %s = %+v with body %q, want preserved id/type/dependencies/body and done status", id, frontmatter, parts[2])
		}
		if frontmatter.ActualContextWindow != contract.tokens || frontmatter.ElapsedTime != contract.elapsed || frontmatter.Compactions != contract.compactions {
			t.Errorf("ticket %s metrics = (%d tokens, %ds, %d compactions), want (%d tokens, %ds, %d compactions)", id, frontmatter.ActualContextWindow, frontmatter.ElapsedTime, frontmatter.Compactions, contract.tokens, contract.elapsed, contract.compactions)
		}
	}

	events, ok, err := readEvents(scratchDir, epicName)
	if err != nil || !ok {
		t.Fatalf("readEvents: ok=%v err=%v", ok, err)
	}
	eventCounts := map[string]int{}
	eventPositions := map[string]int{}
	cherrySHAs := map[string]string{}
	conflictCounts := map[string]int{}
	conflictResolvedSHA := ""
	conflictResolvedSessionID := ""
	lifecycleEventCount := 0
	for i, event := range events {
		if event.Type == eventNeedsAnswer || event.Type == eventNeedsRepair {
			t.Errorf("unexpected recovery residue event: %+v", event)
		}
		if event.Type == eventSmartZoneRecoveryFailed {
			t.Errorf("unexpected smart-zone recovery failure: %+v", event)
		}
		key := event.Type + "/" + event.Ticket
		switch event.Type {
		case eventIterationStarted, eventIterationFinished, eventCherryPicked:
			lifecycleEventCount++
			eventCounts[key]++
			eventPositions[key] = i
			if event.Type == eventCherryPicked {
				cherrySHAs[event.Ticket] = event.SHA
			}
		case eventConflictHit, eventConflictResolved:
			conflictCounts[key]++
			if event.Type == eventConflictResolved && event.Ticket == "04" {
				conflictResolvedSHA = event.SHA
				conflictResolvedSessionID = event.AgentSession
			}
		}
	}
	if lifecycleEventCount != len(contracts)*3 {
		t.Errorf("lifecycle event count = %d, want exactly one start, finish, and cherry-pick per ticket", lifecycleEventCount)
	}
	for id := range contracts {
		started := eventIterationStarted + "/" + id
		finished := eventIterationFinished + "/" + id
		cherryPicked := eventCherryPicked + "/" + id
		for _, key := range []string{started, finished, cherryPicked} {
			if eventCounts[key] != 1 {
				t.Errorf("event %s count = %d, want exactly one", key, eventCounts[key])
			}
		}
		if eventPositions[started] >= eventPositions[finished] || eventPositions[finished] >= eventPositions[cherryPicked] {
			t.Errorf("ticket %s lifecycle positions = start:%d finish:%d cherry:%d", id, eventPositions[started], eventPositions[finished], eventPositions[cherryPicked])
		}
		if cherrySHAs[id] != shas[id] {
			t.Errorf("ticket %s cherry-picked SHA = %q, want landed SHA %q", id, cherrySHAs[id], shas[id])
		}
	}
	for _, eventType := range []string{eventConflictHit, eventConflictResolved} {
		key := eventType + "/04"
		if conflictCounts[key] != 1 {
			t.Errorf("event %s count = %d, want exactly one", key, conflictCounts[key])
		}
	}
	if conflictResolvedSHA != shas["04"] {
		t.Errorf("D conflict-resolved SHA = %q, want landed SHA %q", conflictResolvedSHA, shas["04"])
	}
	mu.Lock()
	wantConflictResolutionSessionID := conflictResolutionSessionID
	mu.Unlock()
	if conflictResolvedSessionID != wantConflictResolutionSessionID {
		t.Errorf("D conflict-resolved session = %q, want resolver session %q", conflictResolvedSessionID, wantConflictResolutionSessionID)
	}
	for _, edge := range [][2]string{{"01", "02"}, {"01", "03"}, {"02", "04"}, {"03", "04"}, {"02", "05"}, {"03", "05"}, {"04", "06"}, {"05", "06"}} {
		blockerCherry := eventPositions[eventCherryPicked+"/"+edge[0]]
		dependentStart := eventPositions[eventIterationStarted+"/"+edge[1]]
		if blockerCherry >= dependentStart {
			t.Errorf("dependency lifecycle %s -> %s = cherry:%d start:%d", edge[0], edge[1], blockerCherry, dependentStart)
		}
	}

	for _, id := range []string{"01", "02", "03", "04", "05", "06"} {
		path := iterationWorktreePath(wtDir, epicName, id)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("iteration worktree %s for ticket %s still exists, want removed", path, id)
		}
		branch := iterBranch(epicName, id)
		if _, err := git.RevParse(featurePath, branch); err == nil {
			t.Errorf("iteration branch %s for ticket %s still exists, want deleted", branch, id)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(openTabs) != 0 {
		t.Errorf("openTabs = %v, want all iteration tabs closed", openTabs)
	}
	if len(closedTabs) != 7 {
		t.Errorf("closedTabs = %v, want exactly 7 (one per ticket plus D's conflict-resolution tab)", closedTabs)
	}
	if ctrlCCalls != 1 || compactCalls != 1 || finishUpCalls != 1 {
		t.Errorf("C recovery calls = (ctrl-c:%d compact:%d finish-up:%d), want exactly one each", ctrlCCalls, compactCalls, finishUpCalls)
	}
	if !bLandedDuringCCompact {
		t.Error("B did not land while C was compacting")
	}
	for id := range contracts {
		tabID := "tab-" + iterLabel(epicName, id)
		if closedTabs[tabID] != 1 {
			t.Errorf("tab %s close count = %d, want exactly one", tabID, closedTabs[tabID])
		}
	}
	if _, err := os.Stat(featurePath); err != nil {
		t.Errorf("feature worktree removed: %v", err)
	}
	if _, err := git.RevParse(featurePath, epicName); err != nil {
		t.Errorf("feature branch removed: %v", err)
	}
}

// TestRun_ProductionRealGit_ParkThenResumeReusesBranch pins ticket 13: an
// iteration that parks with a needs-answer report keeps its branch across
// the park (only its worktree/tab are dropped), and a resume that reattaches
// to that surviving branch lands both the pre-park and post-resume commits
// together, in order, in a single cherry-pick — never two, and never
// silently dropping the pre-park commit by basing off the feature branch's
// (by-then-advanced) tip instead of the merge base.
func TestRun_ProductionRealGit_ParkThenResumeReusesBranch(t *testing.T) {
	realGitTimeoutWatchdog(t, realGitTestTimeout)
	const epicName = "epic"
	repoDir := testutil.TempRepo(t)
	wtDir := testWorktreeDir(t, repoDir)
	featurePath := filepath.Join(wtDir, epicName)

	scratchDir := writeEpic(t, epicName, map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
	})
	ticketPath := filepath.Join(scratchDir, epicName, "issues", "01-a.md")
	iterationPath := iterationWorktreePath(wtDir, epicName, "01")
	branch := iterBranch(epicName, "01")

	home := t.TempDir()

	var mu sync.Mutex
	phase := "pre-park"

	handler := func(argv []string) ([]byte, int) {
		if len(argv) < 2 {
			return herdrfake.CommandError(fmt.Sprintf("command too short: %v", argv))
		}
		switch argv[0] + " " + argv[1] {
		case "workspace list":
			return herdrfake.Result(map[string]any{"workspaces": []any{}})
		case "workspace create":
			return herdrfake.Result(map[string]any{"workspace": map[string]any{"workspace_id": "ws1"}})
		case "tab create":
			label := realGitFlagValue(argv, "--label")
			return herdrfake.Result(map[string]any{
				"tab":       map[string]any{"tab_id": "tab-" + label, "label": label, "workspace_id": realGitFlagValue(argv, "--workspace")},
				"root_pane": map[string]any{"pane_id": "pane-" + label},
			})
		case "tab close":
			return []byte(`{"result":null}`), 0
		case "tab list":
			return herdrfake.Result(map[string]any{"tabs": []any{}})
		case "agent start":
			pane := realGitFlagValue(argv, "--pane")
			return agentJSON(pane, "idle", "sess-"+pane)
		case "agent prompt":
			pane, text := argv[2], argv[3]
			if _, ok := ticketIDFromImplementPrompt(text); ok {
				mu.Lock()
				cur := phase
				mu.Unlock()
				if cur == "pre-park" {
					if err := commitIterationWork(iterationPath, "pre"); err != nil {
						return herdrfake.CommandError(err.Error())
					}
					if err := updateTicket(ticketPath, func(tk *schema.Ticket) {
						tk.IterationStatus = schema.IterationStatusNeedsAnswer
					}); err != nil {
						return herdrfake.CommandError(err.Error())
					}
				} else {
					if err := commitIterationWork(iterationPath, "post"); err != nil {
						return herdrfake.CommandError(err.Error())
					}
				}
			}
			return agentJSON(pane, "idle", "sess-"+pane)
		case "agent wait":
			return agentJSON(argv[2], "idle", "sess-"+argv[2])
		case "agent send-keys":
			return agentJSON(argv[2], "idle", "sess-"+argv[2])
		case "agent read":
			return []byte(""), 0
		default:
			return herdrfake.CommandError("unimplemented command: " + argv[0] + " " + argv[1])
		}
	}
	herdrfake.Start(t, handler)

	deps := testDepsWithOverrides(DepsOverrides{Home: home})
	deps.Sleep = func(time.Duration) {}
	deps.VerifySkill = func(AgentKind, string) error { return nil }
	cherryPickCalls := 0
	realCherryPickRange := deps.CherryPickRange
	deps.CherryPickRange = func(dir, fromExclusive, toInclusive string) error {
		cherryPickCalls++
		return realCherryPickRange(dir, fromExclusive, toInclusive)
	}

	runUntilParked(t, RunOptions{EpicName: epicName, Skill: "implement", ScratchDir: scratchDir, RepoDir: repoDir}, deps, &recordingSink{})

	raw, err := os.ReadFile(ticketPath)
	if err != nil {
		t.Fatalf("ReadFile ticket after park: %v", err)
	}
	if !strings.Contains(string(raw), "status: needs-answer") {
		t.Fatalf("ticket after park = %s, want needs-answer", raw)
	}
	if _, err := os.Stat(iterationPath); !os.IsNotExist(err) {
		t.Errorf("iteration worktree %s still exists after park, want removed", iterationPath)
	}
	if _, err := git.RevParse(featurePath, branch); err != nil {
		t.Errorf("iteration branch %s missing after park, want it to survive for the resume: %v", branch, err)
	}
	if cherryPickCalls != 0 {
		t.Errorf("cherry-pick calls after park = %d, want 0 (an adopted needs-answer report must not land anything)", cherryPickCalls)
	}

	// Simulate a human answering the question and gx picking the ticket back
	// up: cleared back to open (Claim, run by the next scheduler pass, clears
	// iteration_status the same way a fresh claim always does).
	if err := SetStatus(ticketPath, "open"); err != nil {
		t.Fatalf("SetStatus open: %v", err)
	}
	mu.Lock()
	phase = "post-resume"
	mu.Unlock()

	if err := Run(RunOptions{EpicName: epicName, Skill: "implement", ScratchDir: scratchDir, RepoDir: repoDir}, deps, noopEventSink{}); err != nil {
		t.Fatalf("Run() (resume) error = %v", err)
	}

	raw, err = os.ReadFile(ticketPath)
	if err != nil {
		t.Fatalf("ReadFile ticket after resume: %v", err)
	}
	if !strings.Contains(string(raw), "status: done") {
		t.Errorf("ticket after resume = %s, want done", raw)
	}

	for _, name := range []string{"pre.txt", "post.txt"} {
		if _, err := os.Stat(filepath.Join(featurePath, name)); err != nil {
			t.Errorf("landed feature branch missing %s (both pre-park and post-resume commits must land together): %v", name, err)
		}
	}
	if _, err := git.RevParse(featurePath, branch); err == nil {
		t.Errorf("iteration branch %s still exists after landing, want deleted", branch)
	}
	if cherryPickCalls != 1 {
		t.Errorf("total cherry-pick calls = %d, want exactly 1 (both commits landed in a single pick)", cherryPickCalls)
	}
}

func TestRun_ProductionRealGit_CodexCompactsThenCompletes(t *testing.T) {
	realGitTimeoutWatchdog(t, realGitTestTimeout)
	const (
		epicName  = "epic"
		smartZone = 150000
		sessionID = "codex-session-01"
	)
	repoDir := testutil.TempRepo(t)
	wtDir := testWorktreeDir(t, repoDir)
	scratchDir := writeEpic(t, epicName, map[string]string{
		"01-compact.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# Compact recovery\n",
	})

	home := t.TempDir()
	codexHome := filepath.Join(home, ".codex")
	cwd := iterationWorktreePath(wtDir, epicName, "01")
	sessionPath := filepath.Join(home, ".codex", "sessions", "2026", "08", "04", "rollout-"+sessionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(sessionPath), 0755); err != nil {
		t.Fatalf("MkdirAll Codex session: %v", err)
	}
	initialSession := fmt.Sprintf(
		"{\"type\":\"session_meta\",\"payload\":{\"id\":%q,\"cwd\":%q}}\n"+
			"{\"type\":\"event_msg\",\"payload\":{\"type\":\"token_count\",\"info\":{\"last_token_usage\":{\"input_tokens\":%d}}}}\n",
		sessionID, cwd, smartZone+1,
	)
	if err := os.WriteFile(sessionPath, []byte(initialSession), 0644); err != nil {
		t.Fatalf("WriteFile Codex session: %v", err)
	}

	ticketPath := filepath.Join(scratchDir, epicName, "issues", "01-compact.md")
	assertClaimed := func(stage string) {
		raw, err := os.ReadFile(ticketPath)
		if err != nil {
			t.Errorf("ReadFile ticket during %s: %v", stage, err)
			return
		}
		if !strings.Contains(string(raw), "status: claimed") {
			t.Errorf("ticket during %s = %s, want claimed", stage, raw)
		}
	}
	appendLowOccupancy := func() error {
		f, err := os.OpenFile(sessionPath, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = fmt.Fprintf(f, "{\"type\":\"event_msg\",\"payload\":{\"type\":\"token_count\",\"info\":{\"last_token_usage\":{\"input_tokens\":%d}}}}\n", smartZone/2)
		return err
	}

	s := herdrfake.NewState(t)
	phase := "starting"
	compactComplete := false
	lowOccupancyWritten := false
	ctrlCCalls := 0
	compactCalls := 0
	closedTabs := 0
	var recoveryPrompts []string

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
		closedTabs++
		return nil, herdrfake.Identities{TabID: "tab-01"}, nil
	})
	s.Register("tab", "list", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{"tabs": []any{}}, herdrfake.Identities{}, nil
	})
	s.Register("agent", "start", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{"agent": map[string]any{
			"pane_id": "pane-01", "agent_status": "idle", "agent_session": map[string]any{"value": sessionID},
		}}, herdrfake.Identities{PaneID: "pane-01", SessionID: sessionID}, nil
	})
	s.Register("agent", "prompt", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		text := argv[3]
		switch {
		case strings.HasPrefix(text, "/implement "), strings.HasPrefix(text, "$implement "):
			phase = "implementing"
			return map[string]any{"agent": map[string]any{"pane_id": "pane-01", "agent_status": "working", "agent_session": map[string]any{"value": sessionID}}}, herdrfake.Identities{PaneID: "pane-01", SessionID: sessionID}, nil
		case text == "/compact":
			assertClaimed("compact prompt")
			compactCalls++
			recoveryPrompts = append(recoveryPrompts, "/compact")
			phase = "compact-blocked"
			return map[string]any{"agent": map[string]any{"pane_id": "pane-01", "agent_status": "blocked", "agent_session": map[string]any{"value": sessionID}}}, herdrfake.Identities{PaneID: "pane-01", SessionID: sessionID}, nil
		case strings.Contains(text, "please finish up quickly"):
			assertClaimed("finish-up prompt")
			if !compactComplete {
				t.Errorf("finish-up prompt arrived before compact completion")
			}
			recoveryPrompts = append(recoveryPrompts, "finish-up")
			if err := commitIterationWork(cwd, "01"); err != nil {
				return nil, herdrfake.Identities{}, err
			}
			phase = "finishing-up"
			return map[string]any{"agent": map[string]any{"pane_id": "pane-01", "agent_status": "working", "agent_session": map[string]any{"value": sessionID}}}, herdrfake.Identities{PaneID: "pane-01", SessionID: sessionID}, nil
		default:
			return nil, herdrfake.Identities{}, fmt.Errorf("unexpected prompt %q", text)
		}
	})
	s.Register("agent", "wait", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		until := parseUntil(argv[3:])
		switch phase {
		case "starting":
			return map[string]any{"agent": map[string]any{"pane_id": "pane-01", "agent_status": "idle", "agent_session": map[string]any{"value": sessionID}}}, herdrfake.Identities{PaneID: "pane-01", SessionID: sessionID}, nil
		case "implementing":
			assertClaimed("smart-zone breach")
			return nil, herdrfake.Identities{}, fmt.Errorf("timed out waiting for agent status")
		case "compact-blocked":
			assertClaimed("compact start confirmation")
			if !slices.Equal(until, []string{"working"}) {
				t.Errorf("compact start Until = %v, want [working]", until)
			}
			phase = "compacting"
			return map[string]any{"agent": map[string]any{"pane_id": "pane-01", "agent_status": "working", "agent_session": map[string]any{"value": sessionID}}}, herdrfake.Identities{PaneID: "pane-01", SessionID: sessionID}, nil
		case "compacting":
			assertClaimed("compact completion")
			compactComplete = true
			phase = "compacted"
			return map[string]any{"agent": map[string]any{"pane_id": "pane-01", "agent_status": "idle", "agent_session": map[string]any{"value": sessionID}}}, herdrfake.Identities{PaneID: "pane-01", SessionID: sessionID}, nil
		case "finishing-up":
			assertClaimed("post-compact occupancy")
			if err := appendLowOccupancy(); err != nil {
				return nil, herdrfake.Identities{}, err
			}
			lowOccupancyWritten = true
			phase = "post-compact"
			return nil, herdrfake.Identities{}, fmt.Errorf("timed out waiting for agent status")
		case "post-compact":
			phase = "done"
			return map[string]any{"agent": map[string]any{"pane_id": "pane-01", "agent_status": "idle", "agent_session": map[string]any{"value": sessionID}}}, herdrfake.Identities{PaneID: "pane-01", SessionID: sessionID}, nil
		case "done":
			return map[string]any{"agent": map[string]any{"pane_id": "pane-01", "agent_status": "idle", "agent_session": map[string]any{"value": sessionID}}}, herdrfake.Identities{PaneID: "pane-01", SessionID: sessionID}, nil
		default:
			return nil, herdrfake.Identities{}, fmt.Errorf("unexpected wait in phase %q", phase)
		}
	})
	s.Register("agent", "send-keys", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		for _, key := range argv[3:] {
			if key == "ctrl+c" {
				ctrlCCalls++
			}
		}
		return map[string]any{"agent": map[string]any{"pane_id": "pane-01", "agent_status": "working", "agent_session": map[string]any{"value": sessionID}}}, herdrfake.Identities{PaneID: "pane-01", SessionID: sessionID}, nil
	})
	s.Register("agent", "read", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return "", herdrfake.Identities{}, nil
	})
	herdrfake.StartState(t, s)

	deps := testDepsWithOverrides(DepsOverrides{Home: home, CodexHome: codexHome})
	deps.PreflightAgent = func(AgentKind) error { return nil }
	deps.VerifySkill = func(AgentKind, string) error { return nil }
	deps.Sleep = func(time.Duration) {}
	deps.Now = func() time.Time { return time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC) }
	sink := &compactRecoverySink{}
	if err := Run(RunOptions{
		EpicName: epicName, Agent: AgentCodex, Skill: "implement", ScratchDir: scratchDir,
		RepoDir: repoDir, SmartZone: smartZone,
	}, deps, sink); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if ctrlCCalls != 1 || compactCalls != 1 {
		t.Errorf("recovery calls = ctrl+c:%d compact:%d, want exactly one each", ctrlCCalls, compactCalls)
	}
	if !slices.Equal(recoveryPrompts, []string{"/compact", "finish-up"}) {
		t.Errorf("recovery prompts = %v, want compact then finish-up", recoveryPrompts)
	}
	if !lowOccupancyWritten {
		t.Error("lower post-compact occupancy event was not written")
	}
	sink.mu.Lock()
	phases := append([]string{}, sink.phases...)
	sink.mu.Unlock()
	if !slices.Equal(phases, []string{"compact-started", "finishing-up", "recovered"}) {
		t.Errorf("compact phases = %v, want compact-started, finishing-up, recovered", phases)
	}

	rawTicket, err := os.ReadFile(ticketPath)
	if err != nil {
		t.Fatalf("ReadFile completed ticket: %v", err)
	}
	if !strings.Contains(string(rawTicket), "status: done") {
		t.Errorf("completed ticket = %s, want done", rawTicket)
	}
	if strings.Contains(string(rawTicket), "compactions:") {
		t.Errorf("completed Codex ticket unexpectedly persisted compactions: %s", rawTicket)
	}
	if closedTabs != 1 {
		t.Errorf("closed tabs = %d, want exactly one", closedTabs)
	}

	events, ok, err := readEvents(scratchDir, epicName)
	if err != nil || !ok {
		t.Fatalf("readEvents: ok=%v err=%v", ok, err)
	}
	wantEventOrder := []string{eventPausedSmartZone, eventResumed, eventIterationFinished, eventCherryPicked}
	var gotEventOrder []string
	for _, event := range events {
		switch event.Type {
		case eventNeedsAnswer, eventNeedsRepair, eventSmartZoneRecoveryFailed:
			t.Errorf("unexpected recovery residue event: %+v", event)
		case eventPausedSmartZone, eventResumed, eventIterationFinished, eventCherryPicked:
			gotEventOrder = append(gotEventOrder, event.Type)
		}
	}
	if !slices.Equal(gotEventOrder, wantEventOrder) {
		t.Errorf("recovery event order = %v, want %v", gotEventOrder, wantEventOrder)
	}
}
