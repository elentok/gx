package terminalrun

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/notify"
)

func TestWrapRun_PrependsGxRun(t *testing.T) {
	prev := osExecutable
	osExecutable = func() (string, error) { return "/usr/local/bin/gx", nil }
	defer func() { osExecutable = prev; resetGxPath() }()
	resetGxPath()

	program, args := wrapRun("git", []string{"commit", "-m", "msg"})
	if program != "/usr/local/bin/gx" {
		t.Fatalf("program = %q, want resolved gx path", program)
	}
	want := []string{"run", "git", "commit", "-m", "msg"}
	if len(args) != len(want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args = %#v, want %#v", args, want)
		}
	}
}

func TestResolveGxPath_FallsBackToGx(t *testing.T) {
	prev := osExecutable
	osExecutable = func() (string, error) { return "", errors.New("unavailable") }
	defer func() { osExecutable = prev }()

	if got := resolveGxPath(); got != "gx" {
		t.Fatalf("resolveGxPath() = %q, want gx (fallback)", got)
	}
}

func TestCommand_ReturnsCmd(t *testing.T) {
	doneFn := func(err error, splitApp string) tea.Msg { return nil }
	cmd := Command("/tmp", ui.TerminalPlain, "echo", []string{"hello"}, doneFn)
	if cmd == nil {
		t.Error("expected non-nil cmd from Command()")
	}
}

func TestCommandWithSplit(t *testing.T) {
	doneFn := func(err error, splitApp string) tea.Msg { return nil }

	tests := []struct {
		name      string
		terminal  ui.Terminal
		splitType SplitType
		wantWarn  bool
	}{
		{name: "inplace/plain", terminal: ui.TerminalPlain, splitType: InPlace, wantWarn: false},
		{name: "inplace/tmux", terminal: ui.TerminalTmux, splitType: InPlace, wantWarn: false},
		{name: "inplace/kitty-remote", terminal: ui.TerminalKittyRemote, splitType: InPlace, wantWarn: false},
		{name: "hsplit/tmux", terminal: ui.TerminalTmux, splitType: HSplit, wantWarn: false},
		{name: "vsplit/tmux", terminal: ui.TerminalTmux, splitType: VSplit, wantWarn: false},
		{name: "tab/tmux", terminal: ui.TerminalTmux, splitType: Tab, wantWarn: false},
		{name: "hsplit/kitty-remote", terminal: ui.TerminalKittyRemote, splitType: HSplit, wantWarn: false},
		{name: "vsplit/kitty-remote", terminal: ui.TerminalKittyRemote, splitType: VSplit, wantWarn: false},
		{name: "tab/kitty-remote", terminal: ui.TerminalKittyRemote, splitType: Tab, wantWarn: false},
		{name: "hsplit/plain", terminal: ui.TerminalPlain, splitType: HSplit, wantWarn: true},
		{name: "vsplit/plain", terminal: ui.TerminalPlain, splitType: VSplit, wantWarn: true},
		{name: "tab/plain", terminal: ui.TerminalPlain, splitType: Tab, wantWarn: true},
		{name: "hsplit/kitty", terminal: ui.TerminalKitty, splitType: HSplit, wantWarn: true},
		{name: "vsplit/kitty", terminal: ui.TerminalKitty, splitType: VSplit, wantWarn: true},
		{name: "tab/kitty", terminal: ui.TerminalKitty, splitType: Tab, wantWarn: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := CommandWithSplit("/tmp", tt.terminal, tt.splitType, "echo", []string{"hello"}, doneFn)
			if cmd == nil {
				t.Fatal("expected non-nil cmd")
			}
			if tt.wantWarn {
				msg := cmd()
				nm, ok := msg.(notify.NotifyMsg)
				if !ok || nm.Kind != notify.KindWarning {
					t.Fatalf("expected warning notify msg, got %T %v", msg, msg)
				}
			}
		})
	}
}

func TestCommandWithSplitBare_ReturnsCmd(t *testing.T) {
	doneFn := func(err error, splitApp string) tea.Msg { return nil }
	cmd := CommandWithSplitBare("/tmp", ui.TerminalPlain, InPlace, "fish", nil, doneFn)
	if cmd == nil {
		t.Error("expected non-nil cmd from CommandWithSplitBare()")
	}
}
