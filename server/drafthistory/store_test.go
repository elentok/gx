package drafthistory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func fixedClock() func() time.Time {
	t := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return func() time.Time { return t }
}

func newStore(t *testing.T, capBytes int64) *Store {
	t.Helper()
	return NewWith(t.TempDir(), capBytes, fixedClock())
}

func img(t *testing.T, name string, size int, fill byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(strings.Repeat(string(fill), size)), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func mustSave(t *testing.T, s *Store, l List, e Entry, imgs ...string) Result {
	t.Helper()
	r, err := s.Save(l, e, imgs)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func texts(t *testing.T, s *Store, l List) []string {
	t.Helper()
	items, err := s.List(l)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, it := range items {
		out = append(out, it.Text)
	}
	return out
}

func TestSave_EmptyIsSkippedImagesOnlyIsSaved(t *testing.T) {
	s := newStore(t, 0)
	if r := mustSave(t, s, Sent, Entry{Text: "  \n\t"}); r != SkippedEmpty {
		t.Fatalf("whitespace: got %v", r)
	}
	if got := texts(t, s, Sent); len(got) != 0 {
		t.Fatalf("list not empty: %v", got)
	}
	if r := mustSave(t, s, Sent, Entry{}, img(t, "img-0-a.png", 4, 'a')); r != Saved {
		t.Fatalf("images-only: got %v", r)
	}
	items, _ := s.List(Sent)
	if len(items) != 1 || items[0].ImageCount != 1 {
		t.Fatalf("items: %+v", items)
	}
}

func TestSave_RepeatOfNewestIsSkipped(t *testing.T) {
	s := newStore(t, 0)
	a := img(t, "img-0-a.png", 4, 'a')
	e := Entry{Text: "hi", Project: "p", Harness: "claude"}
	if r := mustSave(t, s, ChatDrafts, e, a); r != Saved {
		t.Fatal(r)
	}
	// Same bytes under another path still repeat.
	if r := mustSave(t, s, ChatDrafts, e, img(t, "img-0-a.png", 4, 'a')); r != SkippedRepeat {
		t.Fatalf("repeat: got %v", r)
	}
	if r := mustSave(t, s, ChatDrafts, e, a, img(t, "img-1-b.png", 4, 'b')); r != Saved {
		t.Fatalf("added image: got %v", r)
	}
	other := e
	other.Model = "opus"
	if r := mustSave(t, s, ChatDrafts, other, a, img(t, "img-1-b.png", 4, 'b')); r != Saved {
		t.Fatalf("other settings: got %v", r)
	}
	if r := mustSave(t, s, ChatDrafts, other, a, img(t, "img-1-b.png", 4, 'c')); r != Saved {
		t.Fatalf("other bytes: got %v", r)
	}
	if items, _ := s.List(ChatDrafts); len(items) != 4 {
		t.Fatalf("want 4 entries, got %d", len(items))
	}
}

func TestSave_ListHoldsFiftyNewestFirstAcrossProjects(t *testing.T) {
	s := newStore(t, 0)
	for i := range MaxEntries + 1 {
		mustSave(t, s, DesignDrafts, Entry{Text: fmt.Sprint(i), Project: fmt.Sprint("p", i%3), Slug: fmt.Sprint("s", i)})
	}
	got := texts(t, s, DesignDrafts)
	if len(got) != MaxEntries || got[0] != "50" || got[len(got)-1] != "1" {
		t.Fatalf("got %d entries, first %q last %q", len(got), got[0], got[len(got)-1])
	}
}

func TestSave_EntriesKeepTheirFields(t *testing.T) {
	s := newStore(t, 0)
	mustSave(t, s, DesignDrafts, Entry{Text: "d", Project: "p", Slug: "sl"})
	mustSave(t, s, ChatDrafts, Entry{Text: "c", Project: "p", Name: "n", Harness: "h", Model: "m", Effort: "e", Folder: "/f"})
	mustSave(t, s, Sent, Entry{Text: "s", Composer: ComposerDesign, Slug: "sl", Project: "p"})
	d, _ := s.List(DesignDrafts)
	c, _ := s.List(ChatDrafts)
	m, _ := s.List(Sent)
	if d[0].Project != "p" || d[0].Slug != "sl" {
		t.Fatalf("design: %+v", d[0])
	}
	if c[0].Name != "n" || c[0].Harness != "h" || c[0].Model != "m" || c[0].Effort != "e" || c[0].Folder != "/f" {
		t.Fatalf("chat: %+v", c[0])
	}
	if m[0].Composer != ComposerDesign || m[0].Slug != "sl" || m[0].Project != "p" {
		t.Fatalf("sent: %+v", m[0])
	}
}

func TestSave_ImagesSurviveSourceDeletion(t *testing.T) {
	s := newStore(t, 0)
	src := img(t, "img-0-a.png", 6, 'x')
	mustSave(t, s, Sent, Entry{Text: "t"}, src)
	if err := os.Remove(src); err != nil {
		t.Fatal(err)
	}
	items, _ := s.List(Sent)
	paths, err := s.ImagePaths(Sent, items[0].ID)
	if err != nil || len(paths) != 1 {
		t.Fatalf("paths %v err %v", paths, err)
	}
	b, err := os.ReadFile(paths[0])
	if err != nil || string(b) != "xxxxxx" {
		t.Fatalf("read %q err %v", b, err)
	}
}

func TestSave_CapDropsOldestEntries(t *testing.T) {
	s := newStore(t, 100)
	for i := range 4 {
		mustSave(t, s, Sent, Entry{Text: fmt.Sprint(i)}, img(t, "img-0-a.png", 40, byte('a'+i)))
	}
	if got := texts(t, s, Sent); len(got) != 2 || got[0] != "3" || got[1] != "2" {
		t.Fatalf("got %v", got)
	}
	var total int64
	items, _ := s.List(Sent)
	for _, it := range items {
		ps, _ := s.ImagePaths(Sent, it.ID)
		for _, p := range ps {
			fi, err := os.Stat(p)
			if err != nil {
				t.Fatal(err)
			}
			total += fi.Size()
		}
	}
	if total > 100 {
		t.Fatalf("total %d over cap", total)
	}
	// Dropped entries leave no folder behind.
	dirs, _ := os.ReadDir(s.dir(Sent))
	if len(dirs) != 3 { // list.json + 2 entries
		t.Fatalf("dir has %d children", len(dirs))
	}
}

func TestSave_OversizedImagesAreNotKept(t *testing.T) {
	s := newStore(t, 100)
	mustSave(t, s, Sent, Entry{Text: "old"}, img(t, "img-0-a.png", 10, 'o'))
	mustSave(t, s, Sent, Entry{Text: "big"}, img(t, "img-0-a.png", 60, 'b'), img(t, "img-1-b.png", 60, 'c'))
	items, _ := s.List(Sent)
	if len(items) != 2 {
		t.Fatalf("want 2 entries, got %d", len(items))
	}
	if items[0].Text != "big" || items[0].ImageCount != 0 || !items[0].ImagesDropped {
		t.Fatalf("big: %+v", items[0])
	}
}

func TestSave_OverCapRepeatIsSkippedButOtherImagesSave(t *testing.T) {
	s := newStore(t, 100)
	big := func(fill byte) []string {
		return []string{img(t, "img-0-a.png", 60, fill), img(t, "img-1-b.png", 60, 'z')}
	}
	if r := mustSave(t, s, Sent, Entry{Text: "big"}, big('b')...); r != Saved {
		t.Fatal(r)
	}
	if r := mustSave(t, s, Sent, Entry{Text: "big"}, big('b')...); r != SkippedRepeat {
		t.Fatalf("same over-cap draft: got %v", r)
	}
	if r := mustSave(t, s, Sent, Entry{Text: "big"}, big('c')...); r != Saved {
		t.Fatalf("other over-cap images: got %v", r)
	}
	if items, _ := s.List(Sent); len(items) != 2 {
		t.Fatalf("want 2 entries, got %d", len(items))
	}
}

func TestSave_ConcurrentStoresLoseNothing(t *testing.T) {
	dir := t.TempDir()
	const n = 20
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			s := NewWith(dir, 0, fixedClock())
			if _, err := s.Save(Sent, Entry{Text: fmt.Sprint(i)}, nil); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if got := texts(t, NewWith(dir, 0, nil), Sent); len(got) != n {
		t.Fatalf("want %d entries, got %d", n, len(got))
	}
}
