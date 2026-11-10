package pr_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/elentok/gx/git/pr"
)

func TestCreateDraft_ArgsAndParse(t *testing.T) {
	t.Parallel()
	r, log := fakeGH(t, t.TempDir(), "Creating draft pull request\nhttps://github.com/o/r/pull/42", "", 0)
	n, url, err := r.CreateDraft("o/r", "feat", "main", "Title", "/tmp/body.md")
	if err != nil {
		t.Fatal(err)
	}
	if n != 42 || url != "https://github.com/o/r/pull/42" {
		t.Fatalf("got %d %q", n, url)
	}
	want := "pr create --draft --head feat --base main --title Title --body-file /tmp/body.md -R o/r\n"
	if got := readLog(t, log); got != want {
		t.Fatalf("argv = %q, want %q", got, want)
	}
}

func TestCreateDraft_NoURL(t *testing.T) {
	t.Parallel()
	r, _ := fakeGH(t, t.TempDir(), "hello", "", 0)
	if _, _, err := r.CreateDraft("o/r", "b", "main", "t", "f"); err == nil {
		t.Fatal("want error")
	}
}

func TestWriteCalls_FailureClassified(t *testing.T) {
	t.Parallel()
	calls := map[string]func(pr.Runner) error{
		"create": func(r pr.Runner) error { _, _, err := r.CreateDraft("o/r", "b", "main", "t", "f"); return err },
		"ready":  func(r pr.Runner) error { return r.MarkReady("o/r", 1) },
		"rerun":  func(r pr.Runner) error { return r.RerunJob("o/r", 1) },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r, _ := fakeGH(t, t.TempDir(), "", "gh: Forbidden (HTTP 403)", 1)
			var le *pr.PRListError
			if err := call(r); !errors.As(err, &le) || le.Kind != pr.PRListErrorForbidden {
				t.Fatalf("err = %v, want forbidden", err)
			}
		})
	}
}

func TestMarkReadyAndRerun_Args(t *testing.T) {
	t.Parallel()
	r, log := fakeGH(t, t.TempDir(), "", "", 0)
	if err := r.MarkReady("o/r", 7); err != nil {
		t.Fatal(err)
	}
	if err := r.RerunJob("o/r", 99); err != nil {
		t.Fatal(err)
	}
	want := "pr ready 7 -R o/r\nrun rerun --job 99 -R o/r\n"
	if got := readLog(t, log); got != want {
		t.Fatalf("argv = %q, want %q", got, want)
	}
}

func TestTrimBody(t *testing.T) {
	t.Parallel()
	if got := pr.TrimBody("short"); got != "short" {
		t.Fatalf("got %q", got)
	}
	long := strings.Repeat("line of text\n", 10000)
	got := pr.TrimBody(long)
	if len(got) >= pr.MaxBodyChars {
		t.Fatalf("len = %d", len(got))
	}
	if !strings.Contains(got, "trimmed") || !strings.HasPrefix(got, "line of text\nline of text") {
		t.Fatalf("unexpected result tail: %q", got[len(got)-80:])
	}
	if !strings.HasSuffix(got[:strings.Index(got, "\n\n_(")], "\nline of text") {
		t.Fatal("cut mid-line")
	}
}
