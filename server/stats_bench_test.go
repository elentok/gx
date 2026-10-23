package server

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/tickets"
)

const (
	benchEpics           = 260
	benchTicketsPerEpic  = 100
	statsBuildBudget     = time.Second
	benchStampDaysSpread = 90
)

func writeBenchStore(b *testing.B, store string) {
	b.Helper()
	proj := filepath.Join(store, "proj")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "project.json"), []byte(`{"name":"proj"}`), 0o644); err != nil {
		b.Fatal(err)
	}
	now := time.Now()
	aged := now.Add(-time.Hour)
	for e := range benchEpics {
		dir := filepath.Join(proj, fmt.Sprintf("epic-%03d", e), "issues")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "..", "ticket.md"), []byte("---\nstatus: open\n---\n"), 0o644); err != nil {
			b.Fatal(err)
		}
		for i := range benchTicketsPerEpic {
			done := now.AddDate(0, 0, -(e+i)%benchStampDaysSpread).UTC().Format(time.RFC3339)
			md := fmt.Sprintf("---\nid: \"%02d\"\nstatus: done\ntype: implement\ndone_at: %s\n---\n\n# Ticket %d\n\nBody text.\n", i, done, i)
			path := filepath.Join(dir, fmt.Sprintf("%02d-t.md", i))
			if err := os.WriteFile(path, []byte(md), 0o644); err != nil {
				b.Fatal(err)
			}
			// The ticket cache skips files modified moments ago, so age them.
			if err := os.Chtimes(path, aged, aged); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func statsBuildOnce(b *testing.B, store string) time.Duration {
	b.Helper()
	start := time.Now()
	all, err := tickets.LoadStoreEpics(store)
	if err != nil {
		b.Fatal(err)
	}
	buildStats(all, 90, "", time.Now())
	return time.Since(start)
}

// BenchmarkStats_Build loads and aggregates a store of about 26,000 tickets,
// the work one /v1/stats request does. The first request after a server start
// parses every file and is reported as cold-ms; later requests hit the ticket
// cache and must stay inside the budget.
func BenchmarkStats_Build(b *testing.B) {
	store := b.TempDir()
	writeBenchStore(b, store)
	cold := statsBuildOnce(b, store)
	if cold > statsBuildBudget {
		b.Errorf("cold stats build took %v, budget %v", cold, statsBuildBudget)
	}
	b.ResetTimer()
	b.ReportMetric(float64(cold.Milliseconds()), "cold-ms")
	for range b.N {
		if took := statsBuildOnce(b, store); took > statsBuildBudget {
			b.Errorf("stats build took %v, budget %v", took, statsBuildBudget)
		}
	}
}
