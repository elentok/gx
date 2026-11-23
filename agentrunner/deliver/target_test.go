package deliver_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/agentrunner/deliver"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/testutil/agentfake"
)

type held struct {
	*agentfake.HeldProgram
	t    *testing.T
	conv string
	c    *holder.HolderClient
}

func holdInput(t *testing.T) held {
	t.Helper()
	conv := agentfake.NewConversationID(t)
	cmd := agentfake.Harness{}.Launch(harness.LaunchOptions{SessionID: conv})
	p := agentfake.HoldProgram(t, cmd, conv, agentfake.ProgramInputEnv+"=1")
	p.WaitScreen(agentfake.ProgramPlaceholder)
	c, err := holder.DialHolder(p.Dir)
	if err != nil {
		t.Fatal(err)
	}
	return held{HeldProgram: p, t: t, conv: conv, c: c}
}

func (h held) submitted() string {
	data, _ := os.ReadFile(agentfake.ProgramSubmittedPath(h.conv))
	return string(data)
}

func (h held) waitSubmitted(want string) {
	h.t.Helper()
	h.WaitFor("the program to submit "+want, func() bool { return h.submitted() == want })
}

func (h held) screen() string {
	rows, _, _ := h.c.Screen(0)
	return strings.Join(rows, "\n")
}

// target is a Target over the held program. The turn starts once the program
// submitted something.
func (h held) target(t *testing.T, typing harness.Typing) *deliver.Target {
	t.Helper()
	c := h.c
	return &deliver.Target{
		C: c, Typing: typing,
		IO: harness.TypingIO{
			Rows:     func() ([]string, error) { rows, _, err := c.Screen(0); return rows, err },
			SendKeys: c.SendKeys,
			Session:  h.conv,
		},
		Start: func(timeout time.Duration) (bool, error) {
			tick, end := time.NewTicker(10*time.Millisecond), time.After(timeout)
			defer tick.Stop()
			for {
				if h.submitted() != "" {
					return true, nil
				}
				select {
				case <-tick.C:
				case <-end:
					return false, nil
				}
			}
		},
	}
}

var caps = deliver.NudgeCaps{Grace: 5 * time.Second, Nudges: 3, Retypes: 2, Settle: 300 * time.Millisecond}

// A recordingTyping notes what the program's screen showed when the ready
// step ran, so a test can tell it ran before the text was typed.
type recordingTyping struct {
	harness.Typing
	h        held
	ready    bool
	screenAt string
}

func (r *recordingTyping) ReadyToType(harness.TypingIO) bool {
	r.screenAt = r.h.screen()
	return r.ready
}

func TestTarget_TypesAndSubmitsOnce(t *testing.T) {
	h := holdInput(t)
	if err := deliver.Deliver(h.target(t, harness.ClaudeTyping()), "hello there", deliver.Policy{Caps: caps}); err != nil {
		t.Fatal(err)
	}
	h.waitSubmitted("\"hello there\"\n")
}

func TestTarget_DeliverStripsHiddenCharacters(t *testing.T) {
	h := holdInput(t)
	if err := deliver.Deliver(h.target(t, harness.ClaudeTyping()), "he​llo", deliver.Policy{Caps: caps}); err != nil {
		t.Fatal(err)
	}
	h.waitSubmitted("\"hello\"\n")
}

func TestTarget_PastesImagesBeforeTheTextAfterASpace(t *testing.T) {
	h := holdInput(t)
	img := filepath.Join(t.TempDir(), "a.png")
	if err := os.WriteFile(img, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	tg := h.target(t, harness.ClaudeTyping())
	var sent []int
	tg.Images, tg.ImageSent = []string{img}, func(i int) { sent = append(sent, i) }
	if err := deliver.Deliver(tg, "look", deliver.Policy{Caps: caps}); err != nil {
		t.Fatal(err)
	}
	h.WaitFor("the image and text to be submitted", func() bool {
		return strings.Contains(h.submitted(), img) && strings.HasSuffix(strings.TrimSpace(h.submitted()), " look\"")
	})
	if len(sent) != 1 || sent[0] != 0 {
		t.Errorf("ImageSent calls = %v, want [0]", sent)
	}
}

func TestTarget_CodexTabsWhileATurnRunsAndEntersOtherwise(t *testing.T) {
	codex, err := harness.LookupTyping(harness.Codex)
	if err != nil {
		t.Fatal(err)
	}
	idle := holdInput(t)
	if err := idle.target(t, codex).Submit("idle text"); err != nil {
		t.Fatal(err)
	}
	idle.waitSubmitted("\"idle text\"\n")

	busy := holdInput(t)
	busy.Control(agentfake.Control{Action: agentfake.ActionReport, State: "working"})
	busy.WaitStatus(harness.Status{State: harness.Working})
	if err := busy.target(t, codex).Submit("queued text"); err != nil {
		t.Fatal(err)
	}
	// The fake logs a Tab as its own entry and leaves the text in the input:
	// no Enter went out.
	busy.waitSubmitted("\"<tab>\"\n")
	busy.WaitScreen("queued text")
}

func TestTarget_ReadyToTypeRunsBeforeTyping(t *testing.T) {
	h := holdInput(t)
	rec := &recordingTyping{Typing: harness.ClaudeTyping(), h: h, ready: true}
	if err := deliver.Deliver(h.target(t, rec), "after ready", deliver.Policy{Caps: caps}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rec.screenAt, "after ready") {
		t.Errorf("screen at ready step = %q, want the text not typed yet", rec.screenAt)
	}
	h.waitSubmitted("\"after ready\"\n")
}

func TestTarget_NotReadyTypesNothing(t *testing.T) {
	h := holdInput(t)
	rec := &recordingTyping{Typing: harness.ClaudeTyping(), h: h}
	err := deliver.Deliver(h.target(t, rec), "never typed", deliver.Policy{Caps: caps})
	if !errors.Is(err, claudeui.ErrNotReadyToType) {
		t.Fatalf("Deliver() error = %v, want ErrNotReadyToType", err)
	}
	if got := h.submitted(); got != "" {
		t.Errorf("submitted = %q, want nothing", got)
	}
}

func TestTarget_CancelStopsTheKeys(t *testing.T) {
	h := holdInput(t)
	tg := h.target(t, harness.ClaudeTyping())
	cancel := make(chan struct{})
	close(cancel)
	tg.Cancel = cancel
	err := deliver.Deliver(tg, "cancelled", deliver.Policy{Caps: caps})
	if !errors.Is(err, deliver.ErrCancelled) {
		t.Fatalf("Deliver() error = %v, want ErrCancelled", err)
	}
	if got := h.submitted(); got != "" {
		t.Errorf("submitted = %q, want nothing", got)
	}
	if screen := h.screen(); strings.Contains(screen, "cancelled") {
		t.Errorf("screen shows the cancelled text: %q", screen)
	}
}

func TestDeliver_NudgesASwallowedFirstEnterOnce(t *testing.T) {
	h := holdInput(t)
	h.Control(agentfake.Control{Action: agentfake.ActionSwallowEnter})
	var nudges []claudeui.NudgeDecision
	policy := deliver.Policy{Caps: caps, Report: func(d claudeui.NudgeDecision) { nudges = append(nudges, d) }}
	if err := deliver.Deliver(h.target(t, harness.ClaudeTyping()), "swallowed once", policy); err != nil {
		t.Fatal(err)
	}
	h.waitSubmitted("\"swallowed once\"\n")
	if len(nudges) != 1 || nudges[0] != claudeui.NudgeEnter {
		t.Errorf("nudges = %v, want one enter", nudges)
	}
}
