package designs_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/server/designs/drafts"
	"github.com/elentok/gx/server/wire"
)

func (f designsFixture) openFromDraft(t *testing.T, slug, idea, draft string) wire.SessionResult {
	t.Helper()
	res, err := f.h.Client.Sessions.OpenDesign(f.ctx, wire.DesignOpenRequest{Epic: slug, NewDesign: true, Idea: idea, Project: "proj", DraftID: draft})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// draftWithImages seeds a draft holding two PNG uploads and returns it with the image file names.
func (f designsFixture) draftWithImages(t *testing.T) (*wire.DesignDraft, []string) {
	t.Helper()
	d := f.seedDraft(t, "proj", "fresh-idea", "an idea")
	var names []string
	for range 2 {
		res := f.uploadDraftImage(t, d.ID, imageBytes("\x89PNG\r\n\x1a\n", 40))
		if res.Refused {
			t.Fatalf("upload = %+v", res)
		}
		names = append(names, res.Name)
	}
	return d, names
}

func TestDesignsOpenNew_FromDraftMovesImagesAndListsThem(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	d, names := f.draftWithImages(t)
	res := f.openFromDraft(t, "fresh-idea", "an idea", d.ID)
	if res.Refused || res.Session == nil {
		t.Fatalf("open = %+v", res)
	}
	imagesDir := filepath.Join(f.h.TicketStore, "proj", "fresh-idea", "images")
	raw, err := os.ReadFile(f.mapPath("fresh-idea"))
	if err != nil {
		t.Fatal(err)
	}
	want := "## Initial prompt\n\nan idea\n\nImages:\n" + filepath.Join(imagesDir, names[0]) + "\n" + filepath.Join(imagesDir, names[1]) + "\n"
	if !strings.Contains(string(raw), want) {
		t.Errorf("map lacks %q:\n%s", want, raw)
	}
	for _, n := range names {
		if _, err := os.Stat(filepath.Join(imagesDir, n)); err != nil {
			t.Errorf("image %s not moved: %v", n, err)
		}
	}
	if _, err := os.Stat(filepath.Dir(drafts.New(f.h.StateDir).ImagesDir(d.ID))); err == nil {
		t.Error("draft folder still exists")
	}
	if got, _ := drafts.New(f.h.StateDir).LastProject(); got != "proj" {
		t.Errorf("last project = %q, want proj", got)
	}
	if meta := readMeta(t, f.h, res.Session.ID); meta["first_prompt"] != "/gx-design proj:fresh-idea" {
		t.Errorf("first_prompt = %v, want the bare skill command", meta["first_prompt"])
	}
}

func TestDesignsOpenNew_WithoutImagesAddsNoImagesLine(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	d := f.seedDraft(t, "proj", "fresh-idea", "an idea")
	if res := f.openFromDraft(t, "fresh-idea", "an idea", d.ID); res.Refused || res.Session == nil {
		t.Fatalf("open = %+v", res)
	}
	raw, _ := os.ReadFile(f.mapPath("fresh-idea"))
	if strings.Contains(string(raw), "Images:") || !strings.HasSuffix(string(raw), "## Initial prompt\n\nan idea\n") {
		t.Errorf("map = %s", raw)
	}
}

func TestDesignsOpenNew_MissingDraftStillStarts(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	if res := f.openFromDraft(t, "fresh-idea", "an idea", "0123456789abcdef"); res.Refused || res.Session == nil {
		t.Fatalf("open = %+v", res)
	}
	if got, _ := drafts.New(f.h.StateDir).LastProject(); got != "proj" {
		t.Errorf("last project = %q, want proj", got)
	}
}

func TestDesignsOpenNew_EpicExistsKeepsTheDraft(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	d, names := f.draftWithImages(t)
	res := f.openFromDraft(t, "map", "an idea", d.ID)
	if !res.Refused || res.Reason != wire.ReasonDesignEpicExists {
		t.Fatalf("open = %+v", res)
	}
	if got := f.readDraft(t, d.ID); len(got.Images) != len(names) {
		t.Errorf("draft images = %v", got.Images)
	}
	if got, _ := drafts.New(f.h.StateDir).LastProject(); got != "" {
		t.Errorf("last project = %q, want unset", got)
	}
}

func TestDesignsOpenNew_MapWriteFailureKeepsTheDraft(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	d, names := f.draftWithImages(t)
	projDir := filepath.Join(f.h.TicketStore, "proj")
	if err := os.Chmod(projDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(projDir, 0o755) })
	if _, err := os.Create(filepath.Join(projDir, "probe")); err == nil {
		t.Skip("directory permissions are not enforced here")
	}
	if _, err := f.h.Client.Sessions.OpenDesign(f.ctx, wire.DesignOpenRequest{Epic: "fresh-idea", NewDesign: true, Idea: "an idea", Project: "proj", DraftID: d.ID}); err == nil {
		t.Fatal("open succeeded, want a failed map write")
	}
	if got := f.readDraft(t, d.ID); len(got.Images) != len(names) {
		t.Errorf("draft images = %v", got.Images)
	}
}
