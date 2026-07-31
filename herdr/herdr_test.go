package herdr

import (
	"errors"
	"strings"
	"testing"
)

// withFakeCommand swaps runCommand for fn for the duration of the test.
func withFakeCommand(t *testing.T, fn func(args ...string) ([]byte, error)) {
	t.Helper()
	prev := runCommand
	runCommand = fn
	t.Cleanup(func() { runCommand = prev })
}

func TestRun_Success_ReturnsOutput(t *testing.T) {
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		return []byte("ok"), nil
	})
	out, err := run("workspace", "list")
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if string(out) != "ok" {
		t.Fatalf("run() = %q, want %q", out, "ok")
	}
}

func TestRun_Failure_WrapsCommandAndOutput(t *testing.T) {
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		return []byte("boom"), errors.New("exit 1")
	})
	_, err := run("workspace", "list")
	if err == nil {
		t.Fatal("run() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "$ herdr workspace list") {
		t.Errorf("error missing command line: %v", err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error missing output: %v", err)
	}
	if !strings.Contains(err.Error(), "exit 1") {
		t.Errorf("error missing underlying error: %v", err)
	}
}

func TestRunJSON_ParsesResultField(t *testing.T) {
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		return []byte(`{"id":"cli:x","result":{"count":3}}`), nil
	})
	var result struct {
		Count int `json:"count"`
	}
	if err := runJSON([]string{"x"}, &result); err != nil {
		t.Fatalf("runJSON() error = %v", err)
	}
	if result.Count != 3 {
		t.Fatalf("result.Count = %d, want 3", result.Count)
	}
}

func TestRunJSON_PropagatesCommandError(t *testing.T) {
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		return []byte(`{"error":{"code":"nope"}}`), errors.New("exit 1")
	})
	var result struct{}
	if err := runJSON([]string{"x"}, &result); err == nil {
		t.Fatal("runJSON() error = nil, want error")
	}
}

func TestRunJSON_MalformedOutput_ReturnsError(t *testing.T) {
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		return []byte("not json"), nil
	})
	var result struct{}
	if err := runJSON([]string{"x"}, &result); err == nil {
		t.Fatal("runJSON() error = nil, want error")
	}
}

func TestAppendFlag(t *testing.T) {
	got := appendFlag([]string{"a"}, "--flag", "")
	if len(got) != 1 {
		t.Fatalf("appendFlag with empty value = %v, want unchanged", got)
	}
	got = appendFlag([]string{"a"}, "--flag", "v")
	want := []string{"a", "--flag", "v"}
	if len(got) != len(want) || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("appendFlag = %v, want %v", got, want)
	}
}
