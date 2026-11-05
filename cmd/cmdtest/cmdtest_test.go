package cmdtest

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/projectlock"
)

var envAcrossTests = map[string][]string{}

func TestEnvFor_SharedAcrossSubtests(t *testing.T) {
	top := EnvFor(t)
	t.Run("a", func(t *testing.T) {
		if got := EnvValue(EnvFor(t), "HOME"); got != EnvValue(top, "HOME") {
			t.Fatalf("subtest HOME = %q, want %q", got, EnvValue(top, "HOME"))
		}
	})
	SetEnvFor(t, "FOO", "bar")
	t.Run("b", func(t *testing.T) {
		if got := EnvValue(EnvFor(t), "FOO"); got != "bar" {
			t.Fatalf("subtest FOO = %q, want bar", got)
		}
	})
}

func TestEnvFor_SeparateAcrossTestsOne(t *testing.T) {
	envAcrossTests[t.Name()] = EnvFor(t)
}

func TestEnvFor_SeparateAcrossTestsTwo(t *testing.T) {
	one, ok := envAcrossTests["TestEnvFor_SeparateAcrossTestsOne"]
	if !ok {
		t.Skip("first test did not run")
	}
	if EnvValue(EnvFor(t), "HOME") == EnvValue(one, "HOME") {
		t.Fatal("two tests share one HOME")
	}
}

func TestPrependPath(t *testing.T) {
	PrependPath(t, "/x/bin")
	if got := EnvValue(EnvFor(t), "PATH"); !strings.HasPrefix(got, "/x/bin"+string(os.PathListSeparator)) {
		t.Fatalf("PATH = %q, want /x/bin first", got)
	}
}

func TestRun_ArgsOutputAndSilencedErrors(t *testing.T) {
	var out, errOut bytes.Buffer
	env := cmdbase.Env{Stdout: &out, Stderr: &errOut}
	var gotArgs []string
	boom := errors.New("boom")
	cmd := &cobra.Command{
		Use: "say",
		RunE: func(c *cobra.Command, args []string) error {
			gotArgs = args
			c.Println("to-out")
			c.PrintErrln("to-err")
			return boom
		},
	}

	err := Run(t, env, []string{"say", "x", "y"}, cmd)

	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if strings.Join(gotArgs, ",") != "x,y" {
		t.Fatalf("args = %v", gotArgs)
	}
	if out.String() != "to-out\n" {
		t.Fatalf("stdout = %q", out.String())
	}
	// Errors and usage are silenced: only the command's own stderr line shows.
	if errOut.String() != "to-err\n" {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestRun_NilArgsDoNotFallBackToOSArgs(t *testing.T) {
	ran := false
	cmd := &cobra.Command{Use: "x", Run: func(*cobra.Command, []string) { ran = true }}
	if err := Run(t, cmdbase.Env{}, nil, cmd); err != nil {
		t.Fatal(err)
	}
	if ran {
		t.Fatal("subcommand ran with nil args")
	}
}

func TestSetWaitHook_NoticeWhileLockHeldThenRestored(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	t.Setenv("HOME", state)
	project := filepath.Join(t.TempDir(), "proj")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "project.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	ticket := filepath.Join(project, "epic", "issues", "01-a.md")

	// A buffer written from the lock-waiting goroutine; guarded by the channel.
	notice := make(chan string, 1)
	pr, pw := newChanWriter(notice)
	t.Run("hook", func(t *testing.T) {
		SetWaitHook(t, pw, 10*time.Millisecond)
		unlock, err := projectlock.Exclusive(project)
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan struct{})
		go func() {
			defer close(done)
			u, err := projectlock.Shared(ticket)
			if err == nil {
				u()
			}
		}()
		select {
		case got := <-pr:
			if got != "waiting for the migration of proj\n" {
				t.Fatalf("notice = %q", got)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("no wait notice")
		}
		unlock()
		<-done
	})

	// After cleanup the hook is gone: a second wait prints nothing. The delay is
	// shortened here so the wait would fire the hook if it were still set.
	t.Cleanup(projectlock.SetWaitHookDelayForTest(10 * time.Millisecond))
	unlock, err := projectlock.Exclusive(project)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		if u, err := projectlock.Shared(ticket); err == nil {
			u()
		}
	}()
	time.Sleep(200 * time.Millisecond)
	unlock()
	<-done
	select {
	case got := <-pr:
		t.Fatalf("hook not restored, got notice %q", got)
	default:
	}
}

// chanWriter forwards each Write to a channel.
type chanWriter struct{ ch chan string }

func newChanWriter(ch chan string) (<-chan string, *chanWriter) {
	return ch, &chanWriter{ch: ch}
}

func (w *chanWriter) Write(p []byte) (int, error) {
	select {
	case w.ch <- string(p):
	default:
	}
	return len(p), nil
}
