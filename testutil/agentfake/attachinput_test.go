package agentfake

import (
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner/screen"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner"
)

// The fake program echoes what it reads but drops each ESC, so the screen
// shows how the holder encoded the input: "[A" for a plain arrow, "OA" for
// one in application cursor mode.

func holdProgramAttach(t *testing.T) (*heldProgram, *nativerunner.HolderAttach) {
	t.Helper()
	conv := newConversationID(t)
	p := holdProgram(t, Harness{}.Launch(harness.LaunchOptions{SessionID: conv}), conv)
	// The program records its conversation once its tty is raw; input before
	// that is echoed by the tty itself and then read again.
	p.waitFor("the conversation id", func() bool { return p.watcher.ConversationID() == conv })
	a, err := p.client.Attach()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	return p, a
}

// writeOutput makes the program write out and waits for the holder's screen
// to take it in, so input sent next meets the new modes.
func (p *heldProgram) writeOutput(out, marker string) {
	p.t.Helper()
	p.control(Control{Action: ActionWrite, Output: out + marker})
	p.waitScreen(marker)
}

func TestAttachInput_KeyReachesChildAndNextFrameShowsIt(t *testing.T) {
	p, a := holdProgramAttach(t)
	frames := make(chan nativerunner.Frame)
	go func() {
		for {
			m, err := a.Next()
			if err != nil {
				return
			}
			if m.Type == nativerunner.StreamFrame {
				frames <- m.Frame
			}
		}
	}()
	<-frames

	for _, r := range "hi" {
		if err := a.SendKey(screen.Key{Code: r, Text: string(r)}); err != nil {
			t.Fatal(err)
		}
	}
	timeout := time.After(10 * time.Second)
	for seen := ""; !strings.Contains(seen, "hi"); {
		select {
		case f := <-frames:
			for _, r := range f.Rows {
				seen += r.ANSI
			}
		case <-timeout:
			t.Fatalf("no frame shows the typed keys; rows seen: %q, screen %q", seen, p.rows())
		}
	}
}

func TestAttachInput_KeysAreEncodedForTheChildsModes(t *testing.T) {
	p, a := holdProgramAttach(t)
	if err := a.SendKey(screen.Key{Code: screen.KeyUp}); err != nil {
		t.Fatal(err)
	}
	p.waitScreen("[A")

	p.writeOutput("\x1b[?1h", "app-cursor")
	if err := a.SendKey(screen.Key{Code: screen.KeyUp}); err != nil {
		t.Fatal(err)
	}
	p.waitScreen("app-cursorOA")
}

func TestAttachInput_PasteIsBracketedOnlyWhenChildEnabledIt(t *testing.T) {
	p, a := holdProgramAttach(t)
	if err := a.Paste("plain"); err != nil {
		t.Fatal(err)
	}
	p.waitScreen("plain")
	if s := strings.Join(p.rows(), "\n"); strings.Contains(s, "200~") {
		t.Errorf("paste without bracketed paste mode = %q; want it plain", s)
	}

	p.writeOutput("\x1b[?2004h", "bracketed")
	if err := a.Paste("wrapped"); err != nil {
		t.Fatal(err)
	}
	p.waitScreen("[200~wrapped[201~")
}

func TestAttachInput_TerminalQueryGetsReply(t *testing.T) {
	p, _ := holdProgramAttach(t)
	// A primary device attributes reply is CSI ? ... c.
	p.control(Control{Action: ActionWrite, Output: "\x1b[c"})
	p.waitScreen("[?")
}

func TestAttachInput_KeyEmitsInputEventOnWatch(t *testing.T) {
	p, a := holdProgramAttach(t)
	watch, err := p.client.Watch()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = watch.Close() })
	if m, err := watch.Next(); err != nil || m.Type != nativerunner.StreamStatus {
		t.Fatalf("first watch message = %+v, %v; want a status", m, err)
	}

	if err := a.SendKey(screen.Key{Code: 'x', Text: "x"}); err != nil {
		t.Fatal(err)
	}
	got := make(chan error, 1)
	go func() {
		for {
			m, err := watch.Next()
			if err != nil || m.Type == nativerunner.StreamInput {
				got <- err
				return
			}
		}
	}()
	select {
	case err := <-got:
		if err != nil {
			t.Fatalf("watch: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no input event on the watch stream")
	}
}
