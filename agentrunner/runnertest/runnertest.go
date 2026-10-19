// Package runnertest is the conformance suite every agentrunner.Runner
// passes. Each runner supplies a Harness that drives its fake host; the
// scenarios only go through the Runner interface plus those hooks.
package runnertest

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
)

// Harness drives the agent behind a Runner. The hooks stand in for what a
// real agent does on its own.
type Harness struct {
	Runner agentrunner.Runner
	// FinishTurn makes the agent behind s end its current turn.
	FinishTurn func(t *testing.T, s agentrunner.Session)
	// Block makes the agent behind s wait on a prompt gx did not send.
	Block func(t *testing.T, s agentrunner.Session, reason string)
	// RateLimit makes the agent behind s report a limit that resets at resetAt.
	// Leave it nil for a runner that never reports one.
	RateLimit func(t *testing.T, s agentrunner.Session, resetAt time.Time)
	// Stall makes the agent behind s never start a turn for later prompts.
	Stall func(t *testing.T, s agentrunner.Session)
	// BackgroundTask starts (running) or ends a background task of the agent
	// behind s.
	BackgroundTask func(t *testing.T, s agentrunner.Session, running bool)
	// Restart returns a fresh Runner over the same host, as after a gx
	// restart.
	Restart func(t *testing.T) agentrunner.Runner
	// BlockOnDialog makes the agent behind s wait on a permission prompt, or
	// on another dialog that only deny may answer. Leave it nil for a runner
	// that doesn't tell dialogs apart; the dialog scenarios then skip.
	BlockOnDialog func(t *testing.T, s agentrunner.Session, permission bool)
	// LastPrompt returns the prompt the agent behind s last started a turn
	// on. Leave it nil to skip DenyWithReason.
	LastPrompt func(t *testing.T, s agentrunner.Session) string
	// Received returns the text and keys the agent behind s has received so
	// far, in order. Leave it nil to skip the busy-guard scenarios.
	Received func(t *testing.T, s agentrunner.Session) string
	// FastTurn makes the next turn of the agent behind s start and end with no
	// status edge a poll could see. Leave it nil to skip FastTurnDeliveredOnce.
	FastTurn func(t *testing.T, s agentrunner.Session)
	// OpenDialogOnNextPrompt makes the agent behind s open a dialog on the next
	// "/" command it receives, as /help does. Leave it nil to skip
	// SlashDialogDelivered.
	OpenDialogOnNextPrompt func(t *testing.T, s agentrunner.Session)
	// Skip maps a scenario this runner can't pass yet to the reason why.
	Skip map[string]string
}

// waitTimeout bounds every Wait expected to succeed.
const waitTimeout = 5 * time.Second

var idleOrDone = []agentrunner.State{agentrunner.StateIdle, agentrunner.StateDone}

// Run runs every scenario against a fresh harness from newHarness.
func Run(t *testing.T, newHarness func(t *testing.T) Harness) {
	scenarios := []struct {
		name string
		fn   func(t *testing.T, h Harness)
	}{
		{"StartIsIdle", startIsIdle},
		{"StartLabelTaken", startLabelTaken},
		{"PromptStartsTurn", promptStartsTurn},
		{"PromptNotDelivered", promptNotDelivered},
		{"WaitTimeout", waitTimesOut},
		{"InterruptKeepsSession", interruptKeepsSession},
		{"CompactAfterInterrupt", compactAfterInterrupt},
		{"FinishWaitsForBackgroundTask", finishWaitsForBackgroundTask},
		{"StopIsIdempotent", stopIsIdempotent},
		{"FindAndList", findAndList},
		{"StoppedSessionNotFound", stoppedSessionNotFound},
		{"Blocked", blocked},
		{"BlockedThenAnswer", blockedThenAnswer},
		{"AnswerNotBlocked", answerNotBlocked},
		{"PermissionAllow", permissionAllow},
		{"PermissionDeny", permissionDeny},
		{"DenyWithReason", denyWithReason},
		{"VerbsDuringDenyReason", verbsDuringDenyReason},
		{"OtherDialogDeny", otherDialogDeny},
		{"OtherDialogAllowRefused", otherDialogAllowRefused},
		{"RateLimit", rateLimit},
		{"RestartThenFind", restartThenFind},
		{"TurnSurvivesRestart", turnSurvivesRestart},
		{"BusyPromptWaitsThenSends", busyPromptWaitsThenSends},
		{"BusyPastNNotReady", busyPastNNotReady},
		{"BackgroundTaskNotBusy", backgroundTaskNotBusy},
		{"FastTurnDeliveredOnce", fastTurnDeliveredOnce},
		{"SlashIdleDeliveredOnce", slashIdleDeliveredOnce},
		{"SlashClearReturnsAtOnce", slashClearReturnsAtOnce},
		{"SlashDialogDelivered", slashDialogDelivered},
		{"InterruptBusyCtrlC", interruptBusyCtrlC},
		{"InterruptBlockedEsc", interruptBlockedEsc},
		{"InterruptIdleSendsNothing", interruptIdleSendsNothing},
		{"InterruptNotIdleNotReady", interruptNotIdleNotReady},
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			h := newHarness(t)
			if reason, ok := h.Skip[sc.name]; ok {
				t.Skip(reason)
			}
			sc.fn(t, h)
		})
	}
}

func start(t *testing.T, r agentrunner.Runner, label, epic string) agentrunner.Session {
	t.Helper()
	s, err := r.Start(agentrunner.StartOptions{Label: label, Epic: epic, Cwd: t.TempDir(), Kind: "claude"})
	if err != nil {
		t.Fatalf("Start(%q): %v", label, err)
	}
	return s
}

func status(t *testing.T, r agentrunner.Runner, s agentrunner.Session) agentrunner.Status {
	t.Helper()
	st, err := r.Status(s)
	if err != nil {
		t.Fatalf("Status(%q): %v", s.Label, err)
	}
	return st
}

func prompt(t *testing.T, r agentrunner.Runner, s agentrunner.Session, text string) {
	t.Helper()
	if err := r.Prompt(s, text); err != nil {
		t.Fatalf("Prompt(%q): %v", s.Label, err)
	}
}

func startIsIdle(t *testing.T, h Harness) {
	s := start(t, h.Runner, "e-01", "e")
	if s.Label != "e-01" || s.ID == "" {
		t.Fatalf("session = %+v, want label e-01 and an ID", s)
	}
	if st := status(t, h.Runner, s); !slices.Contains(idleOrDone, st.State) {
		t.Fatalf("status after Start = %+v, want idle", st)
	}
}

// promptAdvancesTurn prompts s and returns its Turn, failing unless the
// prompt advanced it.
func promptAdvancesTurn(t *testing.T, r agentrunner.Runner, s agentrunner.Session, text string) int {
	t.Helper()
	before := status(t, r, s).Turn
	prompt(t, r, s, text)
	after := status(t, r, s).Turn
	if after <= before {
		t.Fatalf("Turn after Prompt(%q) = %d, want > %d", text, after, before)
	}
	return after
}

func startLabelTaken(t *testing.T, h Harness) {
	start(t, h.Runner, "e-01", "e")
	_, err := h.Runner.Start(agentrunner.StartOptions{Label: "e-01", Epic: "e", Cwd: t.TempDir(), Kind: "claude"})
	if !errors.Is(err, agentrunner.ErrLabelTaken) {
		t.Fatalf("second Start err = %v, want ErrLabelTaken", err)
	}
}

func promptStartsTurn(t *testing.T, h Harness) {
	s := start(t, h.Runner, "e-01", "e")
	promptAdvancesTurn(t, h.Runner, s, "do it")
	h.FinishTurn(t, s)
	if _, err := h.Runner.Wait(s, idleOrDone, waitTimeout); err != nil {
		t.Fatalf("Wait idle after FinishTurn: %v", err)
	}
	promptAdvancesTurn(t, h.Runner, s, "again")
}

// turnSurvivesRestart: a caller compares Turn with a value logged before a gx
// restart to tell whether the agent did anything since.
func turnSurvivesRestart(t *testing.T, h Harness) {
	s := start(t, h.Runner, "e-01", "e")
	promptAdvancesTurn(t, h.Runner, s, "do it")
	h.FinishTurn(t, s)
	st, err := h.Runner.Wait(s, idleOrDone, waitTimeout)
	if err != nil {
		t.Fatalf("Wait idle after FinishTurn: %v", err)
	}
	r := h.Restart(t)
	got, ok, err := r.Find("e-01")
	if err != nil || !ok {
		t.Fatalf("Find after restart = %v, %v; want found", ok, err)
	}
	if after := status(t, r, got).Turn; after != st.Turn {
		t.Fatalf("Turn after restart = %d, want %d", after, st.Turn)
	}
	promptAdvancesTurn(t, r, got, "again")
}

func waitTimesOut(t *testing.T, h Harness) {
	s := start(t, h.Runner, "e-01", "e")
	prompt(t, h.Runner, s, "do it")
	st, err := h.Runner.Wait(s, idleOrDone, 50*time.Millisecond)
	if !errors.Is(err, agentrunner.ErrTimeout) {
		t.Fatalf("Wait on a working agent err = %v, want ErrTimeout", err)
	}
	if st.State != agentrunner.StateWorking {
		t.Fatalf("Wait timeout status = %+v, want working", st)
	}
}

func interruptKeepsSession(t *testing.T, h Harness) {
	s := start(t, h.Runner, "e-01", "e")
	prompt(t, h.Runner, s, "do it")
	if err := h.Runner.Interrupt(s); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	if _, err := h.Runner.Wait(s, idleOrDone, waitTimeout); err != nil {
		t.Fatalf("Wait idle after Interrupt: %v", err)
	}
	if _, ok, err := h.Runner.Find("e-01"); err != nil || !ok {
		t.Fatalf("Find after Interrupt = %v, %v; want found", ok, err)
	}
}

func promptNotDelivered(t *testing.T, h Harness) {
	s := start(t, h.Runner, "e-01", "e")
	h.Stall(t, s)
	if err := h.Runner.Prompt(s, "do it"); !errors.Is(err, agentrunner.ErrNotDelivered) {
		t.Fatalf("Prompt to a stalled agent err = %v, want ErrNotDelivered", err)
	}
}

// compactAfterInterrupt is smart-zone recovery: interrupt, then /compact in
// the same session.
func compactAfterInterrupt(t *testing.T, h Harness) {
	s := start(t, h.Runner, "e-01", "e")
	prompt(t, h.Runner, s, "do it")
	if err := h.Runner.Interrupt(s); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	if _, err := h.Runner.Wait(s, idleOrDone, waitTimeout); err != nil {
		t.Fatalf("Wait idle after Interrupt: %v", err)
	}
	promptAdvancesTurn(t, h.Runner, s, "/compact")
	h.FinishTurn(t, s)
	st, err := h.Runner.Wait(s, idleOrDone, waitTimeout)
	if err != nil {
		t.Fatalf("Wait idle after /compact: %v", err)
	}
	if st.SessionID != s.SessionID {
		t.Fatalf("SessionID after /compact = %q, want %q", st.SessionID, s.SessionID)
	}
	if got, ok, err := h.Runner.Find("e-01"); err != nil || !ok || got.ID != s.ID {
		t.Fatalf("Find after /compact = %+v, %v, %v; want %+v", got, ok, err, s)
	}
}

// finishWaitsForBackgroundTask: a turn that ends with a background task
// still running is not finished, since the task wakes the agent again.
func finishWaitsForBackgroundTask(t *testing.T, h Harness) {
	s := start(t, h.Runner, "e-01", "e")
	prompt(t, h.Runner, s, "do it")
	h.BackgroundTask(t, s, true)
	h.FinishTurn(t, s)
	st, err := h.Runner.Wait(s, idleOrDone, 50*time.Millisecond)
	if !errors.Is(err, agentrunner.ErrTimeout) || st.State != agentrunner.StateWorking {
		t.Fatalf("Wait with a background task = %+v, %v; want working, ErrTimeout", st, err)
	}
	h.BackgroundTask(t, s, false)
	if _, err := h.Runner.Wait(s, idleOrDone, waitTimeout); err != nil {
		t.Fatalf("Wait idle after the background task ended: %v", err)
	}
}

func stopIsIdempotent(t *testing.T, h Harness) {
	s := start(t, h.Runner, "e-01", "e")
	for i := range 2 {
		if err := h.Runner.Stop(s); err != nil {
			t.Fatalf("Stop #%d: %v", i+1, err)
		}
	}
	if _, ok, err := h.Runner.Find("e-01"); err != nil || ok {
		t.Fatalf("Find after Stop = %v, %v; want not found", ok, err)
	}
}

func findAndList(t *testing.T, h Harness) {
	a1 := start(t, h.Runner, "a-01", "a")
	a2 := start(t, h.Runner, "a-02", "a")
	start(t, h.Runner, "b-01", "b")

	got, ok, err := h.Runner.Find("a-02")
	if err != nil || !ok || got.ID != a2.ID {
		t.Fatalf("Find(a-02) = %+v, %v, %v; want %+v", got, ok, err, a2)
	}
	if _, ok, err := h.Runner.Find("missing"); err != nil || ok {
		t.Fatalf("Find(missing) = %v, %v; want not found", ok, err)
	}

	list, err := h.Runner.List("a")
	if err != nil {
		t.Fatalf("List(a): %v", err)
	}
	var labels []string
	for _, s := range list {
		labels = append(labels, s.Label)
	}
	slices.Sort(labels)
	if !slices.Equal(labels, []string{a1.Label, a2.Label}) {
		t.Fatalf("List(a) labels = %v, want [a-01 a-02]", labels)
	}
}

func stoppedSessionNotFound(t *testing.T, h Harness) {
	s := start(t, h.Runner, "e-01", "e")
	if err := h.Runner.Stop(s); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := h.Runner.Prompt(s, "hi"); !errors.Is(err, agentrunner.ErrNotFound) {
		t.Fatalf("Prompt after Stop err = %v, want ErrNotFound", err)
	}
	if _, err := h.Runner.Status(s); !errors.Is(err, agentrunner.ErrNotFound) {
		t.Fatalf("Status after Stop err = %v, want ErrNotFound", err)
	}
}

// blocked covers BlockedThenAnswer's blocked half without a prompt, for a
// runner that can't prompt or answer yet.
func blocked(t *testing.T, h Harness) {
	waitBlocked(t, h, start(t, h.Runner, "e-01", "e"))
}

func waitBlocked(t *testing.T, h Harness, s agentrunner.Session) {
	t.Helper()
	h.Block(t, s, "allow Bash?")
	st, err := h.Runner.Wait(s, []agentrunner.State{agentrunner.StateBlocked}, waitTimeout)
	if err != nil {
		t.Fatalf("Wait blocked: %v", err)
	}
	if st.BlockedReason == "" {
		t.Fatalf("blocked status = %+v, want a reason", st)
	}
}

func blockedThenAnswer(t *testing.T, h Harness) {
	s := start(t, h.Runner, "e-01", "e")
	prompt(t, h.Runner, s, "do it")
	waitBlocked(t, h, s)
	if err := h.Runner.Prompt(s, "more"); !errors.Is(err, agentrunner.ErrNotReady) {
		t.Fatalf("Prompt while blocked err = %v, want ErrNotReady", err)
	}
	if err := h.Runner.Answer(s, agentrunner.Answer{Decision: agentrunner.DecisionAllow}); err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if st := status(t, h.Runner, s); st.State == agentrunner.StateBlocked {
		t.Fatalf("status after Answer = %+v, want not blocked", st)
	}
}

func answerNotBlocked(t *testing.T, h Harness) {
	s := start(t, h.Runner, "e-01", "e")
	err := h.Runner.Answer(s, agentrunner.Answer{Decision: agentrunner.DecisionAllow})
	if !errors.Is(err, agentrunner.ErrNotReady) {
		t.Fatalf("Answer on idle agent err = %v, want ErrNotReady", err)
	}
}

// blockOnDialog prompts a fresh session and waits until it is blocked on a
// dialog, skipping when the runner has no BlockOnDialog hook.
func blockOnDialog(t *testing.T, h Harness, permission bool) agentrunner.Session {
	t.Helper()
	if h.BlockOnDialog == nil {
		t.Skip("runner has no BlockOnDialog hook")
	}
	s := start(t, h.Runner, "e-01", "e")
	prompt(t, h.Runner, s, "do it")
	h.BlockOnDialog(t, s, permission)
	if _, err := h.Runner.Wait(s, []agentrunner.State{agentrunner.StateBlocked}, waitTimeout); err != nil {
		t.Fatalf("Wait blocked: %v", err)
	}
	return s
}

func answer(t *testing.T, r agentrunner.Runner, s agentrunner.Session, d agentrunner.Decision) {
	t.Helper()
	if err := r.Answer(s, agentrunner.Answer{Decision: d}); err != nil {
		t.Fatalf("Answer(%s): %v", d, err)
	}
}

// permissionAllow: allow resumes the turn the prompt interrupted.
func permissionAllow(t *testing.T, h Harness) {
	s := blockOnDialog(t, h, true)
	answer(t, h.Runner, s, agentrunner.DecisionAllow)
	if st := status(t, h.Runner, s); st.State != agentrunner.StateWorking {
		t.Fatalf("status after allow = %+v, want working", st)
	}
}

func permissionDeny(t *testing.T, h Harness) {
	s := blockOnDialog(t, h, true)
	answer(t, h.Runner, s, agentrunner.DecisionDeny)
	if _, err := h.Runner.Wait(s, idleOrDone, waitTimeout); err != nil {
		t.Fatalf("Wait idle after deny: %v", err)
	}
}

// denyWithReason: the session stays blocked until the reason is typed, so a
// waiter never takes the gap for the turn's end, and the reason is the next
// prompt.
func denyWithReason(t *testing.T, h Harness) {
	if h.LastPrompt == nil {
		t.Skip("runner has no LastPrompt hook")
	}
	s := blockOnDialog(t, h, true)
	stop := make(chan struct{})
	sawIdle := make(chan agentrunner.Status, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
			}
			if st, err := h.Runner.Status(s); err == nil && slices.Contains(idleOrDone, st.State) {
				sawIdle <- st
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()
	const reason = "use rg instead"
	err := h.Runner.Answer(s, agentrunner.Answer{Decision: agentrunner.DecisionDeny, Text: reason})
	close(stop)
	<-done
	if err != nil {
		t.Fatalf("Answer(deny, %q): %v", reason, err)
	}
	select {
	case st := <-sawIdle:
		t.Fatalf("status during deny with a reason = %+v, want never idle", st)
	default:
	}
	deadline := time.Now().Add(waitTimeout)
	for got := h.LastPrompt(t, s); got != reason; got = h.LastPrompt(t, s) {
		if time.Now().After(deadline) {
			t.Fatalf("last prompt = %q, want %q", got, reason)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func otherDialogDeny(t *testing.T, h Harness) {
	s := blockOnDialog(t, h, false)
	answer(t, h.Runner, s, agentrunner.DecisionDeny)
	if _, err := h.Runner.Wait(s, idleOrDone, waitTimeout); err != nil {
		t.Fatalf("Wait idle after deny: %v", err)
	}
}

// verbsDuringDenyReason: while a deny with a reason is open (a stalled agent
// keeps its dialog through the Esc), Prompt, Interrupt and a second Answer are
// refused and send nothing.
func verbsDuringDenyReason(t *testing.T, h Harness) {
	if h.BlockOnDialog == nil || h.Stall == nil || h.Received == nil {
		t.Skip("runner has no BlockOnDialog, Stall or Received hook")
	}
	s := start(t, h.Runner, "e-01", "e")
	prompt(t, h.Runner, s, "do it")
	h.Stall(t, s)
	h.BlockOnDialog(t, s, true)
	if _, err := h.Runner.Wait(s, []agentrunner.State{agentrunner.StateBlocked}, waitTimeout); err != nil {
		t.Fatalf("Wait blocked: %v", err)
	}
	denied := make(chan error, 1)
	go func() {
		denied <- h.Runner.Answer(s, agentrunner.Answer{Decision: agentrunner.DecisionDeny, Text: "use rg"})
	}()
	deadline := time.Now().Add(waitTimeout)
	for !strings.Contains(h.Received(t, s), "\x1b") {
		if time.Now().After(deadline) {
			t.Fatal("deny never pressed Esc")
		}
		time.Sleep(time.Millisecond)
	}
	before := h.Received(t, s)
	verbs := map[string]error{
		"Prompt":      h.Runner.Prompt(s, "more"),
		"Interrupt":   h.Runner.Interrupt(s),
		"Answer":      h.Runner.Answer(s, agentrunner.Answer{Decision: agentrunner.DecisionAllow}),
		"deny reason": h.Runner.Answer(s, agentrunner.Answer{Decision: agentrunner.DecisionDeny, Text: "other"}),
	}
	for verb, err := range verbs {
		if !errors.Is(err, agentrunner.ErrNotReady) {
			t.Errorf("%s during a deny with a reason err = %v, want ErrNotReady", verb, err)
		}
	}
	if after := h.Received(t, s); after != before {
		t.Errorf("received %q during the deny, want nothing beyond %q", after, before)
	}
	<-denied
}

func otherDialogAllowRefused(t *testing.T, h Harness) {
	s := blockOnDialog(t, h, false)
	err := h.Runner.Answer(s, agentrunner.Answer{Decision: agentrunner.DecisionAllow})
	if !errors.Is(err, agentrunner.ErrInvalidDecision) {
		t.Fatalf("allow on a non-permission dialog err = %v, want ErrInvalidDecision", err)
	}
	if st := status(t, h.Runner, s); st.State != agentrunner.StateBlocked {
		t.Fatalf("status after refused allow = %+v, want still blocked", st)
	}
}

func rateLimit(t *testing.T, h Harness) {
	s := start(t, h.Runner, "e-01", "e")
	if _, limited, err := h.Runner.RateLimit(s); err != nil || limited {
		t.Fatalf("RateLimit on fresh agent = %v, %v; want not limited", limited, err)
	}
	if h.RateLimit == nil {
		return
	}
	resetAt := time.Now().Add(2 * time.Hour).Truncate(time.Minute)
	h.RateLimit(t, s, resetAt)
	got, limited, err := h.Runner.RateLimit(s)
	if err != nil || !limited {
		t.Fatalf("RateLimit = %v, %v; want limited", limited, err)
	}
	// Scraped reset times are only minute-precise.
	if d := got.Sub(resetAt).Abs(); d > time.Minute {
		t.Fatalf("RateLimit resetAt = %v, want ~%v", got, resetAt)
	}
}

// received returns what the agent behind s has received, skipping when the
// runner has no Received hook.
func received(t *testing.T, h Harness, s agentrunner.Session) string {
	t.Helper()
	if h.Received == nil {
		t.Skip("runner has no Received hook")
	}
	return h.Received(t, s)
}

// busyPromptWaitsThenSends: a prompt to a busy agent waits for idle, then
// sends once.
func busyPromptWaitsThenSends(t *testing.T, h Harness) {
	s := start(t, h.Runner, "e-01", "e")
	received(t, h, s)
	prompt(t, h.Runner, s, "first")
	go func() {
		time.Sleep(50 * time.Millisecond)
		h.FinishTurn(t, s)
	}()
	prompt(t, h.Runner, s, "second")
	if n := strings.Count(received(t, h, s), "second"); n != 1 {
		t.Fatalf("agent received %q %d times, want once", "second", n)
	}
}

// busyPastNNotReady: an agent still busy after N refuses the prompt and
// receives nothing.
func busyPastNNotReady(t *testing.T, h Harness) {
	s := start(t, h.Runner, "e-01", "e")
	received(t, h, s)
	prompt(t, h.Runner, s, "first")
	if err := h.Runner.Prompt(s, "second"); !errors.Is(err, agentrunner.ErrNotReady) {
		t.Fatalf("Prompt to a busy agent err = %v, want ErrNotReady", err)
	}
	if got := received(t, h, s); strings.Contains(got, "second") {
		t.Fatalf("agent received %q after a refused prompt, want nothing of it", got)
	}
}

// backgroundTaskNotBusy: an idle agent with a background task takes a prompt
// at once, though Status still says working.
func backgroundTaskNotBusy(t *testing.T, h Harness) {
	s := start(t, h.Runner, "e-01", "e")
	received(t, h, s)
	prompt(t, h.Runner, s, "first")
	h.BackgroundTask(t, s, true)
	h.FinishTurn(t, s)
	promptAdvancesTurn(t, h.Runner, s, "second")
	if n := strings.Count(received(t, h, s), "second"); n != 1 {
		t.Fatalf("agent received %q %d times, want once", "second", n)
	}
}

// fastTurnDeliveredOnce: a turn that ends before any poll sees it still counts
// as delivered, so the text is never retyped.
func fastTurnDeliveredOnce(t *testing.T, h Harness) {
	if h.FastTurn == nil {
		t.Skip("runner has no FastTurn hook")
	}
	s := start(t, h.Runner, "e-01", "e")
	received(t, h, s)
	h.FastTurn(t, s)
	promptAdvancesTurn(t, h.Runner, s, "quick")
	if n := strings.Count(received(t, h, s), "quick"); n != 1 {
		t.Fatalf("agent received %q %d times, want once", "quick", n)
	}
}

// slashIdleDeliveredOnce: a "/" command that starts no turn counts as
// delivered once the agent is still idle after N, and is never retyped.
func slashIdleDeliveredOnce(t *testing.T, h Harness) {
	s := start(t, h.Runner, "e-01", "e")
	received(t, h, s)
	prompt(t, h.Runner, s, "/cost")
	if n := strings.Count(received(t, h, s), "/cost"); n != 1 {
		t.Fatalf("agent received %q %d times, want once", "/cost", n)
	}
}

// slashClearReturnsAtOnce: /clear starts a new conversation, which moves Turn
// without waiting out N.
func slashClearReturnsAtOnce(t *testing.T, h Harness) {
	s := start(t, h.Runner, "e-01", "e")
	received(t, h, s)
	promptAdvancesTurn(t, h.Runner, s, "/clear")
	if n := strings.Count(received(t, h, s), "/clear"); n != 1 {
		t.Fatalf("agent received %q %d times, want once", "/clear", n)
	}
}

// slashDialogDelivered: a dialog command counts as delivered and leaves the
// session blocked.
func slashDialogDelivered(t *testing.T, h Harness) {
	if h.OpenDialogOnNextPrompt == nil {
		t.Skip("runner has no OpenDialogOnNextPrompt hook")
	}
	s := start(t, h.Runner, "e-01", "e")
	h.OpenDialogOnNextPrompt(t, s)
	prompt(t, h.Runner, s, "/help")
	if st := status(t, h.Runner, s); st.State != agentrunner.StateBlocked {
		t.Fatalf("status after a dialog command = %+v, want blocked", st)
	}
}

func restartThenFind(t *testing.T, h Harness) {
	s := start(t, h.Runner, "e-01", "e")
	r := h.Restart(t)
	got, ok, err := r.Find("e-01")
	if err != nil || !ok || got.ID != s.ID {
		t.Fatalf("Find after restart = %+v, %v, %v; want %+v", got, ok, err, s)
	}
	status(t, r, got)
}

// interruptBusyCtrlC: a busy agent gets Ctrl+C, never Esc, and the prompt it
// restores is cleared so the next prompt arrives unprefixed.
func interruptBusyCtrlC(t *testing.T, h Harness) {
	s := start(t, h.Runner, "e-01", "e")
	before := received(t, h, s)
	prompt(t, h.Runner, s, "do it")
	if err := h.Runner.Interrupt(s); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	keys := strings.TrimPrefix(received(t, h, s), before)
	if !strings.Contains(keys, "\x03") || strings.Contains(keys, "\x1b") {
		t.Fatalf("agent received %q on Interrupt, want Ctrl+C and no Esc", keys)
	}
	promptAdvancesTurn(t, h.Runner, s, "next")
	if h.LastPrompt != nil {
		if got := h.LastPrompt(t, s); got != "next" {
			t.Fatalf("prompt after Interrupt = %q, want %q", got, "next")
		}
	}
}

// interruptBlockedEsc: a blocked agent gets a lone Esc.
func interruptBlockedEsc(t *testing.T, h Harness) {
	s := start(t, h.Runner, "e-01", "e")
	received(t, h, s)
	prompt(t, h.Runner, s, "do it")
	h.Block(t, s, "permission prompt")
	if _, err := h.Runner.Wait(s, []agentrunner.State{agentrunner.StateBlocked}, waitTimeout); err != nil {
		t.Fatalf("Wait blocked: %v", err)
	}
	before := received(t, h, s)
	if err := h.Runner.Interrupt(s); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	keys := strings.TrimPrefix(received(t, h, s), before)
	if !strings.Contains(keys, "\x1b") || strings.Contains(keys, "\x03") {
		t.Fatalf("agent received %q on Interrupt, want Esc and no Ctrl+C", keys)
	}
}

// interruptIdleSendsNothing: an idle agent gets no key.
func interruptIdleSendsNothing(t *testing.T, h Harness) {
	s := start(t, h.Runner, "e-01", "e")
	before := received(t, h, s)
	if err := h.Runner.Interrupt(s); err != nil {
		t.Fatalf("Interrupt on an idle agent: %v", err)
	}
	if got := received(t, h, s); got != before {
		t.Fatalf("idle agent received %q on Interrupt, want nothing", strings.TrimPrefix(got, before))
	}
}

// interruptNotIdleNotReady: a stalled agent ignores the key, stays busy past
// N, and is not ready.
func interruptNotIdleNotReady(t *testing.T, h Harness) {
	s := start(t, h.Runner, "e-01", "e")
	received(t, h, s)
	prompt(t, h.Runner, s, "do it")
	h.Stall(t, s)
	if err := h.Runner.Interrupt(s); !errors.Is(err, agentrunner.ErrNotReady) {
		t.Fatalf("Interrupt of an agent that stays busy err = %v, want ErrNotReady", err)
	}
}
