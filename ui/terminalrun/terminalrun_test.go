package terminalrun

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/notify"
)

func TestSplitShellCommand_DefaultShell(t *testing.T) {
	t.Setenv("SHELL", "")
	program, args := splitShellCommand("git rebase -i abc123", true)
	if program != "sh" {
		t.Fatalf("program = %q, want sh", program)
	}
	if len(args) != 2 || args[0] != "-lc" {
		t.Fatalf("args = %#v, want -lc script", args)
	}
	script := args[1]
	if !strings.Contains(script, "read -r _") {
		t.Fatalf("expected POSIX read in script: %q", script)
	}
	if !strings.Contains(script, "gx: COMMAND FAILED, press Enter to close") {
		t.Fatalf("missing failure prompt in script: %q", script)
	}
	if !strings.Contains(script, "--- gx: command finished, press Enter to close ---") {
		t.Fatalf("missing success prompt in script: %q", script)
	}
}

func TestSplitShellCommand_FishShell(t *testing.T) {
	t.Setenv("SHELL", "/usr/bin/fish")
	program, args := splitShellCommand("git rebase -i abc123", true)
	if program != "/usr/bin/fish" {
		t.Fatalf("program = %q, want fish", program)
	}
	script := args[1]
	if !strings.Contains(script, "set code $status") {
		t.Fatalf("missing fish exit status capture in script: %q", script)
	}
	if !strings.Contains(script, "read -P '' _") {
		t.Fatalf("expected fish read in script: %q", script)
	}
}

func TestEscapeShellArg(t *testing.T) {
	got := escapeShellArg("a'b c")
	want := "'a'\\''b c'"
	if got != want {
		t.Fatalf("escapeShellArg() = %q, want %q", got, want)
	}
}

func TestCommand_ReturnsCmd(t *testing.T) {
	doneFn := func(err error, splitApp string) tea.Msg { return nil }
	cmd := Command("/tmp", ui.TerminalPlain, "echo", []string{"hello"}, doneFn)
	if cmd == nil {
		t.Error("expected non-nil cmd from Command()")
	}
}

func TestCommandCustom_KeepOpen(t *testing.T) {
	doneFn := func(err error, splitApp string) tea.Msg { return nil }
	cmd := CommandCustom("/tmp", ui.TerminalPlain, "echo", []string{"hello"}, true, doneFn)
	if cmd == nil {
		t.Error("expected non-nil cmd from CommandCustom() with keepOpen=true")
	}
}

func TestCommandWithSplit(t *testing.T) {
	t.Parallel()
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
