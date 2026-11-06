package index

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server/broker"
)

const benchTicketCount = 3000

// BenchmarkRefresh measures a warm refresh (parse cache populated, no file
// changed) on a store of about 3000 tickets.
func BenchmarkRefresh(b *testing.B) {
	store := b.TempDir()
	const epics = 30
	// The parse cache distrusts files modified moments ago, so age them.
	old := time.Now().Add(-time.Hour)
	for e := range epics {
		dir := filepath.Join(store, "proj", fmt.Sprintf("epic-%02d", e), "issues")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			b.Fatal(err)
		}
		for n := range benchTicketCount / epics {
			body := fmt.Sprintf("---\nid: \"%02d\"\nstatus: open\ntype: implement\n---\nBody of ticket %d.\n", n, n)
			path := filepath.Join(dir, fmt.Sprintf("%02d-ticket.md", n))
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				b.Fatal(err)
			}
			if err := os.Chtimes(path, old, old); err != nil {
				b.Fatal(err)
			}
		}
	}
	if err := os.WriteFile(filepath.Join(store, "proj", "project.json"), []byte("{}"), 0o644); err != nil {
		b.Fatal(err)
	}
	idx := New(store, broker.New(0), nil)
	if err := idx.Refresh(); err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		if err := idx.Refresh(); err != nil {
			b.Fatal(err)
		}
	}
}
