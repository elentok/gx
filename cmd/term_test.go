package cmd

import (
	"errors"
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/terminalrun"
)

func TestResolveSplitType(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		flags   termFlags
		want    terminalrun.SplitType
		wantErr bool
	}{
		{name: "default/none", flags: termFlags{}, want: terminalrun.HSplit},
		{name: "right", flags: termFlags{right: true}, want: terminalrun.VSplit},
		{name: "below", flags: termFlags{below: true}, want: terminalrun.HSplit},
		{name: "tab", flags: termFlags{tab: true}, want: terminalrun.Tab},
		{name: "here", flags: termFlags{here: true}, want: terminalrun.InPlace},
		{name: "conflict/right+tab", flags: termFlags{right: true, tab: true}, wantErr: true},
		{name: "conflict/here+below", flags: termFlags{here: true, below: true}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveSplitType(tt.flags)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("splitType = %v, want %v", got, tt.want)
			}
		})
	}
}

// capturedLaunch records the arguments passed to the split-launch seam.
type capturedLaunch struct {
	called    bool
	cwd       string
	terminal  ui.Terminal
	splitType terminalrun.SplitType
	program   string
	args      []string
	shell     bool
}

// capturedExec records the arguments passed to the in-place exec-replace seam.
type capturedExec struct {
	called  bool
	program string
	args    []string
	cwd     string
}

// withSeams swaps launchInSplit/execReplace for capturing fakes and restores
// them when the test ends. Both fakes return nil (no real launch/exec).
func withSeams(t *testing.T) (*capturedLaunch, *capturedExec) {
	t.Helper()
	var l capturedLaunch
	var e capturedExec
	prevLaunch, prevExec := launchInSplit, execReplace
	launchInSplit = func(cwd string, terminal ui.Terminal, st terminalrun.SplitType, program string, args []string, opts ...terminalrun.LaunchOption) (string, error) {
		l = capturedLaunch{called: true, cwd: cwd, terminal: terminal, splitType: st, program: program, args: args, shell: len(opts) > 0}
		return "", nil
	}
	execReplace = func(program string, args []string, cwd string) error {
		e = capturedExec{called: true, program: program, args: args, cwd: cwd}
		return nil
	}
	t.Cleanup(func() { launchInSplit, execReplace = prevLaunch, prevExec })
	return &l, &e
}

// envFunc builds a getenv from a map for terminal detection.
func envFunc(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestRunTerm_SplitWrapsExplicitCommand(t *testing.T) {
	// not parallel: shares package-level launchInSplit/execReplace seam vars with other TestRunTerm_* tests via withSeams
	l, e := withSeams(t)
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return "/wd", nil },
		getenv: envFunc(map[string]string{"TMUX": "/tmp/tmux-1,1,0"}),
	}

	err := execute([]string{"term", "--below", "nvim", "-u", "NONE", "file"}, d)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if e.called {
		t.Fatal("expected split path, got in-place exec")
	}
	if !l.called {
		t.Fatal("expected launchInSplit to be called")
	}
	if l.terminal != ui.TerminalTmux || l.splitType != terminalrun.HSplit {
		t.Fatalf("terminal/split = %v/%v, want tmux/HSplit", l.terminal, l.splitType)
	}
	// Explicit command is wrapped in `gx run`, with its own flags passed through.
	wantArgs := []string{"run", "nvim", "-u", "NONE", "file"}
	if !reflect.DeepEqual(l.args, wantArgs) {
		t.Fatalf("launch args = %#v, want %#v", l.args, wantArgs)
	}
	if l.program == "" {
		t.Fatal("expected non-empty gx program path for wrapped command")
	}
	if l.shell {
		t.Fatal("explicit command must not be launched as a shell")
	}
}

func TestRunTerm_NoCommandLaunchesShellBare(t *testing.T) {
	// not parallel: shares package-level launchInSplit/execReplace seam vars with other TestRunTerm_* tests via withSeams
	l, _ := withSeams(t)
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return "/wd", nil },
		getenv: envFunc(map[string]string{"TMUX": "x", "SHELL": "/usr/bin/fish"}),
	}

	if err := execute([]string{"term", "--right"}, d); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !l.called {
		t.Fatal("expected launchInSplit to be called")
	}
	if l.splitType != terminalrun.VSplit {
		t.Fatalf("splitType = %v, want VSplit", l.splitType)
	}
	// Shell is launched bare — not wrapped in `gx run`.
	if l.program != "/usr/bin/fish" {
		t.Fatalf("program = %q, want /usr/bin/fish (bare shell)", l.program)
	}
	if len(l.args) != 0 {
		t.Fatalf("args = %#v, want empty (bare shell)", l.args)
	}
	if !l.shell {
		t.Fatal("no-command launch must use the shell option")
	}
}

func TestRunTerm_ShellFallsBackToSh(t *testing.T) {
	// not parallel: shares package-level launchInSplit/execReplace seam vars with other TestRunTerm_* tests via withSeams
	l, _ := withSeams(t)
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return "/wd", nil },
		getenv: envFunc(map[string]string{"TMUX": "x"}),
	}
	if err := execute([]string{"term"}, d); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if l.program != "/bin/sh" {
		t.Fatalf("program = %q, want /bin/sh fallback", l.program)
	}
}

func TestRunTerm_CwdOverride(t *testing.T) {
	// not parallel: shares package-level launchInSplit/execReplace seam vars with other TestRunTerm_* tests via withSeams
	l, _ := withSeams(t)
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return "/from/getwd", nil },
		getenv: envFunc(map[string]string{"TMUX": "x"}),
	}
	if err := execute([]string{"term", "--cwd", "/explicit/dir", "lazygit"}, d); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if l.cwd != "/explicit/dir" {
		t.Fatalf("cwd = %q, want /explicit/dir", l.cwd)
	}
}

func TestRunTerm_CwdDefaultsToGetwd(t *testing.T) {
	// not parallel: shares package-level launchInSplit/execReplace seam vars with other TestRunTerm_* tests via withSeams
	l, _ := withSeams(t)
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return "/from/getwd", nil },
		getenv: envFunc(map[string]string{"TMUX": "x"}),
	}
	if err := execute([]string{"term", "lazygit"}, d); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if l.cwd != "/from/getwd" {
		t.Fatalf("cwd = %q, want /from/getwd", l.cwd)
	}
}

func TestRunTerm_HereRunsInPlace(t *testing.T) {
	// not parallel: shares package-level launchInSplit/execReplace seam vars with other TestRunTerm_* tests via withSeams
	l, e := withSeams(t)
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return "/wd", nil },
		// Even inside tmux, --here forces in-place.
		getenv: envFunc(map[string]string{"TMUX": "x"}),
	}
	if err := execute([]string{"term", "--here", "ls", "-la"}, d); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if l.called {
		t.Fatal("expected in-place exec, got split launch")
	}
	if !e.called {
		t.Fatal("expected execReplace to be called")
	}
	// In-place is never wrapped.
	if e.program != "ls" || !reflect.DeepEqual(e.args, []string{"-la"}) {
		t.Fatalf("exec program/args = %q/%#v, want ls/[-la]", e.program, e.args)
	}
	if e.cwd != "/wd" {
		t.Fatalf("exec cwd = %q, want /wd", e.cwd)
	}
}

func TestRunTerm_PlainTerminalFallsBackInPlaceSilently(t *testing.T) {
	// not parallel: shares package-level launchInSplit/execReplace seam vars with other TestRunTerm_* tests via withSeams
	l, e := withSeams(t)
	var stderr bytes.Buffer
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: &stderr,
		getwd:  func() (string, error) { return "/wd", nil },
		getenv: envFunc(map[string]string{}), // plain terminal
	}
	if err := execute([]string{"term", "--below", "lazygit"}, d); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if l.called {
		t.Fatal("expected in-place exec on plain terminal")
	}
	if !e.called {
		t.Fatal("expected execReplace to be called")
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected silent fallback on plain terminal, got stderr: %q", stderr.String())
	}
}

func TestRunTerm_KittyWithoutRemotePrintsHint(t *testing.T) {
	// not parallel: shares package-level launchInSplit/execReplace seam vars with other TestRunTerm_* tests via withSeams
	l, e := withSeams(t)
	var stderr bytes.Buffer
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: &stderr,
		getwd:  func() (string, error) { return "/wd", nil },
		getenv: envFunc(map[string]string{"KITTY_WINDOW_ID": "1"}), // kitty, no remote
	}
	if err := execute([]string{"term", "--right", "lazygit"}, d); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if l.called {
		t.Fatal("expected in-place exec on kitty-without-remote")
	}
	if !e.called {
		t.Fatal("expected execReplace to be called")
	}
	if !strings.Contains(stderr.String(), "remote control") {
		t.Fatalf("expected kitty remote-control hint on stderr, got: %q", stderr.String())
	}
}

func TestRunTerm_GoosReachesDetection(t *testing.T) {
	// not parallel: shares package-level launchInSplit/execReplace seam vars with other TestRunTerm_* tests via withSeams
	l, _ := withSeams(t)
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return "/wd", nil },
		getenv: envFunc(map[string]string{"TERM_PROGRAM": "ghostty"}),
		goos:   "darwin",
	}
	if err := execute([]string{"term", "--right", "lazygit"}, d); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !l.called || l.terminal != ui.TerminalGhostty {
		t.Fatalf("expected ghostty split launch, got called=%v terminal=%v", l.called, l.terminal)
	}
}

func TestRunTerm_KittyRemoteUsesSplit(t *testing.T) {
	// not parallel: shares package-level launchInSplit/execReplace seam vars with other TestRunTerm_* tests via withSeams
	l, _ := withSeams(t)
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return "/wd", nil },
		getenv: envFunc(map[string]string{"KITTY_LISTEN_ON": "unix:/tmp/k"}),
	}
	if err := execute([]string{"term", "--tab", "lazygit"}, d); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !l.called {
		t.Fatal("expected split launch on kitty-remote")
	}
	if l.terminal != ui.TerminalKittyRemote || l.splitType != terminalrun.Tab {
		t.Fatalf("terminal/split = %v/%v, want kitty-remote/Tab", l.terminal, l.splitType)
	}
}

func TestRunTerm_DashTerminator(t *testing.T) {
	// not parallel: shares package-level launchInSplit/execReplace seam vars with other TestRunTerm_* tests via withSeams
	l, _ := withSeams(t)
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return "/wd", nil },
		getenv: envFunc(map[string]string{"TMUX": "x"}),
	}
	// `--` lets a program name starting with `-` (or flag-like args) pass through.
	if err := execute([]string{"term", "--below", "--", "myprog", "--right"}, d); err != nil {
		t.Fatalf("execute: %v", err)
	}
	wantArgs := []string{"run", "myprog", "--right"}
	if !reflect.DeepEqual(l.args, wantArgs) {
		t.Fatalf("launch args = %#v, want %#v", l.args, wantArgs)
	}
}

func TestRunTerm_ConflictingDirectionsError(t *testing.T) {
	// not parallel: shares package-level launchInSplit/execReplace seam vars with other TestRunTerm_* tests via withSeams
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		getenv: envFunc(map[string]string{"TMUX": "x"}),
	}
	err := execute([]string{"term", "--right", "--tab", "lazygit"}, d)
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected mutually-exclusive error, got: %v", err)
	}
}

func ghosttyDeps(stderr *bytes.Buffer) deps {
	return deps{
		stdout: bytes.NewBuffer(nil),
		stderr: stderr,
		getwd:  func() (string, error) { return "/wd", nil },
		getenv: envFunc(map[string]string{"TERM_PROGRAM": "ghostty"}),
		goos:   "darwin",
	}
}

func TestRunTerm_GhosttyFallbackErrorRunsInPlace(t *testing.T) {
	// not parallel: shares package-level launchInSplit/execReplace seam vars with other TestRunTerm_* tests via withSeams
	_, e := withSeams(t)
	launchInSplit = func(string, ui.Terminal, terminalrun.SplitType, string, []string, ...terminalrun.LaunchOption) (string, error) {
		return "", &terminalrun.FallbackError{Hint: "Ghostty: set macos-applescript = true in your Ghostty config"}
	}
	var stderr bytes.Buffer

	if err := execute([]string{"term", "--right", "ls"}, ghosttyDeps(&stderr)); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !e.called || e.program != "ls" {
		t.Fatalf("expected in-place exec of ls, got %+v", e)
	}
	want := "gx: Ghostty: set macos-applescript = true in your Ghostty config — running in place.\n"
	if stderr.String() != want {
		t.Fatalf("stderr = %q, want %q", stderr.String(), want)
	}
}

func TestRunTerm_GhosttyPlainErrorDoesNotExecReplace(t *testing.T) {
	// not parallel: shares package-level launchInSplit/execReplace seam vars with other TestRunTerm_* tests via withSeams
	_, e := withSeams(t)
	launchInSplit = func(string, ui.Terminal, terminalrun.SplitType, string, []string, ...terminalrun.LaunchOption) (string, error) {
		return "", errors.New("boom")
	}
	var stderr bytes.Buffer

	err := execute([]string{"term", "--tab", "ls"}, ghosttyDeps(&stderr))
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v, want boom", err)
	}
	if e.called {
		t.Fatal("plain error must not exec-replace")
	}
	if strings.Contains(stderr.String(), "running in place") {
		t.Fatalf("unexpected hint on stderr: %q", stderr.String())
	}
}

func TestRunTerm_GhosttySuccessNeitherHintNorExec(t *testing.T) {
	// not parallel: shares package-level launchInSplit/execReplace seam vars with other TestRunTerm_* tests via withSeams
	l, e := withSeams(t)
	var stderr bytes.Buffer

	if err := execute([]string{"term", "--below"}, ghosttyDeps(&stderr)); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !l.called || l.terminal != ui.TerminalGhostty {
		t.Fatalf("expected ghostty launch, got %+v", l)
	}
	if e.called || stderr.Len() != 0 {
		t.Fatalf("expected no exec/stderr, got exec=%v stderr=%q", e.called, stderr.String())
	}
}

func TestTermHelpNamesAllBackends(t *testing.T) {
	t.Parallel()
	c := newTermCmd(deps{})
	for _, s := range []string{c.Short, c.Long} {
		for _, b := range []string{"tmux", "kitty", "herdr", "ghostty"} {
			if !strings.Contains(strings.ToLower(s), b) {
				t.Errorf("help %q missing %q", s, b)
			}
		}
	}
	if !strings.Contains(c.Long, "remote control") || !strings.Contains(c.Long, "1.3.0") {
		t.Errorf("long help missing kitty remote control / ghostty version note")
	}
}
