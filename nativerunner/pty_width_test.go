package nativerunner_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/testutil/agentfake"
)

const narrowCols = 120

// viewerResized makes a viewer own the pty at cols×rows, as the live panel does
// when a person focuses it, and returns once the pty has taken the size.
func viewerResized(t *testing.T, p *nativerunner.Pty, s agentrunner.Session, cols, rows int) {
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
	if err := a.Resize(cols, rows); err != nil {
		t.Fatal(err)
	}
	for {
		m, err := a.Next()
		if err != nil {
			t.Fatal(err)
		}
		if m.Type != nativerunner.StreamFrame {
			continue
		}
		for _, r := range m.Frame.Rows {
			if ansi.StringWidth(r.ANSI) == cols && len(m.Frame.Rows) == rows {
				return
			}
		}
	}
}

// longWords is a command wider than narrowCols that wraps between words.
func longWords() string {
	return strings.TrimSpace(strings.Repeat("rg --files --hidden ./internal/some/deep/path ", 4))
}

func TestPty_PermissionDialogAtNarrowWidth(t *testing.T) {
	t.Parallel()
	cmd := longWords()
	wide := newPty(t)
	ws := blockedOnDialog(t, wide, agentfake.BashPermissionDialog(cmd))
	wantID := pendingOf(t, wide, ws).ID

	p := newPty(t)
	s := blockedOnDialog(t, p, agentfake.BashPermissionDialog(cmd))
	viewerResized(t, p, s, narrowCols, 30)

	pend := pendingOf(t, p, s)
	if pend == nil || pend.Tool != "Bash command" || pend.Description != cmd {
		t.Fatalf("Pending = %+v; want the whole Bash command %q", pend, cmd)
	}
	if pend.ID != wantID {
		t.Errorf("ID = %q at %d columns, %q at 200; want the same", pend.ID, narrowCols, wantID)
	}
	if err := p.Answer(s, agentrunner.Answer{Decision: agentrunner.DecisionAllow, RequestID: pend.ID}); err != nil {
		t.Fatalf("Allow = %v", err)
	}
}

func TestPty_DenyAtNarrowWidth(t *testing.T) {
	t.Parallel()
	p := newPty(t)
	s := blockedOnDialog(t, p, agentfake.BashPermissionDialog(longWords()))
	viewerResized(t, p, s, narrowCols, 30)

	if err := p.Answer(s, agentrunner.Answer{Decision: agentrunner.DecisionDeny, RequestID: pendingOf(t, p, s).ID}); err != nil {
		t.Fatalf("Deny = %v", err)
	}
}

func TestPty_LongPromptDeliveredAtNarrowWidth(t *testing.T) {
	t.Parallel()
	p := newPty(t)
	s, err := p.Start(agentrunner.StartOptions{Label: "e-01", Epic: "e", Cwd: t.TempDir(), Kind: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop(s)
	viewerResized(t, p, s, narrowCols, 30)

	text := strings.Repeat("please change the thing ", 20)
	if err := p.Prompt(s, text); err != nil {
		t.Fatalf("Prompt = %v", err)
	}
	in, err := os.ReadFile(agentfake.TUIInputPath(ptyHome(p), s.SessionID))
	if err != nil {
		t.Fatal(err)
	}
	if string(in) != text+"\r" {
		t.Errorf("keys typed = %q; want the prompt once, then Enter", in)
	}
}
