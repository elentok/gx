package tickets

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
)

// writeFrontmatterTicket writes a ticket file with real frontmatter fields
// (id/status/parent), unlike writeTicket's LegacyTicketToFrontmatter
// conversion which doesn't carry parent.
func writeFrontmatterTicket(t *testing.T, root, epic, filename, id, status, parent string) {
	t.Helper()
	path := filepath.Join(root, ".scratch", epic, "issues", filename)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	testutil.EnsureEpicTicketMD(t, filepath.Dir(filepath.Dir(path)))
	var b strings.Builder
	fmt.Fprintf(&b, "---\nid: %q\nstatus: %s\ntype: implement\n", id, status)
	if parent != "" {
		fmt.Fprintf(&b, "parent: %q\n", parent)
	}
	b.WriteString("---\nBody.\n")
	if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
		t.Fatal(err)
	}
}
