package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/repair"
	"github.com/elentok/gx/testutil"
)

// Every command that names a ticket in human output prints its canonical
// address, never the bare id or the file path.

func TestRunTicketsAdd_PrintsAddressNotPath(t *testing.T) {
	t.Parallel()
	epicPath := filepath.Join(t.TempDir(), "proj", "widget-epic")
	issuesDir := filepath.Join(epicPath, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatal(err)
	}
	testutil.EnsureEpicTicketMD(t, epicPath)
	if err := os.WriteFile(filepath.Join(issuesDir, "01-a.md"), []byte("---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := runTicketsAdd(epicPath, "", "do-thing", &stdout); err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(stdout.String())
	if want := repair.EpicTicketLabel(epicPath, "02"); got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
	if strings.Contains(got, string(filepath.Separator)+"issues") {
		t.Errorf("output %q looks like a path", got)
	}
}
