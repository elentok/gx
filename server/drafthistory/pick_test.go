package drafthistory

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/server/imagestore"
)

func pngFile(t *testing.T, w int) string {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, w, 1))); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), fmt.Sprintf("img-%d.png", w))
	if err := os.WriteFile(p, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRestore_OldestEntryOfAFullListKeepsItsImages(t *testing.T) {
	t.Parallel()
	s := newStore(t, 0)
	mustSave(t, s, ChatDrafts, Entry{Text: "oldest"}, pngFile(t, 3), pngFile(t, 4))
	for i := 1; i < MaxEntries; i++ {
		mustSave(t, s, ChatDrafts, Entry{Text: fmt.Sprintf("e%d", i)})
	}
	items, err := s.List(ChatDrafts)
	if err != nil || len(items) != MaxEntries {
		t.Fatalf("list = %d %v, want a full list", len(items), err)
	}
	p, err := s.Load(ChatDrafts, items[len(items)-1].ID)
	if err != nil {
		t.Fatal(err)
	}

	res, err := s.Restore(ChatDrafts, p, Entry{Text: "page"}, t.TempDir(), nil, func() error {
		_, err := s.Save(ChatDrafts, Entry{Text: "page"}, nil)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed || len(res.Images) != 2 || res.Missing != 0 {
		t.Errorf("restore = %+v, want 2 images and none missing", res)
	}
}

func TestRestore_SentEntryEqualInTextAndImagesChangesNothing(t *testing.T) {
	t.Parallel()
	s := newStore(t, 0)
	src := pngFile(t, 5)
	mustSave(t, s, Sent, Entry{Text: "same", Composer: ComposerChat, Name: "other", Harness: "codex"}, src)
	items, _ := s.List(Sent)
	p, err := s.Load(Sent, items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	saved, err := imagestore.Save(dir, b)
	if err != nil || saved.Refused {
		t.Fatalf("save image: %+v %v", saved, err)
	}

	page := Entry{Text: "same", Composer: ComposerChat, Name: "my-chat", Harness: "claude"}
	res, err := s.Restore(Sent, p, page, dir, []string{saved.ID}, func() error {
		_, err := s.Save(Sent, page, []string{src})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed {
		t.Errorf("restore = %+v, want nothing changed", res)
	}
	if got := texts(t, s, Sent); len(got) != 1 {
		t.Errorf("sent holds %v, want the one entry", got)
	}
}
