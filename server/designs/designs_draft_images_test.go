package designs_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/elentok/gx/server/designs/drafts"
	"github.com/elentok/gx/server/imagestore"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/ops"
)

func (f designsFixture) uploadDraftImage(t *testing.T, draft string, data []byte) wire.SessionImageResult {
	t.Helper()
	h := map[string]string{"Origin": f.base, "Content-Type": "application/octet-stream"}
	code, out := servertest.WebPostJSON(t, f.base+"/v1/designs/drafts/images?id="+url.QueryEscape(draft), string(data), f.ck, "", h)
	if code != http.StatusOK {
		t.Fatalf("upload = %d %s", code, out)
	}
	var res wire.SessionImageResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err)
	}
	return res
}

func (f designsFixture) removeDraftImage(t *testing.T, draft, image string) wire.SessionImageResult {
	t.Helper()
	body := fmt.Sprintf(`{"id":%q,"image":%q}`, draft, image)
	code, out := servertest.WebPostJSON(t, f.base+"/v1/designs/drafts/images/delete", body, f.ck, "", servertest.JSONPost(f.base))
	if code != http.StatusOK {
		t.Fatalf("remove = %d %s", code, out)
	}
	var res wire.SessionImageResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err)
	}
	return res
}

func (f designsFixture) readDraft(t *testing.T, id string) *ops.DesignDraft {
	t.Helper()
	var read ops.DesignDraftSaveResult
	f.webGet(t, "/v1/designs/drafts/read?id="+url.QueryEscape(id), &read)
	if read.Draft == nil {
		t.Fatalf("read %s = %+v", id, read)
	}
	return read.Draft
}

func imageBytes(magic string, size int) []byte {
	return append([]byte(magic), make([]byte, size-len(magic))...)
}

func TestWebDesignDraftImages_Upload(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	d := f.seedDraft(t, "proj", "s", "idea")

	cases := []struct {
		name   string
		data   []byte
		reason string
	}{
		{"png", imageBytes("\x89PNG\r\n\x1a\n", 40), ""},
		{"jpeg", imageBytes("\xff\xd8\xff\xe0", 40), ""},
		{"gif", imageBytes("GIF89a", 40), ""},
		{"webp", append(append([]byte("RIFF\x00\x00\x00\x00WEBP"), 'V', 'P', '8', ' '), make([]byte, 20)...), ""},
		{"text", []byte("just some text, not an image"), wire.ReasonImageTypeNotAllowed},
		{"too large", imageBytes("\x89PNG\r\n\x1a\n", imagestore.MaxBytes+1), wire.ReasonImageTooLarge},
	}
	accepted := 0
	for _, c := range cases {
		res := f.uploadDraftImage(t, d.ID, c.data)
		if res.Reason != c.reason || res.Refused != (c.reason != "") {
			t.Errorf("%s: result = %+v, want reason %q", c.name, res, c.reason)
		}
		if c.reason == "" {
			accepted++
		}
	}
	if got := f.readDraft(t, d.ID).Images; len(got) != accepted {
		t.Errorf("draft lists %d images, want %d", len(got), accepted)
	}
}

func TestWebDesignDraftImages_TooMany(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	d := f.seedDraft(t, "proj", "s", "idea")
	png := imageBytes("\x89PNG\r\n\x1a\n", 40)
	for i := 0; i < wire.MaxPromptImages; i++ {
		if res := f.uploadDraftImage(t, d.ID, png); res.Refused {
			t.Fatalf("image %d = %+v", i+1, res)
		}
	}
	if res := f.uploadDraftImage(t, d.ID, png); res.Reason != wire.ReasonImageTooMany {
		t.Errorf("11th image = %+v, want too-many-images", res)
	}
}

func TestWebDesignDraftImages_GoneDraft(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	d := f.seedDraft(t, "proj", "s", "idea")
	png := imageBytes("\x89PNG\r\n\x1a\n", 40)
	if err := drafts.New(f.h.StateDir).Delete(d.ID); err != nil {
		t.Fatal(err)
	}
	if res := f.uploadDraftImage(t, d.ID, png); res.Reason != ops.ReasonDraftGone {
		t.Errorf("upload to a deleted draft = %+v, want draft-gone", res)
	}
	if res := f.removeDraftImage(t, d.ID, "abcd1234"); res.Reason != ops.ReasonDraftGone {
		t.Errorf("remove from a deleted draft = %+v, want draft-gone", res)
	}
}

func TestWebDesignDraftImages_UploadDuringTextSaveRaisesNoConflict(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	d := f.seedDraft(t, "proj", "s", "idea")
	loaded := f.readDraft(t, d.ID)

	up := f.uploadDraftImage(t, d.ID, imageBytes("\x89PNG\r\n\x1a\n", 40))
	if up.Refused {
		t.Fatalf("upload = %+v", up)
	}
	res := f.webSave(t, d.ID, loaded.Revision, "typed meanwhile")
	if res.Refused || res.Draft == nil {
		t.Fatalf("save after an upload = %+v, want no conflict", res)
	}
	if res.Draft.Revision != loaded.Revision+1 {
		t.Errorf("revision = %d, want %d: the upload must not bump it", res.Draft.Revision, loaded.Revision+1)
	}
	if got := f.readDraft(t, d.ID).Images; len(got) != 1 || got[0] != up.ID {
		t.Errorf("images after the save = %v, want [%s]", got, up.ID)
	}
}

func TestWebDesignDraftImages_RemoveAndDelete(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	d := f.seedDraft(t, "proj", "s", "idea")
	png := imageBytes("\x89PNG\r\n\x1a\n", 40)
	a := f.uploadDraftImage(t, d.ID, png)
	b := f.uploadDraftImage(t, d.ID, png)
	dir := drafts.New(f.h.StateDir).ImagesDir(d.ID)

	before := f.readDraft(t, d.ID)
	if res := f.removeDraftImage(t, d.ID, a.ID); res.Refused {
		t.Fatalf("remove = %+v", res)
	}
	if _, err := os.Stat(filepath.Join(dir, a.Name)); !os.IsNotExist(err) {
		t.Errorf("removed file still on disk: %v", err)
	}
	after := f.readDraft(t, d.ID)
	if len(after.Images) != 1 || after.Images[0] != b.ID || after.Revision != before.Revision {
		t.Errorf("draft after remove = %+v, want only %s at revision %d", after, b.ID, before.Revision)
	}
	if res := f.removeDraftImage(t, d.ID, a.ID); res.Reason != wire.ReasonImageNotFound {
		t.Errorf("second remove = %+v, want image-not-found", res)
	}

	if err := drafts.New(f.h.StateDir).Delete(d.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("image folder survived the delete: %v", err)
	}
}

func TestWebDesignDraftImages_DraftWithImageSurvivesEmptyingText(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	d := f.seedDraft(t, "proj", "s", "idea")
	up := f.uploadDraftImage(t, d.ID, imageBytes("\x89PNG\r\n\x1a\n", 40))

	body := fmt.Sprintf(`{"id":%q,"project":"proj","slug":"","idea":"","revision":%d}`, d.ID, d.Revision)
	code, out := servertest.WebPostJSON(t, f.base+"/v1/designs/drafts/save", body, f.ck, "", servertest.JSONPost(f.base))
	var res ops.DesignDraftSaveResult
	if code != http.StatusOK || json.Unmarshal([]byte(out), &res) != nil || res.Deleted || res.Draft == nil {
		t.Fatalf("emptying save = %d %s, want the draft kept", code, out)
	}
	if got := f.readDraft(t, d.ID).Images; !slices.Equal(got, []string{up.ID}) {
		t.Errorf("images = %v, want [%s]", got, up.ID)
	}

	// With the image gone too, the same save deletes the draft.
	f.removeDraftImage(t, d.ID, up.ID)
	cur := f.readDraft(t, d.ID)
	body = fmt.Sprintf(`{"id":%q,"project":"proj","slug":"","idea":"","revision":%d}`, d.ID, cur.Revision)
	code, out = servertest.WebPostJSON(t, f.base+"/v1/designs/drafts/save", body, f.ck, "", servertest.JSONPost(f.base))
	res = ops.DesignDraftSaveResult{}
	if code != http.StatusOK || json.Unmarshal([]byte(out), &res) != nil || !res.Deleted {
		t.Errorf("emptying a draft with no images = %d %s, want deleted", code, out)
	}
}
