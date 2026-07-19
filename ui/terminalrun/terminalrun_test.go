package terminalrun

import (
	"errors"
	"reflect"
	"strings"
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

	program, args := WrapRun("git", []string{"commit", "-m", "msg"})
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
		{name: "inplace/herdr", terminal: ui.TerminalHerdr, splitType: InPlace, wantWarn: false},
		{name: "hsplit/herdr", terminal: ui.TerminalHerdr, splitType: HSplit, wantWarn: false},
		{name: "vsplit/herdr", terminal: ui.TerminalHerdr, splitType: VSplit, wantWarn: false},
		{name: "tab/herdr", terminal: ui.TerminalHerdr, splitType: Tab, wantWarn: false},
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

func TestLaunchSplit_ArgVectors(t *testing.T) {
	tests := []struct {
		name      string
		terminal  ui.Terminal
		splitType SplitType
		wantName  string
		wantArgs  []string
		wantApp   string
	}{
		{
			// HSplit (vim :split) = stacked, maps to tmux -v.
			name: "tmux/hsplit", terminal: ui.TerminalTmux, splitType: HSplit, wantApp: "tmux",
			wantName: "tmux", wantArgs: []string{"split-window", "-v", "-c", "/wt", "gx", "run", "lazygit"},
		},
		{
			// VSplit (vim :vsplit) = side-by-side, maps to tmux -h.
			name: "tmux/vsplit", terminal: ui.TerminalTmux, splitType: VSplit, wantApp: "tmux",
			wantName: "tmux", wantArgs: []string{"split-window", "-h", "-c", "/wt", "gx", "run", "lazygit"},
		},
		{
			name: "tmux/tab", terminal: ui.TerminalTmux, splitType: Tab, wantApp: "tmux",
			wantName: "tmux", wantArgs: []string{"new-window", "-c", "/wt", "gx", "run", "lazygit"},
		},
		{
			// HSplit (vim :split, stacked) maps to kitty's hsplit — see ADR 0005.
			name: "kitty/hsplit", terminal: ui.TerminalKittyRemote, splitType: HSplit, wantApp: "kitty",
			wantName: "kitty", wantArgs: []string{"@", "launch", "--copy-env", "--type=window", "--location=hsplit", "--cwd=/wt", "gx", "run", "lazygit"},
		},
		{
			// VSplit (vim :vsplit, side-by-side) maps to kitty's vsplit — see ADR 0005.
			name: "kitty/vsplit", terminal: ui.TerminalKittyRemote, splitType: VSplit, wantApp: "kitty",
			wantName: "kitty", wantArgs: []string{"@", "launch", "--copy-env", "--type=window", "--location=vsplit", "--cwd=/wt", "gx", "run", "lazygit"},
		},
		{
			name: "kitty/tab", terminal: ui.TerminalKittyRemote, splitType: Tab, wantApp: "kitty",
			wantName: "kitty", wantArgs: []string{"@", "launch", "--copy-env", "--type=tab", "--cwd=/wt", "gx", "run", "lazygit"},
		},
		{
			// HSplit (vim :split, stacked) maps to herdr's "down" split.
			name: "herdr/hsplit", terminal: ui.TerminalHerdr, splitType: HSplit, wantApp: "herdr",
			wantName: "herdr", wantArgs: []string{"agent", "start", "gx", "--cwd", "/wt", "--split", "down", "--focus", "--", "gx", "run", "lazygit"},
		},
		{
			// VSplit (vim :vsplit, side-by-side) maps to herdr's "right" split.
			name: "herdr/vsplit", terminal: ui.TerminalHerdr, splitType: VSplit, wantApp: "herdr",
			wantName: "herdr", wantArgs: []string{"agent", "start", "gx", "--cwd", "/wt", "--split", "right", "--focus", "--", "gx", "run", "lazygit"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotName string
			var gotArgs []string
			prev := runCommand
			runCommand = func(name string, args ...string) ([]byte, error) {
				gotName, gotArgs = name, args
				return nil, nil
			}
			defer func() { runCommand = prev }()

			app, err := launchSplit("/wt", tt.terminal, tt.splitType, "gx", []string{"run", "lazygit"})
			if err != nil {
				t.Fatalf("launchSplit() error = %v", err)
			}
			if app != tt.wantApp {
				t.Errorf("app = %q, want %q", app, tt.wantApp)
			}
			if gotName != tt.wantName {
				t.Errorf("command = %q, want %q", gotName, tt.wantName)
			}
			if !reflect.DeepEqual(gotArgs, tt.wantArgs) {
				t.Errorf("args = %#v, want %#v", gotArgs, tt.wantArgs)
			}
		})
	}
}

func TestLaunchSplit_HerdrTabCreatesTabThenStartsAgent(t *testing.T) {
	prev := runCommand
	var calls [][]string
	runCommand = func(name string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{name}, args...))
		if len(calls) == 1 {
			return []byte(`{"result":{"tab":{"tab_id":"w2:t9"}}}`), nil
		}
		return nil, nil
	}
	defer func() { runCommand = prev }()

	app, err := launchSplit("/wt", ui.TerminalHerdr, Tab, "gx", []string{"run", "lazygit"})
	if err != nil {
		t.Fatalf("launchSplit() error = %v", err)
	}
	if app != "herdr" {
		t.Errorf("app = %q, want herdr", app)
	}
	if len(calls) != 2 {
		t.Fatalf("expected 2 herdr calls, got %d: %#v", len(calls), calls)
	}
	wantTabCreate := []string{"herdr", "tab", "create", "--cwd", "/wt"}
	if !reflect.DeepEqual(calls[0], wantTabCreate) {
		t.Errorf("first call = %#v, want %#v", calls[0], wantTabCreate)
	}
	wantAgentStart := []string{"herdr", "agent", "start", "gx", "--tab", "w2:t9", "--focus", "--", "gx", "run", "lazygit"}
	if !reflect.DeepEqual(calls[1], wantAgentStart) {
		t.Errorf("second call = %#v, want %#v", calls[1], wantAgentStart)
	}
}

func TestLaunchSplit_HerdrErrorIncludesCommandAndOutput(t *testing.T) {
	prev := runCommand
	runCommand = func(name string, args ...string) ([]byte, error) {
		return []byte("no such agent name\n"), errors.New("exit status 1")
	}
	defer func() { runCommand = prev }()

	_, err := launchSplit("/wt", ui.TerminalHerdr, VSplit, "gx", []string{"run", "lazygit"})
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	for _, want := range []string{"$ herdr agent start", "exit status 1", "no such agent name"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
}

func TestLaunchSplit_KittyErrorIncludesCommandAndOutput(t *testing.T) {
	prev := runCommand
	runCommand = func(name string, args ...string) ([]byte, error) {
		return []byte("no listening socket\n"), errors.New("exit status 1")
	}
	defer func() { runCommand = prev }()

	_, err := launchSplit("/wt", ui.TerminalKittyRemote, VSplit, "gx", []string{"run", "lazygit"})
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	for _, want := range []string{"$ kitty @ launch", "exit status 1", "no listening socket"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
}

func TestCommandWithSplitBare_ReturnsCmd(t *testing.T) {
	doneFn := func(err error, splitApp string) tea.Msg { return nil }
	cmd := CommandWithSplitBare("/tmp", ui.TerminalPlain, InPlace, "fish", nil, doneFn)
	if cmd == nil {
		t.Error("expected non-nil cmd from CommandWithSplitBare()")
	}
}
