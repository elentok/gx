// Package sleepguard holds the repo-wide guard against fixed time.Sleep calls
// in tests (docs/specs better-tests, story 48).
package sleepguard

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// sleepCall matches a call. A value use such as `c.Sleep = time.Sleep` has no
// opening parenthesis and does not match.
var sleepCall = regexp.MustCompile(`\btime\.Sleep\(`)

// selfFile is this guard's own test file; its samples contain sleeps.
const selfFile = "sleepguard/sleepguard_test.go"

// allowed maps a test file to the number of time.Sleep calls it may hold. To
// remove a sleep, replace it with a condition wait and lower the count. Keep
// the map sorted, one entry per line.
var allowed = map[string]int{
	"agentrunner/herdrrunner/conformance_test.go":          1,
	"brief/brief_test.go":                                  2, // poll loops with a deadline
	"check/check_test.go":                                  1, // poll loop with a deadline waiting on the go step
	"check/lock_test.go":                                   2, // a held-lock stand-in that sleeps until killed, and a poll loop with a deadline waiting on the lock state
	"cmd/brief_cleancache_test.go":                         1, // negative wait: go clean does not run while a build holds the gate
	"cmd/brief_test.go":                                    2, // poll loop with a deadline waiting on the fake go; a fixture test source that is slow on purpose
	"cmd/check_record_test.go":                             3, // fixture test sources that are slow on purpose, run by a child go test
	"cmd/agent_permission_request_test.go":                 3, // one poll loop with a deadline; negative waits: no request is filed; a stale decision is ignored
	"cmd/brief_retention_test.go":                          1,
	"codexdoctor/session_test.go":                          1, // poll loop with a deadline waiting on the rollout
	"codexsetup/setup_test.go":                             1, // negative wait: a rerun leaves the files' mtimes alone
	"nativerunner/pty_codex_prompt_test.go":                3, // two poll loops with a deadline; negative wait: a queued prompt is not submitted
	"nativerunner/pty_codex_test.go":                       2, // poll loops with a deadline
	"nativerunner/pty_launch_test.go":                      1, // negative wait: the launch must still be waiting for delivery when cancelled
	"nativerunner/pty_codexapproval_test.go":               2, // poll loops with a deadline
	"server/ticketwrite/blocked_pause_test.go":             3, // negative waits: no pause before the dwell ends, after a re-check, or on a usage limit
	"server/budget/extrausage_internal_test.go":            2, // poll loop with a deadline; negative wait: no second notice is sent
	"server/servertest/longticket_test.go":                 1, // a delayed transcript write
	"server/health/startincident_test.go":                  2, // poll loops with a deadline
	"testutil/agentfake/codex_test.go":                     1, // poll loop with a deadline
	"testutil/agentfake/codexdesign_test.go":               1, // poll loop with a deadline
	"testutil/agentfake/codexhooks_test.go":                4, // poll loops with a deadline; negative waits: a held hook keeps the dialog open
	"testutil/gxfixture/spend_test.go":                     1, // poll loop with a deadline
	"cmd/server_agents_test.go":                            1,
	"cmd/server_disk_status_test.go":                       1,
	"cmd/server_logs_test.go":                              1,
	"e2e/agents_terminal_test.go":                          6,
	"e2e/idle_while_working_test.go":                       1,
	"e2e/server_lifecycle_test.go":                         3, // one is a poll loop with a deadline waiting on a condition
	"harness/harness_test.go":                              1,
	"nativerunner/answer_test.go":                          1,
	"nativerunner/conformance_test.go":                     1,
	"nativerunner/headless_test.go":                        3,
	"nativerunner/holder_test.go":                          1,
	"nativerunner/holderanswer_test.go":                    1, // negative wait: a request mismatch sends no key
	"nativerunner/holderattach_test.go":                    1,
	"nativerunner/holderwatch_test.go":                     1,
	"nativerunner/pty_pending_test.go":                     1,
	"nativerunner/pty_ratelimit_test.go":                   1, // negative wait: another API error never marks the session limited
	"nativerunner/pty_test.go":                             1,
	"ralphloop/chat_eventsink_test.go":                     4,
	"ralphloop/commitless_test.go":                         1, // negative wait: a kept worktree is not removed and no wait is scheduled
	"ralphloop/claim_test.go":                              1,
	"ralphloop/eventlog_test.go":                           2,
	"ralphloop/resumeticket_test.go":                       1,
	"ralphloop/slack_transport_test.go":                    4, // 15 renamed slack_eventsink_test.go and dropped its entry by mistake
	"ralphloop/telegram_transport_test.go":                 7, // same rename as slack_transport_test.go
	"server/budget/grace_test.go":                          5,
	"server/budget/limits_test.go":                         2,
	"server/claimhold_test.go":                             1, // negative wait: a herdr event would show within a few retries
	"server/configreload_chat_test.go":                     2,
	"server/configreload_test.go":                          2,
	"server/deadlock_test.go":                              2,
	"server/disk/diskhealth_test.go":                       1, // negative wait: failed readings never leave the waiting state
	"server/disk/diskhold_test.go":                         3,
	"server/disk/diskwatch_test.go":                        3,
	"server/merge/handmerged_test.go":                      1,
	"server/headless_block_reclaim_test.go":                2,
	"server/health/healthprocs_test.go":                    4,
	"server/merge/latefinish_test.go":                      1,
	"server/limits/limithistory_codex_test.go":             2, // settle wait for the event coalescer; received-at has second granularity
	"server/epics/map_done_test.go":                        1,
	"server/merge/merge_finish_test.go":                    1,
	"server/merge/mergedecide_test.go":                     3,
	"server/merge/mergefork_test.go":                       1,
	"server/migrate_gate_test.go":                          1, // negative wait: a gate that leaks would have acted by now
	"server/projects/migrate_test.go":                      1, // negative wait: a step that ignored the gate would have run by now
	"server/merge/missingtrunk_test.go":                    2,
	"server/queue/oneoffsweep_test.go":                     2,
	"server/queue/pause_test.go":                           1,
	"server/reclaim_waiting_test.go":                       1,
	"server/ratelimit_claim_hold_test.go":                  1, // negative wait: a claim would show within a few ticks
	"server/ratelimit_hold_run_test.go":                    1, // negative wait: a launch would show within a few ticks
	"server/recovery/recovery_notify_test.go":              4,
	"server/recovery/recovery_investigate_test.go":         1,
	"server/recovery/recovery_gates_test.go":               6,
	"server/retry_prompt_test.go":                          1, // negative wait: no second interrupt or prompt follows the retry
	"server/runner_test.go":                                1,
	"server/server_notify_test.go":                         1,
	"server/server_test.go":                                1,
	"server/sessionchatwatch_test.go":                      2,
	"server/sessions/prompt/action_test.go":                7,
	"server/sessions/prompt/autoadvance_countdown_test.go": 6,
	"server/sessions/prompt/autoadvance_guard_test.go":     2,
	"server/sessions/prompt/keys_test.go":                  2, // negative waits: no nudge follows a person's key; a guarded key keeps the countdown
	"server/sessions/prompt/sessionprompt_test.go":         5, // four negative waits: no nudge or Continue follows a typed / command; a failure is not raised within the grace window
	"server/sessions/prompt/firstprompt_failure_test.go":   1,
	"server/sessions/prompt/firstprompt_launch_test.go":    1, // negative wait: a resend would come within a few ResendEvery
	"server/sessions/prompt/firstprompt_test.go":           3,
	"server/sessions/sessions_banner_test.go":              1,
	"server/sessions/sessions_failstart_test.go":           1,
	"server/sessions/sessions_failure_banner_test.go":      1,
	"server/sessions/sessions_test.go":                     1,
	"server/limits/tokenpoll_test.go":                      2, // poll loop with a deadline; negative wait: no poll while a migration is held
	"server/merge/trunkconflict_claim_test.go":             1,
	"server/servertest/ptyrunner_test.go":                  3, // negative waits: a wrong reap or kill would come within a few polls
	"server/merge/trunklock_test.go":                       1,
	"server/unavailable_test.go":                           1,
	"storecommit/storecommit_test.go":                      3,
	"testutil/agentfake/tui_test.go":                       10, // poll loop with a deadline; negative waits: a slow-marker status must stay busy; an unarmed slash prompt starts no turn; an ignored Allow keeps the dialog open; a lone Esc leaves the turn alone; a stall lands before the prompt; the registry delay (1s) must pass before the retry; the launch-record opt-out writes no user record
	"testutil/agentfake/worker_test.go":                    1,
	"testutil/gxfixture/cachegate_test.go":                 5, // two poll loops with a deadline; negative waits: the gate holds the clean
	"testutil/gxfixture/design_test.go":                    5,
	"testutil/gxfixture/disk_test.go":                      2,
	"testutil/gxfixture/harness_test.go":                   1,
	"testutil/gxfixture/limitburn_test.go":                 4, // three poll loops with a deadline; a settle wait so the poll sees the first line
	"testutil/gxfixture/migrate_test.go":                   2, // poll loop with a deadline; negative wait: the hold must keep the server migrating
	"testutil/gxfixture/ready_test.go":                     1,
	"testutil/gxfixture/restart_test.go":                   1, // poll loop with a deadline waiting on the running-agent count
	"testutil/gxfixture/united_test.go":                    1,
	"testutil/herdrfake/state_test.go":                     2,
	"testutil/runnerfake/runnerfake_test.go":               1,
	"tickets/stamp_lock_test.go":                           1,
	"tokenledger/ledger_test.go":                           3, // mtime granularity: a no-op write must leave the file's mtime alone
	"ui/agents/attach_test.go":                             1,
	"ui/agents/fake_client_test.go":                        1,
	"ui/agents/focus_test.go":                              2,
	"ui/app/title_bar_e2e_test.go":                         1,
	"ui/components/runner_pty_test.go":                     4,
	"ui/components/runner_test.go":                         2,
	"ui/log/e2e_test.go":                                   2,
	"ui/push/push_test.go":                                 1,
	"ui/status/e2e_test.go":                                1,
}

// countSleeps counts the time.Sleep calls in src. Lines that start with a
// comment do not count.
func countSleeps(src string) int {
	n := 0
	for line := range strings.SplitSeq(src, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		n += len(sleepCall.FindAllString(line, -1))
	}
	return n
}

// compare returns one problem per file whose count differs from its allowance.
func compare(found, allow map[string]int) []string {
	var problems []string
	for path, n := range found {
		if n > allow[path] {
			problems = append(problems, fmt.Sprintf(
				"%s: allowed %d time.Sleep call(s), found %d. Replace the sleep with a condition wait (poll with a timeout). "+
					"Only a negative wait (proving nothing happens) may be added to the allowlist, with a reason.",
				path, allow[path], n))
		}
	}
	for path, want := range allow {
		got := found[path]
		switch {
		case got >= want:
		case got == 0:
			problems = append(problems, fmt.Sprintf("%s: allowlist says %d, found none; drop the entry", path, want))
		default:
			problems = append(problems, fmt.Sprintf("%s: allowlist says %d, found %d; lower the count", path, want, got))
		}
	}
	sort.Strings(problems)
	return problems
}

func trackedTestFiles(t *testing.T) []string {
	t.Helper()
	out, err := exec.Command("git", "-C", "..", "ls-files", "*_test.go").Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	return strings.Fields(string(out))
}

func TestNoNewSleeps(t *testing.T) {
	found := map[string]int{}
	for _, path := range trackedTestFiles(t) {
		if path == selfFile {
			continue
		}
		src, err := os.ReadFile(filepath.Join("..", path))
		if os.IsNotExist(err) {
			continue // tracked but deleted in the working tree
		}
		if err != nil {
			t.Fatal(err)
		}
		if n := countSleeps(string(src)); n > 0 {
			found[path] = n
		}
	}
	for _, p := range compare(found, allowed) {
		t.Error(p)
	}
}

func TestCountSleeps(t *testing.T) {
	cases := []struct {
		name, src string
		want      int
	}{
		{"call", "time.Sleep(d)", 1},
		{"call with spaces in args", "\ttime.Sleep(50 * time.Millisecond)", 1},
		{"value use", "c.Sleep = time.Sleep", 0},
		{"commented out", "// time.Sleep(d)", 0},
		{"indented comment", "\t// time.Sleep(d)", 0},
		{"two lines", "time.Sleep(a)\nx()\ntime.Sleep(b)", 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := countSleeps(c.src); got != c.want {
				t.Fatalf("countSleeps = %d, want %d", got, c.want)
			}
		})
	}
}

func TestCompare(t *testing.T) {
	cases := []struct {
		name         string
		found, allow map[string]int
		wantContains string // empty means no problem
	}{
		{"exact match", map[string]int{"a_test.go": 2}, map[string]int{"a_test.go": 2}, ""},
		{"over", map[string]int{"a_test.go": 3}, map[string]int{"a_test.go": 2}, "allowed 2 time.Sleep call(s), found 3"},
		{"under", map[string]int{"a_test.go": 1}, map[string]int{"a_test.go": 2}, "lower the count"},
		{"gone", map[string]int{}, map[string]int{"a_test.go": 2}, "drop the entry"},
		{"not listed", map[string]int{"b_test.go": 1}, map[string]int{}, "allowed 0 time.Sleep call(s), found 1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := strings.Join(compare(c.found, c.allow), "\n")
			if c.wantContains == "" {
				if got != "" {
					t.Fatalf("unexpected problems: %s", got)
				}
				return
			}
			if !strings.Contains(got, c.wantContains) {
				t.Fatalf("problems %q do not contain %q", got, c.wantContains)
			}
		})
	}
}
