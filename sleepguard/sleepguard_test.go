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
	"agentrunner/herdrrunner/conformance_test.go":   1,
	"brief/brief_test.go":                           1,
	"cmd/brief_retention_test.go":                   1,
	"cmd/server_agents_test.go":                     1,
	"cmd/server_logs_test.go":                       1,
	"e2e/agents_terminal_test.go":                   6,
	"e2e/idle_while_working_test.go":                1,
	"e2e/server_lifecycle_test.go":                  2,
	"harness/harness_test.go":                       1,
	"nativerunner/answer_test.go":                   1,
	"nativerunner/conformance_test.go":              1,
	"nativerunner/headless_test.go":                 3,
	"nativerunner/holder_test.go":                   1,
	"nativerunner/holderattach_test.go":             1,
	"nativerunner/holderwatch_test.go":              1,
	"nativerunner/pty_pending_test.go":              1,
	"nativerunner/pty_test.go":                      1,
	"ralphloop/chat_eventsink_test.go":              4,
	"ralphloop/claim_test.go":                       1,
	"ralphloop/eventlog_test.go":                    2,
	"ralphloop/loop_agent_test.go":                  1,
	"ralphloop/loop_test.go":                        1,
	"ralphloop/mergequeue_test.go":                  2,
	"ralphloop/permit_test.go":                      2,
	"ralphloop/resumeticket_test.go":                1,
	"ralphloop/run_ratelimit_test.go":               1,
	"ralphloop/run_realgit_codex_quota_test.go":     1,
	"ralphloop/run_realgit_watchdog_test.go":        1,
	"ralphloop/slack_eventsink_test.go":             4,
	"ralphloop/telegram_eventsink_test.go":          7,
	"server/budget_limits_test.go":                  2,
	"server/configreload_chat_test.go":              2,
	"server/configreload_grace_test.go":             5,
	"server/configreload_test.go":                   2,
	"server/conflictagent_test.go":                  2,
	"server/conversationsweep_test.go":              1,
	"server/deadlock_test.go":                       2,
	"server/designcard_test.go":                     1,
	"server/epicmerge_test.go":                      1,
	"server/epicnotice_test.go":                     1,
	"server/frictionstamp_test.go":                  1,
	"server/handmerged_test.go":                     1,
	"server/headless_block_reclaim_test.go":         2,
	"server/headless_block_test.go":                 1,
	"server/latefinish_test.go":                     2,
	"server/map_done_test.go":                       1,
	"server/merge_finish_test.go":                   1,
	"server/mergedecide_test.go":                    3,
	"server/mergefork_test.go":                      1,
	"server/missingtrunk_test.go":                   4,
	"server/oneoffs_epic_test.go":                   1,
	"server/oneoffsweep_test.go":                    5,
	"server/pause_test.go":                          2,
	"server/prune_route_test.go":                    1,
	"server/reattach_test.go":                       4,
	"server/reclaim_launching_test.go":              1,
	"server/reclaim_route_test.go":                  2,
	"server/reclaim_waiting_test.go":                1,
	"server/recovery_notify_test.go":                4,
	"server/recovery_test.go":                       7,
	"server/retrohook_test.go":                      1,
	"server/runner_flush_test.go":                   1,
	"server/runner_test.go":                         1,
	"server/server_notify_test.go":                  2,
	"server/server_test.go":                         2,
	"server/sessionchatwatch_test.go":               3,
	"server/sessionprompt_test.go":                  1,
	"server/sessions_action_test.go":                8,
	"server/sessions_afk_test.go":                   1,
	"server/sessions_autoadvance_countdown_test.go": 8,
	"server/sessions_autoadvance_guard_test.go":     2,
	"server/sessions_banner_test.go":                3,
	"server/sessions_failstart_test.go":             1,
	"server/sessions_failure_banner_test.go":        1,
	"server/sessions_firstprompt_failure_test.go":   1,
	"server/sessions_firstprompt_test.go":           3,
	"server/sessions_grilling_test.go":              2,
	"server/sessions_test.go":                       6,
	"server/sessions_unread_test.go":                1,
	"server/slackhook_test.go":                      3,
	"server/stop_test.go":                           1,
	"server/store_commit_test.go":                   2,
	"server/ticketwrites_test.go":                   1,
	"server/trunkconflict_claim_test.go":            1,
	"server/trunklock_test.go":                      1,
	"server/unavailable_test.go":                    3,
	"server/verbs_route_test.go":                    2,
	"storecommit/storecommit_test.go":               3,
	"testutil/agentfake/harness_test.go":            1,
	"testutil/agentfake/tui_test.go":                4,
	"testutil/agentfake/worker_test.go":             1,
	"testutil/gxfixture/main_test.go":               8,
	"testutil/herdrfake/state_test.go":              2,
	"testutil/runnerfake/runnerfake_test.go":        1,
	"tickets/stamp_lock_test.go":                    1,
	"ui/agents/attach_test.go":                      1,
	"ui/agents/fake_client_test.go":                 1,
	"ui/agents/focus_test.go":                       2,
	"ui/app/title_bar_e2e_test.go":                  1,
	"ui/components/runner_pty_test.go":              4,
	"ui/components/runner_test.go":                  2,
	"ui/log/e2e_test.go":                            2,
	"ui/push/push_test.go":                          1,
	"ui/status/e2e_test.go":                         1,
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
