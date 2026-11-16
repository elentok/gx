package transcript

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func bashUseLine(id, ts, input string) string {
	return fmt.Sprintf(`{"type":"assistant","timestamp":%q,"message":{"id":"m-%s","content":[{"type":"tool_use","id":%q,"name":"Bash","input":%s}]}}`, ts, id, id, input)
}

// launchResultLine is the result of a background launch or a call moved to the
// background on timeout.
func launchResultLine(id, taskID, ts, text string) string {
	return fmt.Sprintf(`{"type":"user","timestamp":%q,"message":{"content":[{"type":"tool_result","tool_use_id":%q,"content":%q}]},"toolUseResult":{"backgroundTaskId":%q}}`, ts, id, text, taskID)
}

func plainResultLine(id, ts string, isError bool, text, toolUseResult string) string {
	return fmt.Sprintf(`{"type":"user","timestamp":%q,"message":{"content":[{"type":"tool_result","tool_use_id":%q,"is_error":%t,"content":%q}]},"toolUseResult":%s}`, ts, id, isError, text, toolUseResult)
}

func enqueueLine(taskID, status, summary, ts string) string {
	body := "<task-notification>\n<task-id>" + taskID + "</task-id>\n"
	if status != "" {
		body += "<status>" + status + "</status>\n"
	}
	body += "<summary>" + summary + "</summary>\n<output-file>/out/" + taskID + ".output</output-file>\n</task-notification>"
	return fmt.Sprintf(`{"type":"queue-operation","operation":"enqueue","timestamp":%q,"content":%q}`, ts, body)
}

func turnEndLine(ts string) string {
	return fmt.Sprintf(`{"type":"system","subtype":"turn_duration","timestamp":%q,"durationMs":1}`, ts)
}

func readRecords(t *testing.T, lines ...string) []TaskRecord {
	t.Helper()
	recs, _, err := ReadTaskRecords(writeTranscript(t, lines...))
	if err != nil {
		t.Fatal(err)
	}
	return recs
}

func recByLabel(t *testing.T, recs []TaskRecord, label string) TaskRecord {
	t.Helper()
	for _, r := range recs {
		if r.Label == label {
			return r
		}
	}
	t.Fatalf("no record %q in %+v", label, recs)
	return TaskRecord{}
}

const (
	t0 = "2026-05-01T12:00:00Z"
	t1 = "2026-05-01T12:00:01Z"
	t2 = "2026-05-01T12:05:00Z"
	t3 = "2026-05-01T12:06:41Z"
)

func TestReadTaskRecords_BackgroundShellStartsAtLaunchLine(t *testing.T) {
	t.Parallel()
	recs := readRecords(t,
		bashUseLine("u1", t0, `{"command":"make","description":"build","run_in_background":true}`),
		launchResultLine("u1", "b1", t1, "Command running in background with ID: b1. Output is being written to: /tmp/b1.output"),
		enqueueLine("b1", "completed", `Background command "build" completed (exit code 0)`, t2),
	)
	if len(recs) != 1 {
		t.Fatalf("recs = %+v", recs)
	}
	r := recs[0]
	if r.Kind != TaskKindShell || r.Label != "build" || r.Command != "make" || !r.StartedAt.Equal(mustParseTime(t0)) {
		t.Errorf("rec = %+v", r)
	}
	if r.End == nil || !r.End.At.Equal(mustParseTime(t2)) || r.End.Outcome != TaskOutcomeSuccess || r.End.Source != TaskEndNotification {
		t.Errorf("end = %+v", r.End)
	}
	if r.ExitCode == nil || *r.ExitCode != 0 {
		t.Errorf("exit code = %v", r.ExitCode)
	}
	if r.OutputPath != "/out/b1.output" {
		t.Errorf("output path = %q, want the notification's", r.OutputPath)
	}
}

func TestReadTaskRecords_LaunchResultGivesOutputPath(t *testing.T) {
	t.Parallel()
	recs := readRecords(t,
		bashUseLine("u1", t0, `{"command":"make","run_in_background":true}`),
		launchResultLine("u1", "b1", t1, "Command running in background with ID: b1. Output is being written to: /tmp/b1.output"),
	)
	if recs[0].OutputPath != "/tmp/b1.output" || recs[0].End != nil {
		t.Errorf("rec = %+v", recs[0])
	}
}

func TestReadTaskRecords_TimedOutCallBecomesShellAtItsLaunchTime(t *testing.T) {
	t.Parallel()
	use := bashUseLine("u1", t0, `{"command":"sleep 999","description":"slow"}`)
	pending := readRecords(t, use)
	if pending[0].Kind != TaskKindCall {
		t.Errorf("kind before the result = %q, want call", pending[0].Kind)
	}
	recs := readRecords(t, use, launchResultLine("u1", "b1", t3, "Command timed out (ID: b1). Output is being written to: /tmp/b1.output"))
	r := recs[0]
	if r.Kind != TaskKindShell || !r.StartedAt.Equal(mustParseTime(t0)) || r.OutputPath != "/tmp/b1.output" {
		t.Errorf("rec = %+v", r)
	}
}

func TestReadTaskRecords_ShellEndRules(t *testing.T) {
	t.Parallel()
	launch := func(id, task, label string) []string {
		return []string{
			bashUseLine(id, t0, `{"command":"c","description":"`+label+`","run_in_background":true}`),
			launchResultLine(id, task, t1, "ok"),
		}
	}
	tests := map[string]struct {
		tail    []string
		outcome string
		at      string // empty means no end
		code    int
	}{
		"failed":      {[]string{enqueueLine("task-1", "failed", "failed with exit code 3", t2)}, TaskOutcomeFailure, t2, 3},
		"killed":      {[]string{enqueueLine("task-1", "killed", "was stopped", t2)}, TaskOutcomeKilled, t2, -1},
		"stopped":     {[]string{enqueueLine("task-1", "stopped", "was stopped", t2)}, TaskOutcomeKilled, t2, -1},
		"no status":   {[]string{enqueueLine("task-1", "", "event", t2)}, "", "", -1},
		"interim":     {[]string{enqueueLine("task-1", "completed", "the result below may be interim", t2)}, "", "", -1},
		"other id":    {[]string{enqueueLine("task-10", "completed", "done", t2)}, "", "", -1},
		"taskstop":    {[]string{taskStopLine("task-1", t2)}, TaskOutcomeKilled, t2, -1},
		"stop other":  {[]string{taskStopLine("task-10", t2)}, "", "", -1},
		"notice only": {[]string{notificationLine("task-1", "u1", "completed", t2)}, TaskOutcomeSuccess, t2, -1},
		"first wins": {[]string{
			enqueueLine("task-1", "completed", "done (exit code 0)", t2),
			enqueueLine("task-1", "failed", "failed with exit code 1", t3),
		}, TaskOutcomeSuccess, t2, 0},
		"notification beats later taskstop": {[]string{
			enqueueLine("task-1", "completed", "done", t2),
			taskStopLine("task-1", t3),
		}, TaskOutcomeSuccess, t2, -1},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := readRecords(t, append(launch("u1", "task-1", "x"), tc.tail...)...)[0]
			if tc.at == "" {
				if r.End != nil {
					t.Fatalf("end = %+v, want none", r.End)
				}
				return
			}
			if r.End == nil || r.End.Outcome != tc.outcome || !r.End.At.Equal(mustParseTime(tc.at)) {
				t.Fatalf("end = %+v, want %s at %s", r.End, tc.outcome, tc.at)
			}
			if tc.code >= 0 && (r.ExitCode == nil || *r.ExitCode != tc.code) {
				t.Errorf("exit code = %v, want %d", r.ExitCode, tc.code)
			}
			if tc.code < 0 && r.ExitCode != nil {
				t.Errorf("exit code = %d, want none", *r.ExitCode)
			}
		})
	}
}

func TestReadTaskRecords_ForegroundCalls(t *testing.T) {
	t.Parallel()
	recs := readRecords(t,
		bashUseLine("ok", t0, `{"command":"echo hi","description":"say hi"}`),
		bashUseLine("bad", t0, `{"command":"false"}`),
		bashUseLine("open", t0, `{"command":"sleep 5"}`),
		// Results arrive in the other order.
		plainResultLine("bad", t1, true, "Exit code 2\nboom", `"Error: Exit code 2"`),
		plainResultLine("ok", t2, false, "hi", `{"stdout":"hi"}`),
		bashUseLine("late", t3, `{"command":"later"}`),
	)
	ok, bad, open := recByLabel(t, recs, "say hi"), recByLabel(t, recs, "false"), recByLabel(t, recs, "sleep 5")
	if ok.Kind != TaskKindCall || ok.End == nil || ok.End.Outcome != TaskOutcomeSuccess || !ok.End.At.Equal(mustParseTime(t2)) {
		t.Errorf("ok = %+v", ok)
	}
	if bad.End == nil || bad.End.Outcome != TaskOutcomeFailure || !bad.End.At.Equal(mustParseTime(t1)) || bad.ExitCode == nil || *bad.ExitCode != 2 {
		t.Errorf("bad = %+v exit %v", bad.End, bad.ExitCode)
	}
	if open.End != nil || !open.TurnEnd.IsZero() {
		t.Errorf("open = %+v, want running", open)
	}
}

func TestReadTaskRecords_TurnEndOnlyReachesCallsWithoutAResult(t *testing.T) {
	t.Parallel()
	recs := readRecords(t,
		bashUseLine("a", t0, `{"command":"a"}`),
		bashUseLine("sh", t0, `{"command":"sh","run_in_background":true}`),
		turnEndLine(t2),
		bashUseLine("b", t3, `{"command":"b"}`),
	)
	if a := recByLabel(t, recs, "a"); !a.TurnEnd.Equal(mustParseTime(t2)) {
		t.Errorf("a turn end = %v", a.TurnEnd)
	}
	if sh := recByLabel(t, recs, "sh"); !sh.TurnEnd.IsZero() {
		t.Errorf("shell turn end = %v, want none", sh.TurnEnd)
	}
	if b := recByLabel(t, recs, "b"); !b.TurnEnd.IsZero() {
		t.Errorf("later call turn end = %v, want none", b.TurnEnd)
	}
}

func TestReadTaskRecords_NotificationUserLineIsNotATurnEnd(t *testing.T) {
	t.Parallel()
	recs := readRecords(t,
		bashUseLine("a", t0, `{"command":"a"}`),
		notificationLine("other", "u", "completed", t2),
	)
	if !recs[0].TurnEnd.IsZero() {
		t.Errorf("turn end = %v", recs[0].TurnEnd)
	}
}

func TestReadTaskRecords_SidechainAndMissingFile(t *testing.T) {
	t.Parallel()
	side := strings.Replace(bashUseLine("s", t0, `{"command":"x"}`), `{"type":"assistant"`, `{"isSidechain":true,"type":"assistant"`, 1)
	if recs := readRecords(t, side); len(recs) != 0 {
		t.Errorf("recs = %+v", recs)
	}
	recs, open, err := ReadTaskRecords(filepath.Join(t.TempDir(), "none.jsonl"))
	if recs != nil || open != nil || err != nil {
		t.Errorf("missing file = %v, %v, %v", recs, open, err)
	}
}

func TestReadTaskRecords_OpenCallsMatchReadInFlightTools(t *testing.T) {
	t.Parallel()
	path := writeTranscript(t,
		`{"type":"user","message":{"content":"earlier prompt"}}`,
		bashUseLine("old", t0, `{"command":"echo old"}`),
		`{"type":"user","message":{"content":"go"}}`,
		bashUseLine("a", t0, `{"command":"sleep 1"}`),
		bashUseLine("b", t0, `{"command":"make","run_in_background":true}`),
		`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"g","name":"Grep","input":{"pattern":"foo"}}]}}`,
		`{"isSidechain":true,"type":"assistant","message":{"content":[{"type":"tool_use","id":"s","name":"Bash","input":{"command":"x"}}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"a","content":"ok"}]}}`,
	)
	want, err := ReadInFlightTools(path)
	if err != nil {
		t.Fatal(err)
	}
	_, got, err := ReadTaskRecords(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(want) != 2 || !slices.Equal(got, want) {
		t.Errorf("open calls = %+v, want %+v (two calls)", got, want)
	}
}
