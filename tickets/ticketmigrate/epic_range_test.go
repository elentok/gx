package ticketmigrate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/tickets"
)

func writeRangeTicketMD(t *testing.T, dir, name, kind, extra string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nkind: " + kind + "\nstatus: open\ncreated_at: 2026-01-02T03:04:05Z\nmerged_at: 2026-02-02T03:04:05Z\n" + extra + "---\n# E\n"
	if err := os.WriteFile(filepath.Join(p, "ticket.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestBackfillEpicRange_WalksLoopEpicsLiveAndArchivedOnly(t *testing.T) {
	root := t.TempDir()
	live := writeRangeTicketMD(t, root, "live", "loop", "")
	writeRangeTicketMD(t, root, "themap", "map", "")
	ranged := writeRangeTicketMD(t, root, "ranged", "loop", "range_base: aaa\nrange_tip: bbb\n")
	refused := writeRangeTicketMD(t, root, "refused", "loop", "")
	archived := writeRangeTicketMD(t, tickets.ArchiveDir(root), "old", "loop", "")

	var asked []string
	changes, err := BackfillEpicRange(root, func(e tickets.Epic) (string, string, bool, error) {
		asked = append(asked, e.Name)
		return "base-" + e.Name, "tip-" + e.Name, e.Name != "refused", nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(asked) != 3 {
		t.Errorf("asked = %v, want live, refused and old only", asked)
	}
	if len(changes) != 2 {
		t.Errorf("changes = %v, want live and old", changes)
	}
	for _, name := range []string{live, archived} {
		e := tickets.LoadEpicTicketMD(name)
		base := filepath.Base(name)
		if e.RangeBase != "base-"+base || e.RangeTip != "tip-"+base {
			t.Errorf("%s range = %s..%s", base, e.RangeBase, e.RangeTip)
		}
		if e.MergedAt.IsZero() {
			t.Errorf("%s lost merged_at", base)
		}
	}
	if e := tickets.LoadEpicTicketMD(refused); e.RangeBase != "" {
		t.Errorf("refused stamped %s", e.RangeBase)
	}
	if e := tickets.LoadEpicTicketMD(ranged); e.RangeBase != "aaa" || e.RangeTip != "bbb" {
		t.Errorf("existing range became %s..%s", e.RangeBase, e.RangeTip)
	}
}

func TestBackfillEpicRange_NoMergedEpicNeverAsks(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "open")
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "ticket.md"), []byte("---\nkind: loop\nstatus: open\n---\n# E\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := BackfillEpicRange(root, func(tickets.Epic) (string, string, bool, error) {
		t.Error("asked about an unmerged epic")
		return "", "", false, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
