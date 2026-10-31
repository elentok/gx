package agentfake

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/harness"
)

// hookArg is the -c argument the Codex harness passes for one hook group.
func hookArg(event, matcher, command string) []string {
	group := "{hooks=[{type=\"command\",command=" + strconv.Quote(command) + "}]}"
	if matcher != "" {
		group = "{matcher=" + strconv.Quote(matcher) + "," + group[1:]
	}
	return []string{"-c", "hooks." + event + "=[" + group + "]"}
}

func joinArgs(parts ...[]string) []string {
	var all []string
	for _, p := range parts {
		all = append(all, p...)
	}
	return all
}

func (p *codexProc) send(c Control) {
	p.t.Helper()
	if err := SendProgramControl(p.id, c); err != nil {
		p.t.Fatal(err)
	}
}

func (p *codexProc) startTurn(prompt string) {
	p.t.Helper()
	p.waitOut("state=idle")
	if _, err := p.stdin.Write([]byte(prompt + "\r")); err != nil {
		p.t.Fatal(err)
	}
	p.waitOut("state=working")
}

func (p *codexProc) hookRuns() []CodexHookRun {
	p.t.Helper()
	runs, err := ReadCodexHookLog(p.home, p.id)
	if err != nil && !os.IsNotExist(err) {
		p.t.Fatal(err)
	}
	return runs
}

func (p *codexProc) waitHookRuns(n int) []CodexHookRun {
	p.t.Helper()
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		if runs := p.hookRuns(); len(runs) >= n {
			return runs
		}
		if time.Now().After(deadline) {
			p.t.Fatalf("timed out waiting for %d hook runs, have %v", n, p.hookRuns())
		}
	}
}

func TestCodexHooks_EachEventRunsItsCommandWithThePayload(t *testing.T) {
	home := t.TempDir()
	args := joinArgs(
		hookArg("SessionStart", "", "true"),
		hookArg("PreToolUse", "Bash", "true"),
		hookArg("PermissionRequest", "Bash", "true"),
		hookArg("Stop", "", "true"),
	)
	p := startCodex(t, home, "hooks1", args...)
	p.startTurn("fix it")
	p.send(Control{Action: ActionToolCall, Tool: "Bash", Command: "rg foo"})
	p.send(Control{Action: ActionPermission, Command: "rg foo"})
	p.send(Control{Action: ActionFinish})
	runs := p.waitHookRuns(4)

	var events []string
	for _, r := range runs {
		events = append(events, r.Event)
		in := r.Payload
		if in.SessionID != "hooks1" || in.Cwd != p.cwd || in.HookEventName != r.Event {
			t.Errorf("%s payload = %+v", r.Event, in)
		}
		if !strings.Contains(in.TranscriptPath, "rollout-") || !strings.HasSuffix(in.TranscriptPath, ".jsonl") {
			t.Errorf("%s transcript_path = %q", r.Event, in.TranscriptPath)
		}
	}
	if got, want := strings.Join(events, ","), "SessionStart,PreToolUse,PermissionRequest,Stop"; got != want {
		t.Fatalf("events = %s, want %s", got, want)
	}
	if tool := runs[1].Payload; tool.ToolName != "Bash" || tool.ToolInput == nil || tool.ToolInput.Command != "rg foo" {
		t.Errorf("PreToolUse payload = %+v", tool)
	}
	raw, err := json.Marshal(runs[1].Payload)
	if err != nil || !strings.Contains(string(raw), `"tool_input":{"command":"rg foo"}`) {
		t.Errorf("wire form = %s (%v)", raw, err)
	}
}

func TestCodexHooks_PreToolUseExit2BlocksTheToolCall(t *testing.T) {
	args := hookArg("PreToolUse", "Bash", "echo nope >&2; exit 2")
	p := startCodex(t, t.TempDir(), "hooks2", args...)
	p.startTurn("fix it")
	p.send(Control{Action: ActionToolCall, Tool: "Bash", Command: "rg foo"})
	p.waitOut("nope")
	p.send(Control{Action: ActionToolCall, Tool: "request_user_input", Command: "x"})
	p.waitOut("ran: x")
	if strings.Contains(p.out.String(), "ran: rg foo") {
		t.Fatalf("a blocked tool call ran:\n%s", p.out.String())
	}
}

var (
	gxOnce sync.Once
	gxPath string
	gxErr  error
)

// builtGx builds the real gx once per test run.
func builtGx(t *testing.T) string {
	t.Helper()
	gxOnce.Do(func() {
		dir, err := os.MkdirTemp("", "agentfake-gx")
		if err != nil {
			gxErr = err
			return
		}
		gxPath = filepath.Join(dir, "gx")
		if out, err := exec.Command("go", "build", "-o", gxPath, "../..").CombinedOutput(); err != nil {
			gxErr = &buildError{err, string(out)}
		}
	})
	if gxErr != nil {
		t.Fatal(gxErr)
	}
	return gxPath
}

type buildError struct {
	err error
	out string
}

func (e *buildError) Error() string { return e.err.Error() + "\n" + e.out }

func TestCodexHooks_RealGuardBashReadsTheFakesPayload(t *testing.T) {
	gx := builtGx(t)
	args := hookArg("PreToolUse", "Bash", gx+" agent guard-bash --harness codex")
	p := startCodex(t, t.TempDir(), "hooks3", args...)
	p.startTurn("fix it")
	p.send(Control{Action: ActionToolCall, Tool: "Bash", Command: "sed -i s/a/b/ f.txt"})
	p.waitOut("sed -i")
	p.send(Control{Action: ActionToolCall, Tool: "Bash", Command: "rg foo"})
	p.waitOut("ran: rg foo")
	if strings.Contains(p.out.String(), "ran: sed") {
		t.Fatalf("guard let sed -i run:\n%s", p.out.String())
	}
}

func TestCodexHooks_PermissionRequestHoldsBlockedUntilTheHookReturns(t *testing.T) {
	release := filepath.Join(t.TempDir(), "release")
	wait := "while [ ! -f " + release + " ]; do sleep 0.02; done"
	p := startCodex(t, t.TempDir(), "hooks4", hookArg("PermissionRequest", "Bash", wait)...)
	p.startTurn("fix it")
	p.send(Control{Action: ActionPermission, Command: "rg foo"})
	p.waitOut("state=blocked:app=codex:kind=permission")
	time.Sleep(100 * time.Millisecond)
	if strings.Contains(p.out.String(), "state=done") {
		t.Fatal("the turn ended before the hook returned")
	}
	if err := os.WriteFile(release, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	p.send(Control{Action: ActionFinish})
	p.waitOut("state=done")
	if strings.Contains(p.out.String(), "denied") {
		t.Fatalf("allow was read as deny:\n%s", p.out.String())
	}
}

func TestCodexHooks_PermissionRequestDenyEndsTheTurnWithTheReason(t *testing.T) {
	for name, command := range map[string]string{
		"exit 2": "echo too risky >&2; exit 2",
		"json":   `echo '{"hookSpecificOutput":{"decision":{"behavior":"deny","message":"too risky"}}}'`,
	} {
		t.Run(name, func(t *testing.T) {
			p := startCodex(t, t.TempDir(), "hooks5", hookArg("PermissionRequest", "Bash", command)...)
			p.startTurn("fix it")
			p.send(Control{Action: ActionPermission, Command: "rg foo"})
			p.waitOut("denied: too risky")
			p.waitOut("state=done")
			runs := p.waitHookRuns(1)
			if runs[0].Event != "PermissionRequest" {
				t.Fatalf("runs = %+v", runs)
			}
		})
	}
}

func TestCodexRollout_ExtrasAreReadByTheProductionReader(t *testing.T) {
	home := t.TempDir()
	p := startCodex(t, home, "extras1")
	p.startTurn("fix it")
	p.send(Control{Action: ActionFinish})
	p.waitOut("state=done")
	p.send(Control{Action: ActionCompact})
	resetsAt := time.Now().Add(time.Hour).Unix()
	p.send(Control{Action: ActionTokenCount, UsedPercent: 100, ResetsAt: resetsAt})
	p.waitFor(func() bool {
		limit, ok, err := codexsession.LastRateLimitIn(home, p.cwd, "extras1")
		return err == nil && ok && limit.ResetAt.Unix() == resetsAt
	}, "the 100% sample")
	tokens, ok, err := codexsession.LastContextTokensIn(home, p.cwd, "extras1")
	if err != nil || !ok || tokens != codexTurnTokens {
		t.Fatalf("tokens after compacted = %d, %v, %v", tokens, ok, err)
	}
	p.send(Control{Action: ActionTaskError, Message: "model gone", Error: "model_not_found"})
	p.waitOut("state=error:app=codex")
	pairs := rolloutPairs(t, rolloutFiles(t, home)[0])
	if !pairs[[2]string{"compacted", ""}] {
		t.Errorf("no compacted line: %v", pairs)
	}
	data, err := os.ReadFile(rolloutFiles(t, home)[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"error":{"codex_error_info":"model_not_found","message":"model gone"}`) {
		t.Errorf("no task_complete error:\n%s", data)
	}
}

func (p *codexProc) waitFor(cond func() bool, what string) {
	p.t.Helper()
	for deadline := time.Now().Add(10 * time.Second); !cond(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			p.t.Fatalf("timed out waiting for %s", what)
		}
	}
}

func TestCodexDialogs_BlockReadyUntilAKeyArrives(t *testing.T) {
	for _, d := range []struct{ env, text string }{
		{CodexTrustEnv, CodexTrustDialog},
		{CodexHooksReviewEnv, CodexHooksReviewDialog},
		{CodexUpdateEnv, CodexUpdateDialog},
		{CodexModelNudgeEnv, CodexModelNudgeDialog},
	} {
		t.Run(d.env, func(t *testing.T) {
			conv := newConversationID(t)
			p := holdProgram(t, harness.Command{Name: os.Args[0], Args: codexLaunchArgs}, conv,
				CodexEnv+"=1", CodexIDEnv+"="+conv, "CODEX_HOME="+t.TempDir(), d.env+"=1")
			p.waitScreen(d.text)
			time.Sleep(100 * time.Millisecond)
			if p.watcher.Status().State == harness.Idle {
				t.Fatal("Ready before the dialog was answered")
			}
			if err := p.client.SendKeys("\r"); err != nil {
				t.Fatal(err)
			}
			p.waitStatus(harness.Status{State: harness.Idle})
		})
	}
}

func TestCodexWorker_CodexUsageConfigWritesARateLimitSampleTheProductionReaderSees(t *testing.T) {
	cfg, _ := workerEnv(t)
	resetsAt := time.Now().Add(72 * time.Hour).Unix()
	cfg.CodexUsage = &UsageWindow{UsedPercentage: 99, ResetsAt: resetsAt}
	repo, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, WorkerConfigFile), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	p := startCodexIn(t, repo, home, "usage1")
	p.startTurn("$gx-implement gx:epic/01")
	p.waitFor(func() bool {
		sample, ok, err := codexsession.LatestUsageIn(home, p.cwd, "usage1")
		return err == nil && ok && len(sample.Windows) == 1 &&
			sample.Windows[0].UsedPercent == 99 && sample.Windows[0].ResetsAt.Unix() == resetsAt
	}, "the 99% sample")
}

func TestCodexWorker_ErrorBehaviourEndsTheFirstPromptWithAModelErrorAndFinishesOnTheNext(t *testing.T) {
	cfg, gxLog := workerEnv(t)
	cfg.Behaviour = map[string]string{"gx:epic/01": BehaviourError}
	repo, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, WorkerConfigFile), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	p := startCodexIn(t, repo, home, "error1")
	p.startTurn("$gx-implement gx:epic/01")
	p.waitOut("state=error:app=codex")
	data, err := os.ReadFile(rolloutFiles(t, home)[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"codex_error_info":"model_not_found"`) || !strings.Contains(string(data), CodexModelError) {
		t.Errorf("no task_complete model error:\n%s", data)
	}
	if strings.Contains(string(data), "rate_limits") {
		t.Errorf("a model error must carry no rate-limit sample:\n%s", data)
	}
	if log, _ := os.ReadFile(gxLog); strings.Contains(string(log), "--iteration-status finished") {
		t.Fatalf("the ticket finished on the error turn: %q", log)
	}
	p.startTurn("$gx-implement gx:epic/01")
	p.waitOut("state=done")
	log, err := os.ReadFile(gxLog)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(log), "tickets set gx:epic/01 --iteration-status finished") {
		t.Errorf("gx log = %q", log)
	}
}

func TestCodexWorker_FinishFromAGxPromptDrivesWorkTicket(t *testing.T) {
	cfg, gxLog := workerEnv(t)
	repo, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, WorkerConfigFile), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	before := commitCount(t)
	p := startCodexIn(t, repo, t.TempDir(), "worker1")
	p.startTurn("$gx-implement gx:epic/01")
	p.waitOut("state=done")
	log, err := os.ReadFile(gxLog)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(log), "tickets set gx:epic/01 --iteration-status finished") {
		t.Errorf("gx log = %q", log)
	}
	if got := commitCount(t); got != before+1 {
		t.Errorf("commits = %d, want %d", got, before+1)
	}
}
