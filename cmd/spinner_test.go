package cmd

import (
	"bytes"
	"errors"
	"os"
	"testing"
)

func TestRunWithSpinner_NonTTYRunsFunction(t *testing.T) {
	var called bool
	err := runWithSpinner(bytes.NewBuffer(nil), bytes.NewBuffer(nil), "working", func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("runWithSpinner: %v", err)
	}
	if !called {
		t.Fatal("expected wrapped function to run")
	}
}

func TestRunWithSpinner_NonTTYPropagatesError(t *testing.T) {
	want := errors.New("boom")
	err := runWithSpinner(bytes.NewBuffer(nil), bytes.NewBuffer(nil), "working", func() error {
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("expected %v, got %v", want, err)
	}
}

func TestIsTerminalWriter_Buffer(t *testing.T) {
	if isTerminalWriter(bytes.NewBuffer(nil)) {
		t.Fatal("bytes.Buffer should not be a terminal writer")
	}
}

func TestIsTerminalWriter_Stderr(t *testing.T) {
	// os.Stderr is an *os.File but may not be a TTY in CI — just verify it doesn't panic.
	_ = isTerminalWriter(os.Stderr)
}
