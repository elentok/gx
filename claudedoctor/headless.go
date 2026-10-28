package claudedoctor

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner"
)

// The headless checks. Fixture checks read the canned sessions in testdata/;
// live checks need a real claude process or the filesystem it writes to.

var headlessChecks = []Check{
	{Runner: "headless", Name: "init-capabilities", Mode: Fixture, Session: "turn", Run: checkInitCapabilities},
	{Runner: "headless", Name: "session-id-honored", Mode: Fixture, Session: "turn", DependsOn: []string{"init-capabilities"}, Run: checkSessionID},
	{Runner: "headless", Name: "transcript-written", Mode: Live, Session: "turn", DependsOn: []string{"session-id-honored"}, Run: checkTranscriptWritten},
	{Runner: "headless", Name: "rate-limit-event", Mode: Fixture, Session: "turn", DependsOn: []string{"init-capabilities"}, Run: checkRateLimitEvent},
	{Runner: "headless", Name: "command-lifecycle", Mode: Fixture, Session: "turn", DependsOn: []string{"init-capabilities"}, Run: checkCommandLifecycle},
	{Runner: "headless", Name: "permission-request-answer", Mode: Fixture, Session: "permission", DependsOn: []string{"init-capabilities"}, Run: checkPermission},
	{Runner: "headless", Name: "permission-request-shape", Mode: Fixture, Session: "permission", DependsOn: []string{"init-capabilities"}, Run: checkPermissionRequestShape},
	{Runner: "headless", Name: "permission-request-blocks", Mode: Fixture, Session: "permission", DependsOn: []string{"init-capabilities"}, Run: checkPermissionBlocks},
	{Runner: "headless", Name: "requires-action-blocks", Mode: Fixture, Session: "requires-action", DependsOn: []string{"init-capabilities"}, Run: checkRequiresActionBlocks},
	{Runner: "headless", Name: "interrupt", Mode: Fixture, Session: "interrupt", DependsOn: []string{"init-capabilities"}, Run: checkInterrupt},
	{Runner: "headless", Name: "compact-same-session", Mode: Fixture, Session: "compact", DependsOn: []string{"init-capabilities"}, Run: checkCompact},
	{Runner: "headless", Name: "survives-launcher-exit", Mode: Live, DependsOn: []string{"claude-version"}, Run: checkSurvivesLauncherExit},
	{Runner: "headless", Name: "survives-launchctl-kickstart", Mode: Live, DependsOn: []string{"claude-version"}, Run: checkSurvivesKickstart},
	{Runner: "headless", Name: "print-mode-hooks-blocked", Mode: Live, DependsOn: []string{"claude-version"}, Run: checkPrintModeHooksBlocked},
}

// doctorEvent holds the stream-json fields the checks read; anything else in
// an event is ignored, as in the runner itself.
type doctorEvent struct {
	Type         string   `json:"type"`
	Subtype      string   `json:"subtype"`
	SessionID    string   `json:"session_id"`
	State        string   `json:"state"`
	CommandUUID  string   `json:"command_uuid"`
	Capabilities []string `json:"capabilities"`
	RequestID    string   `json:"request_id"`
	Request      struct {
		Subtype  string `json:"subtype"`
		ToolName string `json:"tool_name"`
	} `json:"request"`
	RateLimitInfo struct {
		Status string `json:"status"`
	} `json:"rate_limit_info"`
}

func parseEvents(raw []json.RawMessage) []doctorEvent {
	var out []doctorEvent
	for _, r := range raw {
		var e doctorEvent
		if json.Unmarshal(r, &e) == nil {
			out = append(out, e)
		}
	}
	return out
}

func (e doctorEvent) is(typ, subtype string) bool {
	return e.Type == typ && e.Subtype == subtype
}

func findEvent(events []doctorEvent, match func(doctorEvent) bool) (doctorEvent, bool) {
	i := slices.IndexFunc(events, match)
	if i < 0 {
		return doctorEvent{}, false
	}
	return events[i], true
}

func countEvents(events []doctorEvent, match func(doctorEvent) bool) int {
	n := 0
	for _, e := range events {
		if match(e) {
			n++
		}
	}
	return n
}

func initEvent(events []doctorEvent) (doctorEvent, bool) {
	return findEvent(events, func(e doctorEvent) bool { return e.is("system", "init") })
}

func checkInitCapabilities(in Input) (string, bool) {
	init, ok := initEvent(parseEvents(in.Events))
	if !ok {
		return "no system/init event", false
	}
	for _, want := range []string{"msg_lifecycle_v1"} {
		if !slices.Contains(init.Capabilities, want) {
			return fmt.Sprintf("init lacks %s (has %s)", want, strings.Join(init.Capabilities, ",")), false
		}
	}
	return strings.Join(init.Capabilities, ","), true
}

var uuidRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// checkSessionID: gx launches claude with --session-id and later names the
// transcript by it, so init and every later event must carry that one id.
func checkSessionID(in Input) (string, bool) {
	events := parseEvents(in.Events)
	init, ok := initEvent(events)
	if !ok || !uuidRE.MatchString(init.SessionID) {
		return fmt.Sprintf("init session_id %q is not a uuid", init.SessionID), false
	}
	for _, e := range events {
		if e.SessionID != "" && e.SessionID != init.SessionID {
			return fmt.Sprintf("%s event has session_id %s, init has %s", e.Type, e.SessionID, init.SessionID), false
		}
	}
	return init.SessionID, true
}

// checkTranscriptWritten looks for <session>.jsonl under claude's projects
// directory: gx finds a dead agent's work there.
func checkTranscriptWritten(in Input) (string, bool) {
	init, _ := initEvent(parseEvents(in.Events))
	projects, err := harness.ClaudeProjectsDirIn(explicitEnv(in.Env))
	if err != nil {
		return err.Error(), false
	}
	matches, _ := filepath.Glob(filepath.Join(projects, "*", init.SessionID+".jsonl"))
	if len(matches) == 0 {
		return "no transcript for " + init.SessionID, false
	}
	return matches[0], true
}

func checkRateLimitEvent(in Input) (string, bool) {
	e, ok := findEvent(parseEvents(in.Events), func(e doctorEvent) bool { return e.Type == "rate_limit_event" })
	if !ok || e.RateLimitInfo.Status == "" {
		return "no rate_limit_event with a status", false
	}
	return "status " + e.RateLimitInfo.Status, true
}

// checkCommandLifecycle: a turn must show queued, started and completed for
// one command, which is how Prompt knows a turn began.
func checkCommandLifecycle(in Input) (string, bool) {
	states := map[string][]string{}
	for _, e := range parseEvents(in.Events) {
		if e.Type == "command_lifecycle" {
			states[e.CommandUUID] = append(states[e.CommandUUID], e.State)
		}
	}
	for uuid, got := range states {
		if slices.Contains(got, "started") && slices.Contains(got, "completed") {
			return strings.Join(got, ">"), true
		}
		return fmt.Sprintf("command %s only reached %s", uuid, strings.Join(got, ">")), false
	}
	return "no command_lifecycle events", false
}

// checkPermission covers the stdio permission round trip: claude asks via a
// can_use_tool control_request, flags requires_action, and carries on once
// gx writes the answer to its stdin (the FIFO).
func checkPermission(in Input) (string, bool) {
	events := parseEvents(in.Events)
	reqAt := slices.IndexFunc(events, func(e doctorEvent) bool {
		return e.Type == "control_request" && e.Request.Subtype == "can_use_tool"
	})
	if reqAt < 0 {
		return "no can_use_tool control_request", false
	}
	required := countEvents(events, func(e doctorEvent) bool {
		return e.is("system", "session_state_changed") && e.State == "requires_action"
	})
	if required == 0 {
		return "no requires_action state change", false
	}
	resumed := slices.ContainsFunc(events[reqAt:], func(e doctorEvent) bool {
		return e.is("system", "session_state_changed") && e.State == "running"
	})
	if !resumed {
		return "session did not resume after the answer", false
	}
	return fmt.Sprintf("%d requires_action event(s); resumed after answer", required), true
}

// checkPermissionRequestShape: the answer is routed by request_id and the web
// UI shows tool_name, so a can_use_tool request lacking either is unanswerable.
func checkPermissionRequestShape(in Input) (string, bool) {
	e, ok := findEvent(parseEvents(in.Events), func(e doctorEvent) bool {
		return e.Type == "control_request" && e.Request.Subtype == "can_use_tool"
	})
	if !ok {
		return "no can_use_tool control_request", false
	}
	if e.RequestID == "" {
		return "can_use_tool request has no request_id", false
	}
	if e.Request.ToolName == "" {
		return "can_use_tool request has no tool_name", false
	}
	return fmt.Sprintf("request_id %s for %s", e.RequestID, e.Request.ToolName), true
}

// checkPermissionBlocks: a can_use_tool request must pause the runner as
// blocked, or a headless agent waits for an answer nobody is asked for.
func checkPermissionBlocks(in Input) (string, bool) {
	for _, st := range nativerunner.StatusTrail(in.Events) {
		if st.State == agentrunner.StateBlocked && st.Pending != nil {
			return "blocked on " + st.Pending.Tool, true
		}
	}
	return "tracker never reached blocked with a pending request", false
}

// checkRequiresActionBlocks: a requires_action with no can_use_tool must still
// read as blocked ("waiting on a prompt"), and an interrupt must clear it.
func checkRequiresActionBlocks(in Input) (string, bool) {
	blockedAt := -1
	trail := nativerunner.StatusTrail(in.Events)
	for i, st := range trail {
		if st.State != agentrunner.StateBlocked {
			continue
		}
		if st.BlockedReason != "waiting on a prompt" {
			return fmt.Sprintf("blocked with reason %q", st.BlockedReason), false
		}
		blockedAt = i
		break
	}
	if blockedAt < 0 {
		return "bare requires_action never reached blocked", false
	}
	if !slices.ContainsFunc(trail[blockedAt:], func(st agentrunner.Status) bool { return st.State != agentrunner.StateBlocked }) {
		return "interrupt did not leave blocked", false
	}
	return "blocked (waiting on a prompt); interrupt cleared it", true
}

func checkInterrupt(in Input) (string, bool) {
	events := parseEvents(in.Events)
	if _, ok := findEvent(events, func(e doctorEvent) bool { return e.Type == "control_response" }); !ok {
		return "no control_response to the interrupt", false
	}
	if e, ok := findEvent(events, func(e doctorEvent) bool { return e.Type == "result" }); !ok || e.Subtype == "success" {
		return "turn did not end with an error result", false
	}
	return "interrupt acknowledged; turn ended", true
}

func checkCompact(in Input) (string, bool) {
	events := parseEvents(in.Events)
	init, _ := initEvent(events)
	b, ok := findEvent(events, func(e doctorEvent) bool { return e.is("system", "compact_boundary") })
	if !ok {
		return "no compact_boundary event", false
	}
	if b.SessionID != init.SessionID {
		return fmt.Sprintf("compact moved to session %s from %s", b.SessionID, init.SessionID), false
	}
	return "compact_boundary in session " + init.SessionID, true
}

// startDetached starts a headless claude in its own session with stdin held
// open by a fifo, the way a launcher would, and returns its pid. dir holds
// the fifo and is the caller's to remove.
func startDetached(env []string, dir string) (int, error) {
	fifo := filepath.Join(dir, "in")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		return 0, err
	}
	pidFile := filepath.Join(dir, "pid")
	// The inner shell holds the fifo read-write so claude never sees EOF.
	script := fmt.Sprintf(`exec 3<>%q; claude -p --input-format stream-json --output-format stream-json --verbose <&3 >%q 2>&1 & echo $! > %q`,
		fifo, filepath.Join(dir, "out"), pidFile)
	cmd := exec.Command("sh", "-c", script)
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Run(); err != nil {
		return 0, err
	}
	return readPid(pidFile)
}

func readPid(path string) (int, error) {
	var pid int
	for range 20 {
		if data, err := os.ReadFile(path); err == nil {
			if _, err := fmt.Sscan(string(data), &pid); err == nil && pid > 0 {
				return pid, nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return 0, fmt.Errorf("no pid written to %s", path)
}

func alive(pid int) bool { return syscall.Kill(pid, 0) == nil }

func checkSurvivesLauncherExit(in Input) (string, bool) {
	dir, err := os.MkdirTemp("", "gx-doctor-")
	if err != nil {
		return err.Error(), false
	}
	defer os.RemoveAll(dir)
	pid, err := startDetached(in.Env, dir)
	if err != nil {
		return err.Error(), false
	}
	defer syscall.Kill(pid, syscall.SIGKILL)
	time.Sleep(2 * time.Second)
	if !alive(pid) {
		return fmt.Sprintf("claude %d died when its launcher exited", pid), false
	}
	return fmt.Sprintf("claude %d outlived its launcher", pid), true
}

// setsid runs the command after it in a new session; launchd jobs have no
// setsid(1).
const setsid = `/usr/bin/perl -MPOSIX -e 'POSIX::setsid(); exec @ARGV'`

// kickstartJob submits a throwaway launchd job whose shell backgrounds the
// start command, then `launchctl kickstart -k`s it: that kills the job's
// process group, which a setsid'd process must not be in. It returns the
// started pid once kickstart has had time to land; the caller kills it.
func kickstartJob(env []string, dir, start string) (int, error) {
	if _, err := exec.LookPath("launchctl"); err != nil {
		return 0, errors.New("launchctl not found")
	}
	label := fmt.Sprintf("gx.doctor.%d", os.Getpid())
	target := fmt.Sprintf("gui/%d/%s", os.Getuid(), label)
	pidFile := filepath.Join(dir, "pid")
	script := fmt.Sprintf(`%s & echo $! > %q; sleep 600`, start, pidFile)
	submit := exec.Command("launchctl", "submit", "-l", label, "--", "sh", "-c", script)
	submit.Env = env
	if out, err := submit.CombinedOutput(); err != nil {
		return 0, fmt.Errorf("launchctl submit: %v: %s", err, out)
	}
	remove := exec.Command("launchctl", "remove", label)
	remove.Env = env
	defer remove.Run()

	pid, err := readPid(pidFile)
	if err != nil {
		return 0, err
	}
	kickstart := exec.Command("launchctl", "kickstart", "-k", target)
	kickstart.Env = env
	if out, err := kickstart.CombinedOutput(); err != nil {
		return pid, fmt.Errorf("launchctl kickstart: %v: %s", err, out)
	}
	time.Sleep(2 * time.Second)
	return pid, nil
}

// checkSurvivesKickstart starts claude detached from a launchd job that is
// then kickstarted.
func checkSurvivesKickstart(in Input) (string, bool) {
	dir, err := os.MkdirTemp("", "gx-doctor-")
	if err != nil {
		return err.Error(), false
	}
	defer os.RemoveAll(dir)

	fifo := filepath.Join(dir, "in")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		return err.Error(), false
	}
	start := fmt.Sprintf(`exec 3<>%q; %s claude -p --input-format stream-json --output-format stream-json --verbose <&3 >%q 2>&1`,
		fifo, setsid, filepath.Join(dir, "out"))
	pid, err := kickstartJob(in.Env, dir, start)
	if pid > 0 {
		defer syscall.Kill(pid, syscall.SIGKILL)
	}
	if err != nil {
		return err.Error(), false
	}
	if !alive(pid) {
		return fmt.Sprintf("claude %d died on kickstart -k", pid), false
	}
	return fmt.Sprintf("claude %d survived kickstart -k", pid), true
}

// checkPrintModeHooksBlocked: with hooks disabled, `claude -p` must still
// start and answer, using the user's other settings.
func checkPrintModeHooksBlocked(in Input) (string, bool) {
	cmd := exec.Command("claude", "-p", "reply with the word ok",
		"--output-format", "json", "--settings", `{"disableAllHooks":true}`)
	cmd.Env = in.Env
	out, err := cmd.Output()
	if err != nil {
		return fmt.Sprintf("claude -p failed: %v", err), false
	}
	var res struct {
		IsError bool   `json:"is_error"`
		Result  string `json:"result"`
	}
	if json.Unmarshal(out, &res) != nil || res.IsError || res.Result == "" {
		return "claude -p returned no result", false
	}
	return "claude -p answered with hooks disabled", true
}
