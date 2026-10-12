package osnotify

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"testing"
)

type call struct {
	name string
	args []string
}

type fakeRunner struct {
	calls []call
	err   error
}

func (f *fakeRunner) run(_ context.Context, name string, args ...string) error {
	f.calls = append(f.calls, call{name, args})
	return f.err
}

const quotedBody = `bad value "x" at line 3`

func TestNewSystemNotifier_NoopInTests(t *testing.T) {
	if _, ok := NewSystemNotifier().(noop); !ok {
		t.Fatalf("NewSystemNotifier() in a test binary = %T, want noop", NewSystemNotifier())
	}
}

func TestTerminalNotifier_Argv(t *testing.T) {
	r := &fakeRunner{}
	if err := newTerminalNotifier("/opt/bin/terminal-notifier", r.run).Notify(context.Background(), "gx title", quotedBody); err != nil {
		t.Fatal(err)
	}
	want := []call{{"/opt/bin/terminal-notifier", []string{"-title", "gx title", "-message", quotedBody, "-group", "gx-config"}}}
	if !reflect.DeepEqual(r.calls, want) {
		t.Fatalf("calls = %#v, want %#v", r.calls, want)
	}
}

func TestOSAScript_Argv(t *testing.T) {
	r := &fakeRunner{}
	if err := newOSAScript(r.run).Notify(context.Background(), "gx title", quotedBody); err != nil {
		t.Fatal(err)
	}
	want := []call{{"osascript", []string{"-e", osascriptSource, "gx title", quotedBody}}}
	if !reflect.DeepEqual(r.calls, want) {
		t.Fatalf("calls = %#v, want %#v", r.calls, want)
	}
}

func TestNotify_ReturnsRunnerError(t *testing.T) {
	r := &fakeRunner{err: errors.New("boom")}
	if err := newTerminalNotifier("terminal-notifier", r.run).Notify(context.Background(), "t", "b"); err == nil {
		t.Fatal("want error, got nil")
	}
	if len(r.calls) != 1 {
		t.Fatalf("calls = %d, want 1 (no osascript fallback)", len(r.calls))
	}
}

func TestNoop_RunsNothing(t *testing.T) {
	if err := (noop{}).Notify(context.Background(), "t", "b"); err != nil {
		t.Fatal(err)
	}
}

func TestSelect(t *testing.T) {
	found := func(string) (string, error) { return "/opt/bin/terminal-notifier", nil }
	missing := func(string) (string, error) { return "", exec.ErrNotFound }

	tests := []struct {
		name     string
		goos     string
		lookPath func(string) (string, error)
		want     Notifier
	}{
		{"darwin with terminal-notifier", "darwin", found, terminalNotifier{}},
		{"darwin without terminal-notifier", "darwin", missing, osascript{}},
		{"linux with terminal-notifier", "linux", found, noop{}},
		{"linux without terminal-notifier", "linux", missing, noop{}},
		{"windows", "windows", missing, noop{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Select(tt.goos, tt.lookPath, (&fakeRunner{}).run)
			if reflect.TypeOf(got) != reflect.TypeOf(tt.want) {
				t.Fatalf("Select = %T, want %T", got, tt.want)
			}
		})
	}
}

func TestSelect_TerminalNotifierUsesResolvedPath(t *testing.T) {
	r := &fakeRunner{}
	found := func(string) (string, error) { return "/opt/bin/terminal-notifier", nil }
	if err := Select("darwin", found, r.run).Notify(context.Background(), "t", "b"); err != nil {
		t.Fatal(err)
	}
	if r.calls[0].name != "/opt/bin/terminal-notifier" {
		t.Fatalf("ran %q, want resolved path", r.calls[0].name)
	}
}
