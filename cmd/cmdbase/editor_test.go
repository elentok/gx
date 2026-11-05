package cmdbase_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/cmd/cmdtest"
)

func TestRunEditorCommand_Success(t *testing.T) {
	t.Parallel()
	path := t.TempDir() + "/file.txt"
	err := cmdbase.RunEditorCommand(cmdtest.TempEnv(t), "touch", path, bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	if err != nil {
		t.Fatalf("RunEditorCommand touch: %v", err)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatal("expected file to be created by touch")
	}
}

func TestRunEditorCommand_Failure(t *testing.T) {
	t.Parallel()
	err := cmdbase.RunEditorCommand(cmdtest.TempEnv(t), "false", "/irrelevant", bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	if err == nil {
		t.Fatal("expected error from false command")
	}
}

func TestRunEditorCommand_MultiWordEditor(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	// Verify a multi-word $EDITOR (e.g. "code --wait") gets split correctly.
	err := cmdbase.RunEditorCommand(cmdtest.TempEnv(t), "echo hello", "/dev/null", bytes.NewBuffer(nil), &stdout, bytes.NewBuffer(nil))
	if err != nil {
		t.Fatalf("RunEditorCommand multi-word: %v", err)
	}
	if !strings.Contains(stdout.String(), "hello") {
		t.Fatalf("expected hello in output, got: %q", stdout.String())
	}
}

func TestRunEditorCommand_ChildGetsTheGivenEnv(t *testing.T) {
	t.Setenv("EDITOR_LEAK", "leaked")
	var stdout bytes.Buffer
	env := append(cmdtest.TempEnv(t), "EDITOR_PROBE=given")
	err := cmdbase.RunEditorCommand(env, "printenv", "EDITOR_PROBE", bytes.NewBuffer(nil), &stdout, bytes.NewBuffer(nil))
	if err != nil {
		t.Fatalf("printenv EDITOR_PROBE: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "given" {
		t.Fatalf("child env output = %q, want only the given EDITOR_PROBE", got)
	}
	stdout.Reset()
	err = cmdbase.RunEditorCommand(env, "printenv", "EDITOR_LEAK", bytes.NewBuffer(nil), &stdout, bytes.NewBuffer(nil))
	if err == nil {
		t.Fatal("printenv should exit 1: EDITOR_LEAK is not in the given env")
	}
	if stdout.Len() != 0 {
		t.Fatalf("child inherited EDITOR_LEAK: %q", stdout.String())
	}
}
