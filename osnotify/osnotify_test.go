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

var fullNote = Notification{
	Title:    "gx title",
	Body:     quotedBody,
	Group:    "gx-agent-a",
	Execute:  "gx agent focus a",
	Activate: "net.kovidgoyal.kitty",
}

func TestTerminalNotifier_Argv(t *testing.T) {
	tests := []struct {
		name string
		note Notification
		want []string
	}{
		{"all fields", fullNote, []string{
			"-title", "gx title", "-message", quotedBody,
			"-group", "gx-agent-a", "-execute", "gx agent focus a", "-activate", "net.kovidgoyal.kitty",
		}},
		{"empty optional fields", Notification{Title: "gx title", Body: quotedBody}, []string{
			"-title", "gx title", "-message", quotedBody,
		}},
		{"link opens on click", Notification{Title: "gx title", Body: quotedBody, Link: "http://x/#/d/p:e"}, []string{
			"-title", "gx title", "-message", quotedBody, "-open", "http://x/#/d/p:e",
		}},
		{"execute wins over the link", Notification{Title: "gx title", Body: quotedBody, Execute: "gx agent focus a", Link: "http://x/#/d/p:e"}, []string{
			"-title", "gx title", "-message", quotedBody, "-execute", "gx agent focus a",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &fakeRunner{}
			found := func(string) (string, error) { return "/opt/bin/terminal-notifier", nil }
			if err := Select("darwin", found, r.run).Notify(context.Background(), tt.note); err != nil {
				t.Fatal(err)
			}
			want := []call{{"/opt/bin/terminal-notifier", tt.want}}
			if !reflect.DeepEqual(r.calls, want) {
				t.Fatalf("calls = %#v, want %#v", r.calls, want)
			}
		})
	}
}

func TestOSAScript_Argv(t *testing.T) {
	r := &fakeRunner{}
	missing := func(string) (string, error) { return "", exec.ErrNotFound }
	if err := Select("darwin", missing, r.run).Notify(context.Background(), fullNote); err != nil {
		t.Fatal(err)
	}
	want := []call{{"osascript", []string{"-e", osascriptSource, "gx title", quotedBody}}}
	if !reflect.DeepEqual(r.calls, want) {
		t.Fatalf("calls = %#v, want %#v", r.calls, want)
	}
}

func TestOSAScript_PutsTheLinkInTheBody(t *testing.T) {
	r := &fakeRunner{}
	missing := func(string) (string, error) { return "", exec.ErrNotFound }
	note := Notification{Title: "gx title", Body: "Done", Link: "http://x/#/d/p:e"}
	if err := Select("darwin", missing, r.run).Notify(context.Background(), note); err != nil {
		t.Fatal(err)
	}
	want := []call{{"osascript", []string{"-e", osascriptSource, "gx title", "Done\nhttp://x/#/d/p:e"}}}
	if !reflect.DeepEqual(r.calls, want) {
		t.Fatalf("calls = %#v, want %#v", r.calls, want)
	}
}

func TestNotifySend_Argv(t *testing.T) {
	tests := []struct {
		name string
		note Notification
		want []string
	}{
		{"title and body", fullNote, []string{"--", "gx title", quotedBody}},
		{"link goes in the body", Notification{Title: "gx title", Body: "Done", Link: "http://x/#/d/p:e"}, []string{
			"--", "gx title", "Done\nhttp://x/#/d/p:e",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &fakeRunner{}
			found := func(string) (string, error) { return "/usr/bin/notify-send", nil }
			if err := Select("linux", found, r.run).Notify(context.Background(), tt.note); err != nil {
				t.Fatal(err)
			}
			want := []call{{"/usr/bin/notify-send", tt.want}}
			if !reflect.DeepEqual(r.calls, want) {
				t.Fatalf("calls = %#v, want %#v", r.calls, want)
			}
		})
	}
}

func TestNotify_ReturnsRunnerError(t *testing.T) {
	r := &fakeRunner{err: errors.New("boom")}
	if err := newTerminalNotifier("terminal-notifier", r.run).Notify(context.Background(), Notification{Title: "t", Body: "b"}); err == nil {
		t.Fatal("want error, got nil")
	}
	if len(r.calls) != 1 {
		t.Fatalf("calls = %d, want 1 (no osascript fallback)", len(r.calls))
	}
}

func TestNoop_RunsNothing(t *testing.T) {
	if err := (noop{}).Notify(context.Background(), Notification{Title: "t", Body: "b"}); err != nil {
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
		{"linux with notify-send", "linux", found, notifySend{}},
		{"linux without notify-send", "linux", missing, noop{}},
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
	if err := Select("darwin", found, r.run).Notify(context.Background(), Notification{Title: "t", Body: "b"}); err != nil {
		t.Fatal(err)
	}
	if r.calls[0].name != "/opt/bin/terminal-notifier" {
		t.Fatalf("ran %q, want resolved path", r.calls[0].name)
	}
}
