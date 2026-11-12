package fixturetest

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"testing"
)

func TestExplainStartErr_NamesTheProcessLimit(t *testing.T) {
	err := explainStartErr(&os.PathError{Op: "fork/exec", Path: "gx", Err: syscall.EAGAIN})
	if err == nil || !strings.Contains(err.Error(), "process limit hit") {
		t.Fatalf("err = %v, want the process-limit message", err)
	}
	other := fmt.Errorf("boom")
	if got := explainStartErr(other); got != other {
		t.Errorf("a non-EAGAIN error was rewritten: %v", got)
	}
	if explainStartErr(nil) != nil {
		t.Error("nil became an error")
	}
}
