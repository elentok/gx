package brief

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGoTestParse_TimeoutNamesTheStuckTestWithProjectFrames(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("testdata", "gotest-timeout.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	got := GoTest{}.Parse(data, 1)
	if len(got.Failures) != 1 {
		t.Fatalf("failures = %+v, want one", got.Failures)
	}
	u := got.Failures[0]
	if u.Kind != KindTest || u.Name != "TestStuck" || u.Location != "github.com/elentok/gx/server" {
		t.Errorf("unit = %+v, want the stuck test", u)
	}
	for _, want := range []string{
		"panic: test timed out after 1s",
		"/repo/server/stuck_test.go:20 github.com/elentok/gx/server.TestStuck",
		"/repo/server/wait.go:12 github.com/elentok/gx/server.waitForever",
	} {
		if !strings.Contains(u.Text, want) {
			t.Errorf("text lacks %q:\n%s", want, u.Text)
		}
	}
	for _, drop := range []string{"testing.go", "runtime", "proc.go"} {
		if strings.Contains(u.Text, drop) {
			t.Errorf("text keeps non-project frame %q:\n%s", drop, u.Text)
		}
	}
}
