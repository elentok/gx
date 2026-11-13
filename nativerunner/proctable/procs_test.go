package proctable_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/nativerunner/proctable"
)

func TestPsTable_LookupSplitsStartAndCommand(t *testing.T) {
	proc, ok, err := proctable.PS.Lookup(os.Getpid())
	if err != nil || !ok {
		t.Fatalf("Lookup(self) = %v, %v, %v", proc, ok, err)
	}
	if _, err := time.Parse("Mon Jan _2 15:04:05 2006", proc.Start); err != nil {
		t.Errorf("Start = %q, want ps lstart: %v", proc.Start, err)
	}
	if !strings.Contains(proc.Cmdline, "proctable") {
		t.Errorf("Cmdline = %q, want the test binary's command line", proc.Cmdline)
	}
}
