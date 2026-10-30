package nativerunner_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/screen"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
)

// attachTo opens an attach stream on s's holder, the way a person's live panel
// does.
func attachTo(t *testing.T, p *nativerunner.Pty, s agentrunner.Session) *nativerunner.HolderAttach {
	t.Helper()
	c, err := nativerunner.DialHolder(filepath.Join(p.Root, s.Label))
	if err != nil {
		t.Fatal(err)
	}
	a, err := c.Attach()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	return a
}

func startPersonInputPty(t *testing.T, p *nativerunner.Pty) agentrunner.Session {
	t.Helper()
	s, err := p.Start(agentrunner.StartOptions{Label: "e-01", Epic: "e", Cwd: t.TempDir(), Kind: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Stop(s) })
	return s
}

func fakeKeysTyped(t *testing.T, p *nativerunner.Pty, s agentrunner.Session) string {
	t.Helper()
	in, err := os.ReadFile(agentfake.TUIInputPath(ptyHome(p), s.SessionID))
	if err != nil {
		t.Fatal(err)
	}
	return string(in)
}

func waitFakeKeys(t *testing.T, p *nativerunner.Pty, s agentrunner.Session, want string) {
	t.Helper()
	if !testutil.Poll(10*time.Second, 10*time.Millisecond, func() bool {
		return strings.Contains(fakeKeysTyped(t, p, s), want)
	}) {
		t.Fatalf("keys typed = %q; want it to hold %q", fakeKeysTyped(t, p, s), want)
	}
}

func TestPty_PersonInputAfterGxTypedEndsNudgesWithoutError(t *testing.T) {
	t.Parallel()
	p := newPty(t)
	p.PromptGrace = 5 * time.Second
	s := startPersonInputPty(t, p)
	ptyControl(t, p, s, agentfake.Control{Action: agentfake.ActionStall})
	a := attachTo(t, p, s)

	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- p.Prompt(s, "do it") }()
	waitFakeKeys(t, p, s, "do it\r")
	if err := a.SendKey(screen.Key{Code: 'x', Text: "x"}); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Prompt = %v; want nil once a person typed", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Prompt kept waiting after a person typed")
	}
	if elapsed := time.Since(start); elapsed >= p.PromptGrace {
		t.Errorf("Prompt took %v; want it to end before the first nudge at %v", elapsed, p.PromptGrace)
	}
	if got := fakeKeysTyped(t, p, s); got != "do it\rx" {
		t.Errorf("keys typed = %q; want the prompt and the person's key, no nudge", got)
	}
}

func TestPty_AttachWithNoKeysSendsNoPersonInput(t *testing.T) {
	t.Parallel()
	p := newPty(t)
	s := startPersonInputPty(t, p)
	events, stop, err := p.PersonInput(s)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	a := attachTo(t, p, s)

	select {
	case <-events:
		t.Fatal("person input event from an attach that sent no keys")
	case <-time.After(300 * time.Millisecond):
	}
	if err := a.SendKey(screen.Key{Code: 'x', Text: "x"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-events:
	case <-time.After(10 * time.Second):
		t.Fatal("no person input event after a key")
	}
}

func TestPty_AttachWithNoKeysLeavesNudgesAlone(t *testing.T) {
	t.Parallel()
	p := newPty(t)
	p.PromptGrace = 100 * time.Millisecond
	s := startPersonInputPty(t, p)
	ptyControl(t, p, s, agentfake.Control{Action: agentfake.ActionStall})
	attachTo(t, p, s)

	if err := p.Prompt(s, "do it"); !errors.Is(err, agentrunner.ErrNotDelivered) {
		t.Fatalf("Prompt = %v; want ErrNotDelivered, the nudges still run", err)
	}
}

func draftPty(t *testing.T) *nativerunner.Pty {
	t.Helper()
	p := newPty(t)
	p.Env = append(p.Env, agentfake.TUIDraftEnv+"=old draft")
	return p
}

func TestPty_PromptWaitsForDraftToGo(t *testing.T) {
	t.Parallel()
	p := draftPty(t)
	p.PromptGrace = 10 * time.Second
	s := startPersonInputPty(t, p)
	a := attachTo(t, p, s)

	done := make(chan error, 1)
	go func() { done <- p.Prompt(s, "do it") }()
	select {
	case err := <-done:
		t.Fatalf("Prompt returned %v while the draft was in the input", err)
	case <-time.After(300 * time.Millisecond):
	}
	if got := fakeKeysTyped(t, p, s); got != "" {
		t.Fatalf("keys typed = %q; want none while a draft is in the input", got)
	}

	if err := a.SendKey(screen.Key{Code: 'u', Mod: screen.ModCtrl}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Prompt = %v; want nil once the draft is gone", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Prompt kept waiting after the draft went")
	}
	if got := fakeKeysTyped(t, p, s); !strings.HasSuffix(got, "do it\r") {
		t.Errorf("keys typed = %q; want the prompt typed after the draft went", got)
	}
}

func TestPty_DraftThatStaysIsNotTypedOver(t *testing.T) {
	t.Parallel()
	p := draftPty(t)
	p.PromptGrace = 200 * time.Millisecond
	s := startPersonInputPty(t, p)

	if err := p.Prompt(s, "do it"); !errors.Is(err, agentrunner.ErrNotDelivered) {
		t.Fatalf("Prompt = %v; want ErrNotDelivered", err)
	}
	if got := fakeKeysTyped(t, p, s); got != "" {
		t.Errorf("keys typed = %q; want none over a draft", got)
	}
}
