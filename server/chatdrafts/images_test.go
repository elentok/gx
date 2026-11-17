package chatdrafts

import (
	"os"
	"testing"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/ops"
)

var testPNG = append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 16)...)

func TestStore_AddAndRemoveImageKeepsTheRevision(t *testing.T) {
	t.Parallel()
	s := New(t.TempDir())
	d := newDraft(t, s, "brief")
	res, err := s.AddImage(d.ID, testPNG)
	if err != nil || res.Refused || res.ID == "" {
		t.Fatalf("add = %+v, %v", res, err)
	}
	got, _ := s.Get(d.ID)
	if len(got.Images) != 1 || got.Images[0] != res.ID || got.Revision != d.Revision {
		t.Fatalf("after add = %+v", got)
	}
	if rm, err := s.RemoveImage(d.ID, res.ID); err != nil || rm.Refused {
		t.Fatalf("remove = %+v, %v", rm, err)
	}
	got, _ = s.Get(d.ID)
	if len(got.Images) != 0 {
		t.Fatalf("after remove = %+v", got.Images)
	}
	if rm, _ := s.RemoveImage(d.ID, res.ID); rm.Reason != wire.ReasonImageNotFound {
		t.Fatalf("second remove = %+v", rm)
	}
}

func TestStore_AddImageToMissingDraftIsGone(t *testing.T) {
	t.Parallel()
	s := New(t.TempDir())
	if res, _ := s.AddImage("0123456789abcdef", testPNG); !res.Refused || res.Reason != "draft-gone" {
		t.Fatalf("add = %+v", res)
	}
}

func TestStore_AddImageStopsAtTheLimit(t *testing.T) {
	t.Parallel()
	s := New(t.TempDir())
	d := newDraft(t, s, "brief")
	for range wire.MaxPromptImages {
		if res, err := s.AddImage(d.ID, testPNG); err != nil || res.Refused {
			t.Fatalf("add = %+v, %v", res, err)
		}
	}
	if res, _ := s.AddImage(d.ID, testPNG); res.Reason != wire.ReasonImageTooMany {
		t.Fatalf("over the limit = %+v", res)
	}
}

func TestStore_DeleteRemovesTheImages(t *testing.T) {
	t.Parallel()
	s := New(t.TempDir())
	d := newDraft(t, s, "brief")
	if _, err := s.AddImage(d.ID, testPNG); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Delete(d.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.ImagesDir(d.ID)); !os.IsNotExist(err) {
		t.Fatalf("images folder survived Delete: %v", err)
	}
}

func TestStart_PassesTheDraftsImagesApartFromTheBrief(t *testing.T) {
	t.Parallel()
	r := newStartRig(t)
	d := r.draft(t, ops2Draft("the brief"))
	res, err := r.c.store.AddImage(d.ID, testPNG)
	if err != nil || res.Refused {
		t.Fatalf("add = %+v, %v", res, err)
	}
	if out, err := r.c.Start(d.ID); err != nil || out.Refused {
		t.Fatalf("start = %+v, %v", out, err)
	}
	req := r.reqs[0]
	if req.Prompt != "the brief" || len(req.ImagePaths) != 1 {
		t.Fatalf("prompt = %q, images = %v", req.Prompt, req.ImagePaths)
	}
	// The draft is gone by now, but the passed copy still exists.
	if _, err := os.Stat(req.ImagePaths[0]); err != nil {
		t.Fatalf("image %q: %v", req.ImagePaths[0], err)
	}
}

func ops2Draft(brief string) ops.ChatDraft { return ops.ChatDraft{Brief: brief} }
