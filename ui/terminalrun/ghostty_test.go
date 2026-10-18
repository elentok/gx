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

type osaCall struct {
	name string
	args []string
}

// fakeOsa replaces runCommand, recording calls and answering with out/err.
func fakeOsa(t *testing.T, out string, err error) *[]osaCall {
	t.Helper()
	var calls []osaCall
	prev := runCommand
	runCommand = func(name string, args ...string) ([]byte, error) {
		calls = append(calls, osaCall{name, args})
		return []byte(out), err
	}
	t.Cleanup(func() { runCommand = prev })
	return &calls
}

func TestLaunchSplit_GhosttyOsascriptVectors(t *testing.T) {
	t.Setenv("TERM_PROGRAM_VERSION", "1.3.1")
	tests := []struct {
		name      string
		splitType SplitType
		script    string
		program   string
		args      []string
		shell     bool
		wantInput string
	}{
		{"hsplit", HSplit, ghosttyHSplitScript, "/gx", []string{"run", "vim"}, false, "'/gx' 'run' 'vim'; exit\n"},
		{"vsplit", VSplit, ghosttyVSplitScript, "/gx", []string{"run", "vim"}, false, "'/gx' 'run' 'vim'; exit\n"},
		{"tab", Tab, ghosttyTabScript, "/gx", []string{"run", "vim"}, false, "'/gx' 'run' 'vim'; exit\n"},
		{"space and quote args", HSplit, ghosttyHSplitScript, "/gx", []string{"run", "a b", "it's"}, false, `'/gx' 'run' 'a b' 'it'\''s'; exit` + "\n"},
		{"bare drops gx run", VSplit, ghosttyVSplitScript, "claude", []string{"--resume", "x"}, false, "'claude' '--resume' 'x'; exit\n"},
		{"shell sends no input", HSplit, ghosttyHSplitScript, "/bin/zsh", nil, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := fakeOsa(t, "", nil)
			app, err := launchSplitAs("/wt", ui.TerminalGhostty, tt.splitType, tt.program, tt.args, tt.shell)
			if err != nil || app != "ghostty" {
				t.Fatalf("got (%q, %v)", app, err)
			}
			want := []osaCall{{"osascript", []string{"-e", tt.script, "/wt", tt.wantInput}}}
			if !reflect.DeepEqual(*calls, want) {
				t.Fatalf("calls = %#v\nwant %#v", *calls, want)
			}
		})
	}
}

func TestLaunchSplit_GhosttyVersionGate(t *testing.T) {
	for _, v := range []string{"1.2.9", "", "garbage", "1.3", "0.9.0"} {
		t.Run("blocked/"+v, func(t *testing.T) {
			t.Setenv("TERM_PROGRAM_VERSION", v)
			calls := fakeOsa(t, "", nil)
			_, err := launchSplit("/wt", ui.TerminalGhostty, HSplit, "x", nil)
			var fb *FallbackError
			if !errors.As(err, &fb) || !strings.Contains(fb.Hint, "needs 1.3.0 or later") {
				t.Fatalf("err = %v", err)
			}
			if len(*calls) != 0 {
				t.Fatalf("osascript called: %v", *calls)
			}
		})
	}
	for _, v := range []string{"1.3.0", "1.3.1", "1.4.0", "1.3.1-dev+abc", "2.0.0+build"} {
		t.Run("allowed/"+v, func(t *testing.T) {
			t.Setenv("TERM_PROGRAM_VERSION", v)
			calls := fakeOsa(t, "", nil)
			if _, err := launchSplit("/wt", ui.TerminalGhostty, HSplit, "x", nil); err != nil {
				t.Fatal(err)
			}
			if len(*calls) != 1 {
				t.Fatalf("calls = %v", *calls)
			}
		})
	}
}

func TestCheckGhosttySplits_Hints(t *testing.T) {
	tests := []struct{ version, want string }{
		{"", "Ghostty unknown has no AppleScript splits (needs 1.3.0 or later)"},
		{"junk", `Ghostty "junk" (unparsable) has no AppleScript splits (needs 1.3.0 or later)`},
		{"1.2.3", "Ghostty 1.2.3 has no AppleScript splits (needs 1.3.0 or later)"},
		{"1.3.0", ""},
	}
	for _, tt := range tests {
		err := CheckGhosttySplits(tt.version)
		if tt.want == "" {
			if err != nil {
				t.Errorf("%q: err = %v", tt.version, err)
			}
			continue
		}
		var fb *FallbackError
		if !errors.As(err, &fb) || fb.Hint != tt.want {
			t.Errorf("%q: err = %v, want %q", tt.version, err, tt.want)
		}
	}
}

func TestLaunchSplit_GhosttyFailureHints(t *testing.T) {
	tests := []struct {
		name     string
		out      string
		wantHint string
		fallback bool
	}{
		{"macos-applescript", "error -1743 macos-applescript is disabled", "Ghostty: set macos-applescript = true in your Ghostty config", true},
		{"automation", "error -1743 not allowed", "allow gx/Ghostty under System Settings › Privacy & Security › Automation", true},
		{"-2740", "syntax -2740", "Ghostty split failed: syntax -2740", true},
		{"-2741", "syntax -2741", "Ghostty split failed: syntax -2741", true},
		{"-1708", "doesn't understand -1708", "Ghostty split failed: doesn't understand -1708", true},
		{"-1728", "no window -1728", "Ghostty split failed: no window -1728", true},
		{"unknown", "boom -1", "Ghostty split failed: boom -1", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TERM_PROGRAM_VERSION", "1.3.1")
			fakeOsa(t, tt.out+"\n", errors.New("exit status 1"))
			_, err := launchSplit("/wt", ui.TerminalGhostty, HSplit, "x", nil)
			var fb *FallbackError
			if got := errors.As(err, &fb); got != tt.fallback {
				t.Fatalf("fallback = %v, err = %v", got, err)
			}
			if err == nil || err.Error() != tt.wantHint {
				t.Fatalf("err = %v, want %q", err, tt.wantHint)
			}
		})
	}
}

func TestCommandWithSplit_GhosttyFallbackAndError(t *testing.T) {
	t.Setenv("TERM_PROGRAM_VERSION", "1.3.1")
	var gotErr error
	var gotApp string
	done := func(err error, splitApp string) tea.Msg {
		gotErr, gotApp = err, splitApp
		return nil
	}

	t.Run("fallback warns and execs in place", func(t *testing.T) {
		fakeOsa(t, "error -1743 macos-applescript", errors.New("exit status 1"))
		msg := CommandWithSplit("/tmp", ui.TerminalGhostty, HSplit, "echo", nil, done)()
		batch, ok := msg.(tea.BatchMsg)
		if !ok || len(batch) != 2 {
			t.Fatalf("msg = %T %v, want 2-cmd batch", msg, msg)
		}
		nm, ok := batch[0]().(notify.NotifyMsg)
		if !ok || nm.Kind != notify.KindWarning {
			t.Fatalf("first cmd = %T, want warning", nm)
		}
	})

	t.Run("unknown error reaches done, no exec", func(t *testing.T) {
		fakeOsa(t, "boom", errors.New("exit status 1"))
		msg := CommandWithSplit("/tmp", ui.TerminalGhostty, HSplit, "echo", nil, done)()
		if _, ok := msg.(tea.BatchMsg); ok {
			t.Fatal("unexpected batch")
		}
		if gotErr == nil || gotApp != "ghostty" {
			t.Fatalf("done got (%v, %q)", gotErr, gotApp)
		}
	})
}

func TestCommand_GhosttyUsesHSplitScript(t *testing.T) {
	t.Setenv("TERM_PROGRAM_VERSION", "1.3.1")
	calls := fakeOsa(t, "", nil)
	Command("/wt", ui.TerminalGhostty, "echo", []string{"hi"}, func(error, string) tea.Msg { return nil })()
	if len(*calls) != 1 || (*calls)[0].args[1] != ghosttyHSplitScript {
		t.Fatalf("calls = %v", *calls)
	}
}
