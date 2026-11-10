package prflow

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

var t0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func at(m int) time.Time { return t0.Add(time.Duration(m) * time.Minute) }

func mustAppend(t *testing.T, dir string, e Entry) {
	t.Helper()
	if err := Append(dir, e); err != nil {
		t.Fatal(err)
	}
}

func TestReadMissingFileIsEmpty(t *testing.T) {
	es, err := Read(t.TempDir())
	if err != nil || len(es) != 0 {
		t.Fatalf("got %v, %v", es, err)
	}
}

func TestAppendReadNewestFirst(t *testing.T) {
	dir := t.TempDir()
	mustAppend(t, dir, Entry{Time: at(1), Kind: KindPROpened})
	mustAppend(t, dir, Entry{Time: at(2), Kind: KindMarkedReady})
	es, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 2 || es[0].Kind != KindMarkedReady || es[1].Kind != KindPROpened {
		t.Fatalf("order: %+v", es)
	}
}

func TestEveryKindRoundTrips(t *testing.T) {
	dir := t.TempDir()
	kinds := []Kind{KindPROpened, KindPRAdopted, KindMarkedReady, KindReranCheck, KindCIFailed, KindFlaky,
		KindPausedCIFailing, KindTryAgainReset, KindPRMerged, KindTrunkUpdated, KindPRClosed,
		KindGitHubDown, KindGitHubUp, KindMarkedMerged}
	for i, k := range kinds {
		mustAppend(t, dir, Entry{Time: at(i), Kind: k, Detail: "d", Checks: []string{"lint"}, HeadSHA: "abc", TicketID: "07"})
	}
	es, _ := Read(dir)
	if len(es) != len(kinds) {
		t.Fatalf("got %d entries", len(es))
	}
	for i, e := range es {
		want := Entry{Time: at(len(kinds) - 1 - i), Kind: kinds[len(kinds)-1-i], Detail: "d", Checks: []string{"lint"}, HeadSHA: "abc", TicketID: "07"}
		if !e.Time.Equal(want.Time) || e.Kind != want.Kind || e.Detail != "d" || e.Checks[0] != "lint" || e.HeadSHA != "abc" || e.TicketID != "07" {
			t.Fatalf("entry %d: %+v", i, e)
		}
	}
}

func TestTornLastLineIgnoredAndNextAppendSurvives(t *testing.T) {
	dir := t.TempDir()
	mustAppend(t, dir, Entry{Time: at(1), Kind: KindPROpened})
	f, err := os.OpenFile(filepath.Join(dir, ActivityFile), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString(`{"time":"2026-01-01T00:02:00Z","ki`)
	_ = f.Close()
	es, err := Read(dir)
	if err != nil || len(es) != 1 {
		t.Fatalf("torn: %v %v", es, err)
	}
	mustAppend(t, dir, Entry{Time: at(3), Kind: KindPRMerged})
	es, _ = Read(dir)
	if len(es) != 2 || es[0].Kind != KindPRMerged {
		t.Fatalf("after append: %+v", es)
	}
}

func TestCountsRespectNewestReset(t *testing.T) {
	dir := t.TempDir()
	mustAppend(t, dir, Entry{Time: at(1), Kind: KindCIFailed, Checks: []string{"a", "b"}})
	mustAppend(t, dir, Entry{Time: at(2), Kind: KindPRClosed})
	mustAppend(t, dir, Entry{Time: at(3), Kind: KindTryAgainReset})
	mustAppend(t, dir, Entry{Time: at(4), Kind: KindCIFailed, Checks: []string{"a"}})
	mustAppend(t, dir, Entry{Time: at(5), Kind: KindCIFailed, Checks: []string{"a", "c"}})
	mustAppend(t, dir, Entry{Time: at(6), Kind: KindPROpened})

	by, _ := CIFailedByCheck(dir)
	if by["a"] != 2 || by["b"] != 0 || by["c"] != 1 {
		t.Fatalf("per check: %v", by)
	}
	if n, _ := CIFailedAttempts(dir); n != 2 {
		t.Fatalf("per PR: %d", n)
	}
	if c, _ := LastClosed(dir); !c.IsZero() {
		t.Fatalf("closed before reset counted: %v", c)
	}
	if o, _ := LastOpened(dir); !o.Equal(at(6)) {
		t.Fatalf("opened: %v", o)
	}
	if r, _ := ResetTime(dir); !r.Equal(at(3)) {
		t.Fatalf("reset: %v", r)
	}
}

func TestCountsWithoutReset(t *testing.T) {
	dir := t.TempDir()
	mustAppend(t, dir, Entry{Time: at(1), Kind: KindCIFailed, Checks: []string{"a"}})
	mustAppend(t, dir, Entry{Time: at(2), Kind: KindPRClosed})
	if r, _ := ResetTime(dir); !r.IsZero() {
		t.Fatalf("reset: %v", r)
	}
	if n, _ := CIFailedAttempts(dir); n != 1 {
		t.Fatalf("per PR: %d", n)
	}
	if c, _ := LastClosed(dir); !c.Equal(at(2)) {
		t.Fatalf("closed: %v", c)
	}
}
