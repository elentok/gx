package ralphloop

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/storecommit"
)

// Seam B: a loop park leaves a commit in the store with the address-and-status
// message.
func TestPark_CommitsStoreWithAddressAndStatus(t *testing.T) {
	store := t.TempDir()
	loop, err := storecommit.Start(store, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Stop()

	issues := filepath.Join(store, "myepic", "issues")
	if err := os.MkdirAll(issues, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(issues, "01-some-ticket.md")
	if err := os.WriteFile(path, []byte("---\nid: \"01\"\nstatus: claimed\ntype: implement\n---\n# T\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := park(noopEventSink{}, parkRequest{
		ScratchDir: store, EpicName: "myepic", Ticket: "01", Path: path,
		Type: events.NeedsAnswer, Kind: events.Kind("ticket-answered"), Reason: "q",
	}); err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command("git", "-C", store, "log", "--format=%s").CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v: %s", err, out)
	}
	if got := strings.TrimSpace(string(out)); got != "myepic/01: needs-answer" {
		t.Errorf("log = %q, want %q", got, "myepic/01: needs-answer")
	}
}
