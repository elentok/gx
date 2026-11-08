package triage

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/tickets/tickettest"
)

func writeFollowUp(t *testing.T, projectDir, file, status, title string) {
	t.Helper()
	tickettest.WriteFile(t, filepath.Join(projectDir, "follow-ups", "issues", file),
		"---\nid: \""+file[:2]+"\"\nstatus: "+status+"\ntype: implement\n---\n\n# "+title+"\n\n## What to build\n\nx\n")
}

func newProject(t *testing.T) (*Store, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "proj")
	writeFollowUp(t, dir, "01-a.md", "open", "Alpha")
	writeFollowUp(t, dir, "02-b.md", "draft", "Beta")
	writeFollowUp(t, dir, "03-c.md", "done", "Gamma")
	return NewStore(dir), dir
}

func TestCreate_WritesRecordAndSnapshot(t *testing.T) {
	s, _ := newProject(t)
	rec, err := s.Create()
	if err != nil {
		t.Fatal(err)
	}
	if rec.State != StateLive || rec.Step != "verify" || rec.Revision != 1 || rec.ID == "" || rec.StartedAt.IsZero() {
		t.Fatalf("unexpected record: %+v", rec)
	}
	if len(rec.Rows) != 2 {
		t.Fatalf("snapshot should hold only draft and open follow-ups, got %+v", rec.Rows)
	}
	titles := []string{rec.Rows[0].Title, rec.Rows[1].Title}
	if titles[0] != "Alpha" || titles[1] != "Beta" {
		t.Fatalf("titles = %v", titles)
	}
	live, err := s.LoadLive()
	if err != nil || live == nil || live.ID != rec.ID || len(live.Rows) != 2 {
		t.Fatalf("LoadLive = %+v, %v", live, err)
	}
	byID, err := s.Load(rec.ID)
	if err != nil || byID == nil || byID.ID != rec.ID {
		t.Fatalf("Load = %+v, %v", byID, err)
	}
}

func TestCreate_RefusesWhileLive(t *testing.T) {
	s, _ := newProject(t)
	first, err := s.Create()
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Create()
	var live *LiveError
	if !errors.As(err, &live) || live.ID != first.ID {
		t.Fatalf("err = %v, want LiveError for %s", err, first.ID)
	}
}

func TestCreate_ConcurrentEndsWithOne(t *testing.T) {
	s, _ := newProject(t)
	const n = 8
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok, refused := 0, 0
	for range n {
		wg.Go(func() {
			_, err := s.Create()
			var live *LiveError
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				ok++
			case errors.As(err, &live):
				refused++
			default:
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
	wg.Wait()
	if ok != 1 || refused != n-1 {
		t.Fatalf("ok=%d refused=%d", ok, refused)
	}
	entries, _ := filepath.Glob(filepath.Join(s.dir(), "*.json"))
	if len(entries) != 1 {
		t.Fatalf("records on disk = %v", entries)
	}
}

func TestCreate_ReplacesStaleMarker(t *testing.T) {
	s, _ := newProject(t)
	if err := os.MkdirAll(s.dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.markerPath(), []byte("20200101T000000-dead"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec, err := s.Create()
	if err != nil {
		t.Fatalf("stale marker should be replaced: %v", err)
	}
	live, _ := s.LoadLive()
	if live == nil || live.ID != rec.ID {
		t.Fatalf("live = %+v", live)
	}
}

func TestSave_StaleRevisionRefused(t *testing.T) {
	s, _ := newProject(t)
	rec, _ := s.Create()
	other, _ := s.Load(rec.ID)

	rec.Step = "plan"
	if err := s.Save(rec); err != nil {
		t.Fatal(err)
	}
	if rec.Revision != 2 {
		t.Fatalf("revision = %d, want 2", rec.Revision)
	}

	other.Step = "write"
	if err := s.Save(other); !errors.Is(err, ErrStaleRevision) {
		t.Fatalf("err = %v, want ErrStaleRevision", err)
	}
	got, _ := s.Load(rec.ID)
	if got.Step != "plan" || got.Revision != 2 {
		t.Fatalf("record changed by refused save: %+v", got)
	}
}

func TestSave_LeavingLiveReleasesMarker(t *testing.T) {
	s, _ := newProject(t)
	rec, _ := s.Create()
	rec.State = StateDone
	if err := s.Save(rec); err != nil {
		t.Fatal(err)
	}
	if live, err := s.LoadLive(); err != nil || live != nil {
		t.Fatalf("LoadLive = %+v, %v", live, err)
	}
	if _, err := s.Create(); err != nil {
		t.Fatalf("create after done: %v", err)
	}
}

func TestMissingFolderIsNoTriage(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "nothing"))
	if live, err := s.LoadLive(); live != nil || err != nil {
		t.Fatalf("LoadLive = %+v, %v", live, err)
	}
	if h, err := s.History(); len(h) != 0 || err != nil {
		t.Fatalf("History = %+v, %v", h, err)
	}
	if r, err := s.Load("x"); r != nil || err != nil {
		t.Fatalf("Load = %+v, %v", r, err)
	}
}

func TestHistory_NewestFirst(t *testing.T) {
	s, _ := newProject(t)
	var ids []string
	for i := range 3 {
		rec, err := s.Create()
		if err != nil {
			t.Fatal(err)
		}
		rec.State = StateAbandoned
		rec.StartedAt = time.Date(2026, 1, 1+i, 0, 0, 0, 0, time.UTC)
		if err := s.Save(rec); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, rec.ID)
	}
	h, err := s.History()
	if err != nil || len(h) != 3 {
		t.Fatalf("History = %d, %v", len(h), err)
	}
	if h[0].ID != ids[2] || h[1].ID != ids[1] || h[2].ID != ids[0] {
		t.Fatalf("order = %s %s %s, want %v reversed", h[0].ID, h[1].ID, h[2].ID, ids)
	}
}
