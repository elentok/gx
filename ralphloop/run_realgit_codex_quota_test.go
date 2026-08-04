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

	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/herdrfake"
)

// TestRun_ProductionRealGit_CodexQuotaBackfillRecovers drives ticket 28's
// account-wide Codex quota scenario: while ticket 01 sits paused on a
// typed quota exhaustion, a worker slot frees up (ticket 02 finishes) and
// the ready backfill ticket 03 must not be claimed until 01's typed resume
// — proving Gate.claimIfRunning's "any label paused blocks every new claim"
// contract holds even when a slot is free, and that recovery converges on
// stale-but-clearing evidence instead of hanging. Uses a raw
// herdrfake.Start(t, handler) (not NewState/RegisterCodexRollout, whose
// phase machine is hardcoded for the compact flow, not quota) in the same
// style as TestRun_ProductionRealGit_AThenBAndCConcurrently, with all three
// tickets driven by the Codex agent so quota detection is exercised for
// real via Deps.ReadCodexRateLimit.
func TestRun_ProductionRealGit_CodexQuotaBackfillRecovers(t *testing.T) {
	const epicName = "epic"
	repoDir := testutil.TempRepo(t)

	scratchDir := writeEpic(t, epicName, map[string]string{
		"01-quota.md":    "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# Quota\n",
		"02-filler.md":   "---\nid: \"02\"\nstatus: open\ntype: task\n---\n# Filler\n",
		"03-backfill.md": "---\nid: \"03\"\nstatus: open\ntype: task\n---\n# Backfill\n",
	})

	t.Setenv("HOME", t.TempDir())

	var mu sync.Mutex
	status := map[string]string{}  // pane -> agent_status
	session := map[string]string{} // pane -> agent session id
	openTabs := map[string]bool{}
	closedTabs := map[string]bool{}
	waitCounts := map[string]int{} // pane -> number of "agent wait" calls seen so far
	tab02Closed := make(chan struct{})
	var tab02ClosedOnce sync.Once

	pane01 := "pane-" + iterLabel("01")
	dir01 := iterationWorktreePath(repoDir, epicName, "01")

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
			if tabID == "tab-"+iterLabel("02") {
				// Guaranteed-post-landing signal, same idiom as
				// TestRun_ProductionRealGit_AThenBAndCConcurrently's bLanded:
				// 02's tab only closes once finishCleanup runs, strictly after
				// 02's commit already landed and its worker slot is free.
				tab02ClosedOnce.Do(func() { close(tab02Closed) })
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
				if id == "01" {
					// Ticket 01's commit is deferred until quota clears (see
					// "agent wait" below): it must still look "working" when
					// the typed quota hits, not already finished.
					mu.Lock()
					status[pane] = "working"
					mu.Unlock()
					return agentJSON(pane, "working", sess)
				}
				dir := iterationWorktreePath(repoDir, epicName, id)
				if err := commitIterationWork(dir, id); err != nil {
					t.Errorf("commitIterationWork(%s): %v", id, err)
					return herdrfake.CommandError(err.Error())
				}
				mu.Lock()
				status[pane] = "idle"
				mu.Unlock()
			}
			return agentJSON(pane, "working", sess)

		case "agent wait":
			pane := argv[2]
			until := parseUntil(argv[3:])
			mu.Lock()
			waitCounts[pane]++
			count := waitCounts[pane]
			cur, sess := status[pane], session[pane]
			mu.Unlock()
			if pane == pane01 && count == 3 {
				// The recovery-internal AgentWait(Until=[idle,done,working,
				// blocked]) inside recoverCodexRateLimit (waitforfinish.go):
				// force 01's commit here, since there's no other prompt to
				// trigger it after quota clears, and "working" is in this
				// until list so a generic match would short-circuit without
				// ever committing.
				if err := commitIterationWork(dir01, "01"); err != nil {
					t.Errorf("commitIterationWork(01): %v", err)
					return herdrfake.CommandError(err.Error())
				}
				mu.Lock()
				status[pane] = "idle"
				sess = session[pane]
				mu.Unlock()
				return agentJSON(pane, "idle", sess)
			}
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

	var rateLimitMu sync.Mutex
	rateLimitCalls := 0
	deps := DefaultDeps()
	deps.Sleep = func(time.Duration) {}
	deps.VerifySkill = func(AgentKind, string) error { return nil }
	deps.PreflightAgent = func(AgentKind) error { return nil }
	deps.Now = func() time.Time { return time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC) }
	deps.ReadCodexRateLimit = func(cwd, sessionID string) (codexsession.RateLimit, bool, error) {
		rateLimitMu.Lock()
		rateLimitCalls++
		call := rateLimitCalls
		rateLimitMu.Unlock()

		switch call {
		case 1:
			// Initial detection, triggers recoverCodexRateLimit.
			return codexsession.RateLimit{Quota: "usage"}, true, nil
		case 2:
			// The core "slot free while paused" assertion: wait until 02's
			// tab has closed (guaranteed-post-landing) and confirm 03 hasn't
			// been claimed yet, strictly before Gate.ForceResume fires inside
			// recoverCodexRateLimit — correct regardless of goroutine
			// scheduling, since it's read here rather than after the fact.
			<-tab02Closed
			raw, err := os.ReadFile(filepath.Join(scratchDir, epicName, "issues", "03-backfill.md"))
			if err != nil {
				t.Errorf("ReadFile ticket 03: %v", err)
			} else if !strings.Contains(string(raw), "status: open") {
				t.Errorf("ticket 03 while 01 is quota-paused = %s, want still open (not claimed)", raw)
			}
			return codexsession.RateLimit{Quota: "usage"}, true, nil // stale read
		case 3:
			return codexsession.RateLimit{Quota: "usage"}, true, nil // second stale read
		default:
			return codexsession.RateLimit{}, false, nil // genuinely cleared
		}
	}

	var out bytes.Buffer
	if err := Run(RunOptions{
		EpicName: epicName, Agent: AgentCodex, Skill: "implement", ScratchDir: scratchDir, RepoDir: repoDir,
	}, deps, NewTextEventSink(&out)); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if rateLimitCalls < 4 {
		t.Errorf("ReadCodexRateLimit calls = %d, want at least 4 (converging within codexRateLimitMaxRepolls)", rateLimitCalls)
	}

	featurePath := filepath.Join(repoDir, epicName)
	trailers, err := git.TrailerMap(featurePath, "HEAD", ticketTrailerKey)
	if err != nil {
		t.Fatalf("TrailerMap: %v", err)
	}
	for _, id := range []string{"01", "02", "03"} {
		if trailers[ticketTrailerValue(epicName, id)] == "" {
			t.Errorf("landed trailers = %v, want a landed commit for ticket %s", trailers, id)
		}
	}

	for _, name := range []string{"01-quota.md", "02-filler.md", "03-backfill.md"} {
		raw, err := os.ReadFile(filepath.Join(scratchDir, epicName, "issues", name))
		if err != nil {
			t.Fatalf("ReadFile %s: %v", name, err)
		}
		if !strings.Contains(string(raw), "status: done") {
			t.Errorf("%s not marked done:\n%s", name, raw)
		}
	}

	for _, id := range []string{"01", "02", "03"} {
		path := iterationWorktreePath(repoDir, epicName, id)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("iteration worktree %s for ticket %s still exists, want removed", path, id)
		}
		branch := iterBranch(epicName, id)
		if _, err := git.RevParse(featurePath, branch); err == nil {
			t.Errorf("iteration branch %s for ticket %s still exists, want deleted", branch, id)
		}
	}

	mu.Lock()
	openCount, closedCount := len(openTabs), len(closedTabs)
	mu.Unlock()
	if openCount != 0 {
		t.Errorf("openTabs = %d, want all iteration tabs closed", openCount)
	}
	if closedCount != 3 {
		t.Errorf("closedTabs = %d, want exactly 3 (one per ticket)", closedCount)
	}

	if _, err := os.Stat(featurePath); err != nil {
		t.Errorf("feature worktree gone: %v", err)
	}
	if _, err := git.RevParse(featurePath, epicName); err != nil {
		t.Errorf("feature branch gone: %v", err)
	}

	events, ok, err := readEvents(scratchDir, epicName)
	if err != nil || !ok {
		t.Fatalf("readEvents: ok=%v err=%v", ok, err)
	}
	pausedIdx, resumedIdx, started03Idx, finished03Idx := -1, -1, -1, -1
	for i, event := range events {
		if event.Type == eventNeedsInfo || event.Type == eventNeedsAttention || event.Type == eventSmartZoneRecoveryFailed {
			t.Errorf("unexpected recovery residue event: %+v", event)
		}
		switch {
		case event.Type == eventPausedRateLimit && event.Ticket == "01" && pausedIdx == -1:
			pausedIdx = i
		case event.Type == eventResumed && event.Ticket == "01" && resumedIdx == -1:
			resumedIdx = i
		case event.Type == eventIterationStarted && event.Ticket == "03" && started03Idx == -1:
			started03Idx = i
		case event.Type == eventIterationFinished && event.Ticket == "03" && finished03Idx == -1:
			finished03Idx = i
		}
	}
	if pausedIdx == -1 || resumedIdx == -1 || started03Idx == -1 || finished03Idx == -1 ||
		!(pausedIdx < resumedIdx && resumedIdx < started03Idx && started03Idx < finished03Idx) {
		t.Errorf("event order = paused-rate-limit(01):%d resumed(01):%d iteration-started(03):%d iteration-finished(03):%d, want strictly increasing",
			pausedIdx, resumedIdx, started03Idx, finished03Idx)
	}
}

// TestRun_ProductionRealGit_CodexContextAndQuotaConcurrentlyResolve drives
// ticket 29's combined scenario: ticket 01 (context) and ticket 02 (quota)
// both claim a worker slot at once (defaultMaxParallel==2, see loop.go), so
// ticket 03 (backfill) starts out ready but unclaimed. Ticket 01 hits a
// smart-zone context breach, compacts, and lands — freeing a slot — while
// ticket 02 sits on a typed Codex quota pause the whole time; per
// Gate.claimIfRunning (pause.go), that pause blocks *every* new claim
// (including 03) regardless of the free slot, until 02's typed quota resume.
// Combines TestRun_ProductionRealGit_CodexContextRecoveryLandsAndCleansUp's
// smart-zone flow with TestRun_ProductionRealGit_CodexQuotaBackfillRecovers'
// quota/backfill flow inside a single Run() call, on one raw
// herdrfake.Start(t, handler) that dispatches per-pane: 01 runs a hand-rolled
// phase machine mirroring herdrfake.CodexRollout's (ctrl-c -> /compact ->
// finish-up -> idle) with a real synthetic rollout JSONL so context detection
// goes through the actual codexsession reader; 02 and 03 reuse the quota
// test's generic pane bookkeeping.
func TestRun_ProductionRealGit_CodexContextAndQuotaConcurrentlyResolve(t *testing.T) {
	const (
		epicName  = "epic"
		smartZone = 150000
	)
	repoDir := testutil.TempRepo(t)
	scratchDir := writeEpic(t, epicName, map[string]string{
		"01-context.md":  "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# Context\n",
		"02-quota.md":    "---\nid: \"02\"\nstatus: open\ntype: task\n---\n# Quota\n",
		"03-backfill.md": "---\nid: \"03\"\nstatus: open\ntype: task\n---\n# Backfill\n",
	})

	t.Setenv("HOME", t.TempDir())
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)

	dir01 := iterationWorktreePath(repoDir, epicName, "01")
	dir02 := iterationWorktreePath(repoDir, epicName, "02")

	pane01 := "pane-" + iterLabel("01")
	pane02 := "pane-" + iterLabel("02")

	const contextSessionID = "sess-context-flagship"
	rolloutPath := filepath.Join(
		codexHome, "sessions", "2026", "08", "04",
		"rollout-2026-08-04T12-00-00-"+contextSessionID+".jsonl",
	)

	const (
		initialContextTokens   = smartZone + 1
		initialTotalTokens     = smartZone + 5_000
		compactedContextTokens = smartZone / 2
		compactedTotalTokens   = smartZone + 20_000
		finalContextTokens     = smartZone / 2
		finalTotalTokens       = smartZone + 25_000
	)

	var mu sync.Mutex
	status := map[string]string{}
	session := map[string]string{}
	openTabs := map[string]bool{}
	closedTabs := map[string]bool{}
	waitCounts := map[string]int{}
	tab01Closed := make(chan struct{})
	var tab01ClosedOnce sync.Once

	// contextPhase drives pane01's hand-rolled Codex phase machine;
	// contextLine counts the JSONL lines already written to rolloutPath.
	contextPhase := "idle"
	contextLine := 0
	contextStarted := false

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
			if label == iterLabel("01") {
				// The context ticket's own work is already "done" by the time
				// its tab exists — same idiom as
				// TestRun_ProductionRealGit_CodexContextRecoveryLandsAndCleansUp,
				// which commits at tab-create so the rest of the scenario can
				// focus purely on breach detection and recovery.
				if err := commitIterationWork(dir01, "01"); err != nil {
					t.Errorf("commitIterationWork(01): %v", err)
					return herdrfake.CommandError(err.Error())
				}
			}
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
			if tabID == "tab-"+iterLabel("01") {
				// Guaranteed-post-landing signal, same idiom as
				// TestRun_ProductionRealGit_CodexQuotaBackfillRecovers'
				// tab02Closed: 01's tab only closes once finishCleanup runs,
				// strictly after 01's commit already landed and its worker
				// slot is free.
				tab01ClosedOnce.Do(func() { close(tab01Closed) })
			}
			return []byte(`{"result":null}`), 0
		case "tab list":
			return herdrfake.Result(map[string]any{"tabs": []any{}})

		case "agent start":
			pane := realGitFlagValue(argv, "--pane")
			mu.Lock()
			defer mu.Unlock()
			if pane == pane01 {
				session[pane01] = contextSessionID
				if !contextStarted {
					if err := writeCodexSessionMeta(rolloutPath, contextLine, contextSessionID, dir01); err != nil {
						t.Errorf("writeCodexSessionMeta: %v", err)
						return herdrfake.CommandError(err.Error())
					}
					contextLine++
					if err := writeCodexUsageLine(rolloutPath, contextLine, initialContextTokens, initialTotalTokens); err != nil {
						t.Errorf("writeCodexUsageLine(initial): %v", err)
						return herdrfake.CommandError(err.Error())
					}
					contextLine++
					contextStarted = true
				}
				status[pane01] = "idle"
				return agentJSON(pane01, "idle", contextSessionID)
			}
			sess := "sess-" + argv[2]
			status[pane] = "idle"
			session[pane] = sess
			return agentJSON(pane, "idle", sess)

		case "agent prompt":
			pane, text := argv[2], argv[3]
			if pane == pane01 {
				return contextPromptResponse(pane, &contextPhase, text, contextSessionID)
			}
			mu.Lock()
			sess := session[pane]
			mu.Unlock()
			if id, ok := ticketIDFromImplementPrompt(text); ok {
				if id == "02" {
					// Ticket 02's commit is deferred until quota clears (see
					// "agent wait" below): it must still look "working" when
					// the typed quota hits, not already finished.
					mu.Lock()
					status[pane] = "working"
					mu.Unlock()
					return agentJSON(pane, "working", sess)
				}
				dir := iterationWorktreePath(repoDir, epicName, id)
				if err := commitIterationWork(dir, id); err != nil {
					t.Errorf("commitIterationWork(%s): %v", id, err)
					return herdrfake.CommandError(err.Error())
				}
				mu.Lock()
				status[pane] = "idle"
				mu.Unlock()
			}
			return agentJSON(pane, "working", sess)

		case "agent wait":
			pane := argv[2]
			if pane == pane01 {
				return contextWaitResponse(pane, &contextPhase, rolloutPath, &contextLine, contextSessionID,
					compactedContextTokens, compactedTotalTokens, finalContextTokens, finalTotalTokens)
			}
			until := parseUntil(argv[3:])
			mu.Lock()
			waitCounts[pane]++
			count := waitCounts[pane]
			cur, sess := status[pane], session[pane]
			mu.Unlock()
			if pane == pane02 && count == 3 {
				// The recovery-internal AgentWait(Until=[idle,done,working,
				// blocked]) inside recoverCodexRateLimit (waitforfinish.go):
				// force 02's commit here, since there's no other prompt to
				// trigger it after quota clears, and "working" is in this
				// until list so a generic match would short-circuit without
				// ever committing.
				if err := commitIterationWork(dir02, "02"); err != nil {
					t.Errorf("commitIterationWork(02): %v", err)
					return herdrfake.CommandError(err.Error())
				}
				mu.Lock()
				status[pane] = "idle"
				sess = session[pane]
				mu.Unlock()
				return agentJSON(pane, "idle", sess)
			}
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

	var rateLimitMu sync.Mutex
	rateLimitCalls := 0
	deps := DefaultDeps()
	deps.Sleep = func(time.Duration) {}
	deps.VerifySkill = func(AgentKind, string) error { return nil }
	deps.PreflightAgent = func(AgentKind) error { return nil }
	deps.Now = func() time.Time { return time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC) }
	deps.ReadCodexRateLimit = func(cwd, sessionID string) (codexsession.RateLimit, bool, error) {
		if sessionID == contextSessionID {
			// Ticket 01's own quota reads go through the real reader against
			// its synthetic rollout file, which never reports an exhausted
			// quota (zero rate_limits) — its recovery path is smart-zone
			// context, not quota.
			return codexsession.LastRateLimit(cwd, sessionID)
		}

		rateLimitMu.Lock()
		rateLimitCalls++
		call := rateLimitCalls
		rateLimitMu.Unlock()

		switch call {
		case 1:
			// Initial detection, triggers recoverCodexRateLimit for 02.
			return codexsession.RateLimit{Quota: "usage"}, true, nil
		case 2:
			// The core "slot free while paused" assertion: wait until 01's
			// tab has closed (guaranteed-post-landing) and confirm 03 hasn't
			// been claimed yet, strictly before Gate.ForceResume fires
			// inside recoverCodexRateLimit — correct regardless of goroutine
			// scheduling, since it's read here rather than after the fact.
			<-tab01Closed
			raw, err := os.ReadFile(filepath.Join(scratchDir, epicName, "issues", "03-backfill.md"))
			if err != nil {
				t.Errorf("ReadFile ticket 03: %v", err)
			} else if !strings.Contains(string(raw), "status: open") {
				t.Errorf("ticket 03 while 02 is quota-paused = %s, want still open (not claimed)", raw)
			}
			return codexsession.RateLimit{Quota: "usage"}, true, nil // stale read
		case 3:
			return codexsession.RateLimit{Quota: "usage"}, true, nil // second stale read
		default:
			return codexsession.RateLimit{}, false, nil // genuinely cleared
		}
	}

	sink := &compactRecoverySink{}
	if err := Run(RunOptions{
		EpicName: epicName, Agent: AgentCodex, Skill: "implement", ScratchDir: scratchDir,
		RepoDir: repoDir, SmartZone: smartZone,
	}, deps, sink); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if rateLimitCalls < 4 {
		t.Errorf("ReadCodexRateLimit calls (ticket 02) = %d, want at least 4 (converging within codexRateLimitMaxRepolls)", rateLimitCalls)
	}

	// Acceptance criterion: the context ticket's session rollout and Herdr
	// identity match the real iteration cwd, verified through the same
	// production reader reattachment uses.
	if ok, err := codexsession.VerifyIdentity(dir01, contextSessionID); err != nil || !ok {
		t.Errorf("VerifyIdentity(%s, %s) = %v, %v; want the rollout's cwd to match the real iteration worktree", dir01, contextSessionID, ok, err)
	}
	stats, ok, err := codexsession.ReadStats(dir01, contextSessionID)
	if err != nil || !ok || stats.PeakContext != initialContextTokens || stats.TotalTokens != finalTotalTokens {
		t.Errorf("ReadStats = %+v, %v, %v; want peak context %d and final total %d", stats, ok, err, initialContextTokens, finalTotalTokens)
	}

	// Acceptance criterion: exactly one compact recovery cycle ran for
	// ticket 01, in the right order.
	sink.mu.Lock()
	phases := append([]string{}, sink.phases...)
	sink.mu.Unlock()
	if !slices.Equal(phases, []string{"compact-started", "finishing-up", "recovered"}) {
		t.Errorf("compact phases = %v, want compact-started, finishing-up, recovered", phases)
	}

	featurePath := filepath.Join(repoDir, epicName)
	trailers, err := git.TrailerMap(featurePath, "HEAD", ticketTrailerKey)
	if err != nil {
		t.Fatalf("TrailerMap: %v", err)
	}
	for _, id := range []string{"01", "02", "03"} {
		if trailers[ticketTrailerValue(epicName, id)] == "" {
			t.Errorf("landed trailers = %v, want a landed commit for ticket %s", trailers, id)
		}
	}

	for _, name := range []string{"01-context.md", "02-quota.md", "03-backfill.md"} {
		raw, err := os.ReadFile(filepath.Join(scratchDir, epicName, "issues", name))
		if err != nil {
			t.Fatalf("ReadFile %s: %v", name, err)
		}
		if !strings.Contains(string(raw), "status: done") {
			t.Errorf("%s not marked done:\n%s", name, raw)
		}
	}
	// Codex has no reliable persisted compaction-count signal, so a
	// completed Codex ticket must never persist one.
	raw01, err := os.ReadFile(filepath.Join(scratchDir, epicName, "issues", "01-context.md"))
	if err != nil {
		t.Fatalf("ReadFile 01-context.md: %v", err)
	}
	if strings.Contains(string(raw01), "compactions:") {
		t.Errorf("completed Codex ticket 01 unexpectedly persisted compactions: %s", raw01)
	}

	for _, id := range []string{"01", "02", "03"} {
		path := iterationWorktreePath(repoDir, epicName, id)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("iteration worktree %s for ticket %s still exists, want removed", path, id)
		}
		branch := iterBranch(epicName, id)
		if _, err := git.RevParse(featurePath, branch); err == nil {
			t.Errorf("iteration branch %s for ticket %s still exists, want deleted", branch, id)
		}
	}

	mu.Lock()
	openCount, closedCount := len(openTabs), len(closedTabs)
	mu.Unlock()
	if openCount != 0 {
		t.Errorf("openTabs = %d, want all iteration tabs closed", openCount)
	}
	if closedCount != 3 {
		t.Errorf("closedTabs = %d, want exactly 3 (one per ticket)", closedCount)
	}

	if _, err := os.Stat(featurePath); err != nil {
		t.Errorf("feature worktree gone: %v", err)
	}
	if _, err := git.RevParse(featurePath, epicName); err != nil {
		t.Errorf("feature branch gone: %v", err)
	}

	events, ok, err := readEvents(scratchDir, epicName)
	if err != nil || !ok {
		t.Fatalf("readEvents: ok=%v err=%v", ok, err)
	}
	pausedSmartZone01, resumed01, finished01 := -1, -1, -1
	pausedRateLimit02, resumed02 := -1, -1
	started03, finished03 := -1, -1
	for i, event := range events {
		if event.Type == eventNeedsInfo || event.Type == eventNeedsAttention || event.Type == eventSmartZoneRecoveryFailed {
			t.Errorf("unexpected recovery residue event: %+v", event)
		}
		switch {
		case event.Type == eventPausedSmartZone && event.Ticket == "01" && pausedSmartZone01 == -1:
			pausedSmartZone01 = i
		case event.Type == eventResumed && event.Ticket == "01" && resumed01 == -1:
			resumed01 = i
		case event.Type == eventIterationFinished && event.Ticket == "01" && finished01 == -1:
			finished01 = i
		case event.Type == eventPausedRateLimit && event.Ticket == "02" && pausedRateLimit02 == -1:
			pausedRateLimit02 = i
		case event.Type == eventResumed && event.Ticket == "02" && resumed02 == -1:
			resumed02 = i
		case event.Type == eventIterationStarted && event.Ticket == "03" && started03 == -1:
			started03 = i
		case event.Type == eventIterationFinished && event.Ticket == "03" && finished03 == -1:
			finished03 = i
		}
	}
	if pausedSmartZone01 == -1 || resumed01 == -1 || finished01 == -1 ||
		!(pausedSmartZone01 < resumed01 && resumed01 < finished01) {
		t.Errorf("ticket 01 event order = paused-smart-zone:%d resumed:%d finished:%d, want strictly increasing",
			pausedSmartZone01, resumed01, finished01)
	}
	// Acceptance criterion: the context ticket recovers and frees a slot
	// while the quota ticket remains paused — 02 was already paused before
	// 01 finished, and didn't resume until after.
	if pausedRateLimit02 == -1 || resumed02 == -1 ||
		!(pausedRateLimit02 < finished01 && finished01 < resumed02) {
		t.Errorf("acceptance: ticket 02 paused-rate-limit:%d, ticket 01 finished:%d, ticket 02 resumed:%d — want 02 paused before 01 finished, and still paused (resumed after) when 01 finished",
			pausedRateLimit02, finished01, resumed02)
	}
	// Acceptance criterion: the third ticket stays open until the typed
	// quota resume event.
	if started03 == -1 || finished03 == -1 || !(resumed02 < started03 && started03 < finished03) {
		t.Errorf("ticket 03 event order = resumed(02):%d started:%d finished:%d, want 03 only claimed after 02's typed resume",
			resumed02, started03, finished03)
	}
}
