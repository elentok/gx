package stats

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/server/limits"
	"github.com/elentok/gx/tickets/ticketmigrate"
	"github.com/elentok/gx/webperf"
)

const (
	benchEpics           = 260
	benchTicketsPerEpic  = 100
	statsBuildBudget     = time.Second
	benchStampDaysSpread = 90
	benchRecords         = 2000
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
	all, err := ticketmigrate.LoadStoreEpics(store)
	if err != nil {
		b.Fatal(err)
	}
	buildStats(statsInput{Epics: all, Days: 90, Now: time.Now()})
	return time.Since(start)
}

// BenchmarkStats_WebPerformance reads the web performance section at cap-sized
// data: 20,000 samples a day for 90 days. The first read parses every day file
// and is reported as cold-ms; later reads hit the day cache.
func BenchmarkStats_WebPerformance(b *testing.B) {
	now := time.Now()
	store, err := webperf.Open(b.TempDir(), time.Now)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(store.Close)
	metrics := []string{"lcp", "inp", "cls", "fcp", "app_ready", "route_ready"}
	routes := []string{"epics", "epic", "agents", "stats", "tips", "ticket"}
	for d := range benchStampDaysSpread {
		at := now.AddDate(0, 0, -d)
		samples := make([]webperf.Sample, webperf.MaxPerDay)
		for i := range samples {
			samples[i] = webperf.Sample{
				ID: fmt.Sprintf("%d", i), Session: fmt.Sprintf("s-%d", i/webperf.MaxPerSessionPerDay),
				Metric: metrics[i%len(metrics)], Route: routes[i%len(routes)], Tab: "hard", Value: float64(i%4000 + 1), Env: "prod",
			}
		}
		if _, _, err := store.Append(at, samples); err != nil {
			b.Fatal(err)
		}
	}
	read := func() time.Duration {
		start := time.Now()
		if _, err := buildWebPerformance(store, limits.NewStatsWindow(90, now), true); err != nil {
			b.Fatal(err)
		}
		return time.Since(start)
	}
	// Seeding leaves a large heap behind; collect it so cold-ms is the read alone.
	runtime.GC()
	cold := read()
	if cold > statsBuildBudget {
		b.Errorf("cold web performance read took %v, budget %v", cold, statsBuildBudget)
	}
	b.ResetTimer()
	b.ReportMetric(float64(cold.Milliseconds()), "cold-ms")
	for range b.N {
		if took := read(); took > statsBuildBudget {
			b.Errorf("web performance read took %v, budget %v", took, statsBuildBudget)
		}
	}
}

// BenchmarkStats_Grilling reads and sums 2,000 conversation records, the
// Grilling work of one /v1/stats request.
func BenchmarkStats_Grilling(b *testing.B) {
	root := b.TempDir()
	now := time.Now()
	for i := range benchRecords {
		items := make([]conversations.GrillingItem, 5)
		for j := range items {
			at := now.AddDate(0, 0, -(i+j)%benchStampDaysSpread)
			items[j] = conversations.GrillingItem{MessageID: fmt.Sprintf("m-%d-%d", i, j), At: at, Questions: 3, Words: 120}
		}
		rec := conversations.Record{ID: fmt.Sprintf("c-%04d", i), Ticket: "proj:design/01", Design: true, RecordedAt: now, Grilling: items}
		if err := conversations.Save(root, rec); err != nil {
			b.Fatal(err)
		}
	}
	b.ResetTimer()
	for range b.N {
		start := time.Now()
		recs, err := conversations.List(root)
		if err != nil {
			b.Fatal(err)
		}
		buildGrilling(recs, func(string) bool { return true }, limits.NewStatsWindow(90, now), "")
		if took := time.Since(start); took > statsBuildBudget {
			b.Errorf("grilling build took %v, budget %v", took, statsBuildBudget)
		}
	}
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
