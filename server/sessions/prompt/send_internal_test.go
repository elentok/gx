package prompt

import (
	"errors"
	"fmt"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/server/sessions"
)

func newLockPrompt() *Prompt {
	p := New(Deps{})
	p.GiveSessions(&sessions.Manager{Watches: sessions.NewSessionWatches()})
	return p
}

// typeOnce stands in for one clear, type and Enter: it types its text one
// key at a time into out under the chat's lock.
func typeOnce(p *Prompt, id, text string, out *strings.Builder) error {
	release, err := lockTyping(p, id, nil)
	if err != nil {
		return err
	}
	defer release()
	for _, r := range text {
		out.WriteRune(r)
		runtime.Gosched()
	}
	out.WriteString("|")
	return nil
}

func TestTypingLock_TwoTypersOnOneChatNeverMix(t *testing.T) {
	p := newLockPrompt()
	var out strings.Builder
	var wg sync.WaitGroup
	for _, text := range []string{"aaaaaaaa", "bbbbbbbb", "cccccccc"} {
		wg.Go(func() {
			if err := typeOnce(p, "s1", text, &out); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	for part := range strings.SplitSeq(strings.TrimSuffix(out.String(), "|"), "|") {
		if len(part) != 8 || strings.Trim(part, string(part[0])) != "" {
			t.Fatalf("typed text mixed: %q", out.String())
		}
	}
}

func TestTypingLock_TwoSendsInARowArriveWholeAndInOrder(t *testing.T) {
	p := newLockPrompt()
	var out strings.Builder
	for _, text := range []string{"first", "second"} {
		if err := typeOnce(p, "s1", text, &out); err != nil {
			t.Fatal(err)
		}
	}
	if got := out.String(); got != "first|second|" {
		t.Fatalf("got %q", got)
	}
}

func TestTypingLock_WaiterEndsOnCancel(t *testing.T) {
	p := newLockPrompt()
	release, err := lockTyping(p, "s1", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	cancel := make(chan struct{})
	done := make(chan error, 1)
	go func() { _, err := lockTyping(p, "s1", cancel); done <- err }()
	close(cancel)
	select {
	case err := <-done:
		if !errors.Is(err, errNudgeCancelled) {
			t.Fatalf("err = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waiter did not end on cancel")
	}
}

func TestTypingLock_WaiterEndsOnServerStop(t *testing.T) {
	p := newLockPrompt()
	release, err := lockTyping(p, "s1", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	done := make(chan error, 1)
	go func() { _, err := lockTyping(p, "s1", nil); done <- err }()
	close(p.sessions.Watches.Stop)
	select {
	case err := <-done:
		if !errors.Is(err, ErrServerStopping) {
			t.Fatalf("err = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waiter did not end on server stop")
	}
}

// A first prompt whose nudge ends before any Submit still releases the lock it
// handed to the prompter, so a rename or composer send is not left waiting.
func TestTypingLock_FirstPromptNudgeFailureBeforeSubmitReleases(t *testing.T) {
	p := newLockPrompt()
	release, err := lockTyping(p, "s1", nil)
	if err != nil {
		t.Fatal(err)
	}
	hp := &holderPrompter{held: release}
	hp.releaseHeld()
	hp.releaseHeld()
	done := make(chan error, 1)
	go func() {
		r, err := lockTyping(p, "s1", nil)
		if err == nil {
			r()
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second typer waited on a lock the failed nudge never released")
	}
}

func TestTypingLock_OtherChatIsNotBlocked(t *testing.T) {
	p := newLockPrompt()
	release, err := lockTyping(p, "s1", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	done := make(chan error, 1)
	go func() {
		r, err := lockTyping(p, "s2", nil)
		if err == nil {
			r()
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("other chat blocked")
	}
}

func TestFailureReason(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		err         error
		wantReason  string
		clearNeeded bool
	}{
		{"slash step gave up in vim NORMAL", fmt.Errorf("step 0: %w", claudeui.ErrNotReadyToType), "input is in vim NORMAL mode", false},
		{"nudge gave up in vim NORMAL", fmt.Errorf("%w: %w", claudeui.ErrStuckSubmission, claudeui.ErrNotReadyToType), "input is in vim NORMAL mode", false},
		{"blocked before typing", errSessionBlocked, "session is blocked", false},
		{"input not empty", fmt.Errorf("%w: %w", claudeui.ErrStuckSubmission, claudeui.ErrInputNotEmpty), "input not empty." + clearInputHint, false},
		{"start timeout", claudeui.ErrStartTimeout, "agent never started", true},
		{"stuck", claudeui.ErrStuckSubmission, "prompt never reached the input", true},
		{"launch prompt not delivered", fmt.Errorf("%w: %q: Unknown command:", agentrunner.ErrNotDelivered, "/x"), `prompt not delivered: "/x": Unknown command:`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, clearNeeded := failureReason(tt.err)
			if reason != tt.wantReason || clearNeeded != tt.clearNeeded {
				t.Errorf("failureReason() = %q, %v, want %q, %v", reason, clearNeeded, tt.wantReason, tt.clearNeeded)
			}
		})
	}
}

func TestWithClearHint(t *testing.T) {
	t.Parallel()
	if got := withClearHint("x"); !strings.HasPrefix(got, "x.") || !strings.HasSuffix(got, clearInputHint) {
		t.Errorf("withClearHint() = %q", got)
	}
}

type failingPaster struct {
	failAt int
	pasted []string
}

func (p *failingPaster) TypeTextBy(s string, _ bool) error {
	if len(p.pasted) == p.failAt {
		return errors.New("paste failed")
	}
	p.pasted = append(p.pasted, s)
	return nil
}

func TestImageSetPaste_MarksAnImageOnlyAfterItsPaste(t *testing.T) {
	t.Parallel()
	var marked []string
	imgs := imageSet{
		ids:   []string{"a", "b", "c"},
		paths: []string{"/a", "/b", "/c"},
		sent:  func(id string) { marked = append(marked, id) },
	}
	c := &failingPaster{failAt: 1}
	if err := imgs.paste(c); err == nil {
		t.Fatal("paste = nil, want the second paste's error")
	}
	if want := []string{"a"}; !slices.Equal(marked, want) {
		t.Errorf("marked = %v, want %v", marked, want)
	}
}

func TestRenameCommandArg(t *testing.T) {
	cases := []struct {
		in   string
		name string
		ok   bool
	}{
		{"/rename blabla", "blabla", true},
		{"  /name  my agent ", "my agent", true},
		{"/rename", "", false},
		{"/rename   ", "", false},
		{"/renamed x", "", false},
		{"hello /rename x", "", false},
		{"/rename a\nb", "", false},
	}
	for _, c := range cases {
		name, ok := renameCommandArg(c.in)
		if name != c.name && ok || ok != c.ok {
			t.Errorf("renameCommandArg(%q) = %q, %v; want %q, %v", c.in, name, ok, c.name, c.ok)
		}
	}
}

func TestRenameSlots_OnlyANewerRequestOwnsTheSlot(t *testing.T) {
	var s renameSlots
	a := s.put("s1", "/rename A")
	b := s.put("s1", "/rename B")
	if s.owns("s1", a) || !s.owns("s1", b) {
		t.Fatalf("owns(a)=%v owns(b)=%v, want only the newer", s.owns("s1", a), s.owns("s1", b))
	}
	s.settle("s1", a) // the older request finishing must not empty B's command
	if _, got := s.current("s1"); got != "/rename B" {
		t.Errorf("held = %q, want B kept", got)
	}
	s.settle("s1", b)
	if _, got := s.current("s1"); got != "" {
		t.Errorf("held = %q, want empty once B settled", got)
	}
}

func TestRenameSlots_EmptyRequestSupersedesAParkedOne(t *testing.T) {
	var s renameSlots
	s.put("s1", "/rename A")
	s.put("s1", "")
	if _, got := s.current("s1"); got != "" {
		t.Errorf("held = %q, want the held command replaced", got)
	}
}

// A chat that turns Blocked between the request and the send keeps its
// command held and gets the hold watcher the request path would have started.
func TestFinishRename_BlockedAtSendKeepsTheCommandAndClaimsTheWatcher(t *testing.T) {
	p := newLockPrompt()
	p.sessions.Watches.Close() // Start then runs nothing, so the claim stays visible
	seq := p.renames.put("s1", "/rename B")
	(namer{p}).finishRename("/dir/s1", "s1", seq, "/rename B", errRenameBlocked)
	if _, got := p.renames.current("s1"); got != "/rename B" || p.renames.claimWatch("s1") {
		t.Errorf("held = %q, want the command kept with a claimed watcher", got)
	}
}
